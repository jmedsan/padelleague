package notify

import (
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/league"
)

// sendAndCapture delivers n to a verified player and returns the email HTML.
func sendAndCapture(t *testing.T, app core.App, n league.Notification, mailTotal func() int, last func() string) string {
	t.Helper()
	recipient := makeUser(t, app, "player")
	NewNotifier(app, "", "").NotifyPlayers([]string{recipient.Id}, n)
	require.Eventually(t, func() bool { return mailTotal() == 1 }, 2*time.Second, 10*time.Millisecond)
	return last()
}

const cardStart = "margin:16px 0;background:#f7f7f7"

// capture delivers n and returns the email HTML.
func capture(t *testing.T, app *tests.TestApp, n league.Notification) string {
	t.Helper()
	enableSMTP(t, app)
	return sendAndCapture(t, app, n,
		func() int { return app.TestMailer.TotalSend() },
		func() string { return app.TestMailer.LastMessage().HTML })
}

// assertInOrder fails unless every part occurs in html, in the given order.
func assertInOrder(t *testing.T, html string, parts ...string) {
	t.Helper()
	pos := 0
	for _, part := range parts {
		i := strings.Index(html[pos:], part)
		require.GreaterOrEqual(t, i, 0, "missing or out of order: %q", part)
		pos += i + len(part)
	}
}

func TestEmail_ChatMessageThenOneContextCard(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		state string // "", "confirmed" or "proposed"
	}{{"no date", ""}, {"confirmed date, time and venue", "confirmed"}, {"pending proposal", "proposed"}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			app := newTestApp(t)
			match := makeMatch(t, app)
			switch tc.state {
			case "confirmed":
				match.Set("date", "2026-10-20")
				match.Set("time", "18:30")
				match.Set("club", "Padel 360")
				match.Set("court_number", "3")
				require.NoError(t, app.Save(match))
			case "proposed":
				col, err := app.FindCollectionByNameOrId("match_messages")
				require.NoError(t, err)
				msg := core.NewRecord(col)
				msg.Set("match", match.Id)
				msg.Set("author", makeUser(t, app, "player").Id)
				msg.Set("type", "scheduling_proposal")
				msg.Set("proposal_data", `{"date":"2026-11-02","time":"19:00","venue_id":"","venue_name":"","venue_text":"Wurko"}`)
				msg.Set("proposal_status", "pending")
				require.NoError(t, app.Save(msg))
			}
			p1, _ := app.FindRecordById("pairs", match.GetString("pair1"))
			p2, _ := app.FindRecordById("pairs", match.GetString("pair2"))
			html := capture(t, app, league.Notification{Type: "message", Title: "T",
				Body: "Ana (Pareja X) escribió: ¿Jugamos a las 18?", Author: "Ana (Pareja X)", Text: "¿Jugamos a las 18?", MatchID: match.Id, CompName: "Liga Primavera"})

			parts := []string{
				"Ana · Pareja X escribió:",
				"¿Jugamos a las 18?",
				cardStart,
				"Competición</div>", "Liga Primavera",
				p1.GetString("name"),
				">vs</div>",
				p2.GetString("name"),
			}
			switch tc.state {
			case "confirmed":
				parts = append(parts, "Fecha y lugar", ">Confirmada</span>", ">Fecha</td>", "20/10/2026 18:30", ">Lugar</td>", "Padel 360, pista 3")
			case "proposed":
				parts = append(parts, "Fecha y lugar", ">Propuesta</span>", ">Fecha</td>", "02/11/2026 19:00", ">Lugar</td>", "Wurko")
			}
			parts = append(parts, "Ver partido")
			assertInOrder(t, html, parts...)
			assert.Equal(t, 1, strings.Count(html, cardStart), "competition and match share one card")
			assert.Contains(t, html, "border-left:4px solid #b5d334")
			assert.Contains(t, html, pairPlayers(t, app, p1.Id))
			assert.Contains(t, html, pairPlayers(t, app, p2.Id))
			switch tc.state {
			case "":
				assert.NotContains(t, html, "Fecha y lugar")
				assert.NotContains(t, html, ">Fecha</td>")
				assert.NotContains(t, html, ">Lugar</td>")
				assert.NotContains(t, html, ">Confirmada</span>")
				assert.NotContains(t, html, ">Propuesta</span>")
			case "confirmed":
				assert.NotContains(t, html, ">Propuesta</span>")
			case "proposed":
				assert.NotContains(t, html, ">Confirmada</span>")
				assert.NotContains(t, html, "20/10/2026")
			}
		})
	}
}

// pairPlayers is the expected "Player 1 / Player 2" text for a pair.
func pairPlayers(t *testing.T, app core.App, pairID string) string {
	t.Helper()
	pair, err := app.FindRecordById("pairs", pairID)
	require.NoError(t, err)
	return league.PlayerName(app, pair.GetString("player1")) + " / " + league.PlayerName(app, pair.GetString("player2"))
}

func TestEmail_NoContextCardWithoutMatchOrCompetition(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	html := capture(t, app, league.Notification{Type: "general", Title: "T", Body: "Cuerpo"})
	assert.Contains(t, html, "<p style=\"margin:0 0 4px;\">Cuerpo</p>")
	assert.NotContains(t, html, cardStart)
	assert.NotContains(t, html, "Competición")
	assert.NotContains(t, html, ">vs</div>")
}

func TestEmail_CompetitionOnlyCard(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	html := capture(t, app, league.Notification{Type: "penalty", Title: "T", Body: "3 puntos", CompName: "Liga Primavera"})
	assertInOrder(t, html, "3 puntos", cardStart, "Competición</div>", "Liga Primavera")
	assert.NotContains(t, html, ">vs</div>")
}

func TestEmail_NonChatBodyIsAParagraph(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	html := capture(t, app, league.Notification{Type: "general", Title: "T", Body: "Ana escribió: no es chat"})
	assert.Contains(t, html, "Ana escribió: no es chat</p>")
	assert.NotContains(t, html, "border-left:4px")
}

func TestPushBody(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		notif league.Notification
		want  string
	}{
		{"appends the competition", league.Notification{Body: "Acepta o contrapropón.", CompName: "Liga Primavera"}, "Acepta o contrapropón. · Liga Primavera"},
		{"no competition", league.Notification{Body: "Hola"}, "Hola"},
		{"body already names the competition", league.Notification{Body: "Retirada de Liga Primavera.", CompName: "Liga Primavera"}, "Retirada de Liga Primavera."},
		{"keeps the match prefix", league.Notification{Prefix: "A vs B: ", Body: "Hola", CompName: "Liga Primavera"}, "A vs B: Hola · Liga Primavera"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, pushBody(tt.notif))
		})
	}
}

func TestEmail_PrefixDroppedWhenContextCardShowsTheMatch(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	match := makeMatch(t, app)
	html := capture(t, app, league.Notification{Type: "dispute", Title: "T", Prefix: "ZZ1 vs ZZ2: ",
		Body: "Ana ha cancelado la fecha.", MatchID: match.Id, CompName: "Liga Primavera"})
	assert.Contains(t, html, "Ana ha cancelado la fecha.")
	assert.NotContains(t, html, "ZZ1 vs ZZ2")
	assert.Contains(t, html, cardStart)
}

func TestEmail_PrefixKeptWithoutAMatchCard(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	html := capture(t, app, league.Notification{Type: "general", Title: "T", Prefix: "ZZ1 vs ZZ2: ", Body: "Cuerpo"})
	assert.Contains(t, html, "ZZ1 vs ZZ2: Cuerpo")
}

func TestDeliver_InAppBodyIncludesPrefix(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	recipient := makeUser(t, app, "player")
	NewNotifier(app, "", "").NotifyPlayers([]string{recipient.Id}, league.Notification{Type: "general", Title: "T", Prefix: "ZZ1 vs ZZ2: ", Body: "Cuerpo"})
	recs, err := app.FindRecordsByFilter("notifications", "user = {:u}", "", 0, 0, map[string]any{"u": recipient.Id})
	require.NoError(t, err)
	require.Len(t, recs, 1)
	assert.Equal(t, "ZZ1 vs ZZ2: Cuerpo", recs[0].GetString("body"))
}

func TestEmail_ChatMessageIsNotTruncated(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	match := makeMatch(t, app)
	text := strings.Repeat("palabra ", 25) + "FIN"
	n := league.NotifNewMessage(match.Id, "Ana (Pareja X)", text, "Liga Primavera")
	require.Less(t, len(n.Body), len(text), "the in-app body stays truncated")
	html := capture(t, app, n)
	assert.Contains(t, html, text)
}

// proposeSchedule saves a scheduling_proposal message on the match.
func proposeSchedule(t *testing.T, app core.App, match *core.Record, data, status string) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("match_messages")
	require.NoError(t, err)
	msg := core.NewRecord(col)
	msg.Set("match", match.Id)
	msg.Set("author", makeUser(t, app, "player").Id)
	msg.Set("type", "scheduling_proposal")
	msg.Set("proposal_data", data)
	msg.Set("proposal_status", status)
	require.NoError(t, app.Save(msg))
}

func TestLoadMatchInfo_Schedule(t *testing.T) {
	t.Parallel()
	confirm := func(m *core.Record, clubCourt, clock string) {
		m.Set("date", "2026-10-20")
		m.Set("time", clock)
		m.Set("club", "Padel 360")
		m.Set("court_number", clubCourt)
	}
	tests := []struct {
		name  string
		setup func(t *testing.T, app core.App, m *core.Record)
		want  matchInfo
	}{
		{"confirmed without court or time", func(_ *testing.T, _ core.App, m *core.Record) { confirm(m, "", "") },
			matchInfo{Status: scheduleConfirmed, When: "20/10/2026", Place: "Padel 360"}},
		{"date without club is not confirmed", func(_ *testing.T, _ core.App, m *core.Record) { m.Set("date", "2026-10-20") },
			matchInfo{}},
		{"club without date is not confirmed", func(_ *testing.T, _ core.App, m *core.Record) { m.Set("club", "Padel 360") },
			matchInfo{}},
		{"proposal prefers the venue name", func(t *testing.T, app core.App, m *core.Record) {
			proposeSchedule(t, app, m, `{"date":"2026-11-02","time":"19:00","venue_name":"Wurko","venue_text":"otro"}`, "pending")
		}, matchInfo{Status: scheduleProposed, When: "02/11/2026 19:00", Place: "Wurko"}},
		{"proposal without a date shows nothing", func(t *testing.T, app core.App, m *core.Record) {
			proposeSchedule(t, app, m, `{"time":"19:00","venue_text":"Wurko"}`, "pending")
		}, matchInfo{}},
		{"accepted proposal is not pending", func(t *testing.T, app core.App, m *core.Record) {
			proposeSchedule(t, app, m, `{"date":"2026-11-02","venue_text":"Wurko"}`, "accepted")
		}, matchInfo{}},
		{"confirmed date wins over a pending proposal", func(t *testing.T, app core.App, m *core.Record) {
			confirm(m, "", "18:30")
			proposeSchedule(t, app, m, `{"date":"2026-11-02","venue_text":"Wurko"}`, "pending")
		}, matchInfo{Status: scheduleConfirmed, When: "20/10/2026 18:30", Place: "Padel 360"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app := newTestApp(t)
			match := makeMatch(t, app)
			tt.setup(t, app, match)
			require.NoError(t, app.Save(match))
			info, ok := loadMatchInfo(app, match.Id)
			require.True(t, ok)
			assert.Equal(t, tt.want.Status, info.Status)
			assert.Equal(t, tt.want.When, info.When)
			assert.Equal(t, tt.want.Place, info.Place)
		})
	}
}

func TestLoadMatchInfo_NotFoundAndUnknownPair(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, ok := loadMatchInfo(app, "missing")
	assert.False(t, ok)
	assert.Equal(t, matchPair{Name: "Pareja desconocida"}, loadMatchPair(app, "missing"))
}

func TestMessageHTML(t *testing.T) {
	t.Parallel()
	chat := messageHTML(emailContent{Author: "Ana <b> (Pareja X)", Text: "hola <i>"})
	assert.Contains(t, chat, "Ana &lt;b&gt; · Pareja X escribió:")
	assert.Contains(t, chat, "hola &lt;i&gt;")
	assert.NotContains(t, chat, "<i>")
	assert.Contains(t, messageHTML(emailContent{Author: "Ana", Text: "x"}), "Ana escribió:")
	plain := messageHTML(emailContent{Body: "a <b>", Author: "Ana"})
	assert.Contains(t, plain, "a &lt;b&gt;")
	assert.NotContains(t, plain, "escribió")
}

func TestBuildNotificationEmail_LinkLabels(t *testing.T) {
	t.Parallel()
	tests := []struct{ link, want string }{
		{"https://x.test/match/1", "Ver partido"},
		{"https://x.test/competition/1", "Ver competición"},
		{"https://x.test/notifications", "Abrir"},
	}
	for _, tt := range tests {
		html := buildNotificationEmail("Ana", emailContent{Body: "b"}, tt.link)
		assert.Contains(t, html, ">"+tt.want+"<", tt.link)
		for _, other := range []string{"Ver partido", "Ver competición", "Abrir"} {
			if other != tt.want {
				assert.NotContains(t, html, ">"+other+"<", tt.link)
			}
		}
	}
}

func TestPairBlockHTML_PlayersLineOnlyWhenKnown(t *testing.T) {
	t.Parallel()
	assert.NotContains(t, pairBlockHTML(matchPair{Name: "A"}), "font-size:13px")
	assert.Contains(t, pairBlockHTML(matchPair{Name: "A", Players: "x / y"}), "x / y")
}

func TestEmail_RelativeLinkGetsTheAppURL(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	app.Settings().Meta.AppURL = "https://liga.test/"
	html := capture(t, app, league.Notification{Type: "general", Title: "T", Body: "b", Link: "/match/abc"})
	assert.Contains(t, html, `href="https://liga.test/match/abc"`)
}
