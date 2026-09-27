package handlers_test

import (
	"fmt"
	"net/http"
	"padelleague/handlers"
	"padelleague/internal/testapp"
	"padelleague/league"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatusClass(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status string
		want   string
	}{
		{league.StatusPending, "badge-ghost"},
		{league.StatusConfirmed, "badge-warning"},
		{league.StatusDisputed, "badge-error"},
		{league.StatusFinal, "badge-success"},
		{"unknown", "badge-ghost"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			assert.Equal(t, tt.want, handlers.StatusClass(tt.status))
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════
// AdminOverride: score on match with no prior score (lines 319-320)
// ═══════════════════════════════════════════════════════════════════════

func TestAdminOverrideNewScore(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /match/{id}/admin-override sets new score",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, p1p1ID, p2p1ID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := handlers.MakeAdminUserTB(tb, app)
		p1 := handlers.MakePairTB(tb, app, "AO A")
		p2 := handlers.MakePairTB(tb, app, "AO B")
		p1p1ID = p1.GetString("player1")
		p2p1ID = p2.GetString("player1")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = m.Id
		s.URL = "/match/" + m.Id + "/admin-override"
		s.Body = strings.NewReader("scores=6-3+6-4")
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "6-3 6-4", m.GetString("scores"))
		assert.Equal(tb, "final", m.GetString("status"))

		msgs, _ := app.FindRecordsByFilter("match_messages",
			"match = {:id} && type = 'admin_action'", "", 0, 0,
			map[string]any{"id": matchID})
		require.GreaterOrEqual(tb, len(msgs), 1)
		found := false
		for _, msg := range msgs {
			if strings.Contains(msg.GetString("content"), "Resultado establecido") {
				found = true
			}
		}
		assert.True(tb, found, "timeline must contain 'Resultado establecido'")

		want := league.Notification{
			Type:     "general",
			Title:    "Corrección de administrador",
			Body:     "Resultado establecido: 6-3 6-4",
			MatchID:  matchID,
			CompName: "Test Competition",
		}
		assertNotified(tb, app, p1p1ID, want)
		assertNotified(tb, app, p2p1ID, want)
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) })
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// AdminOverride: score correction (existing score → new score) (line 319 negation)
// ═══════════════════════════════════════════════════════════════════════

func TestAdminOverrideCorrectedScore(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /match/{id}/admin-override corrects existing score",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := handlers.MakeAdminUserTB(tb, app)
		p1 := handlers.MakePairTB(tb, app, "AC A")
		p2 := handlers.MakePairTB(tb, app, "AC B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")
		m.Set("scores", "6-3 6-4")
		m.Set("winner", p1.Id)
		require.NoError(tb, app.Save(m))
		matchID = m.Id
		s.URL = "/match/" + m.Id + "/admin-override"
		s.Body = strings.NewReader("scores=6-4+6-3")
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		msgs, _ := app.FindRecordsByFilter("match_messages",
			"match = {:id} && type = 'admin_action'", "", 0, 0,
			map[string]any{"id": matchID})
		require.GreaterOrEqual(tb, len(msgs), 1)
		found := false
		for _, msg := range msgs {
			if strings.Contains(msg.GetString("content"), "Resultado corregido") {
				found = true
			}
		}
		assert.True(tb, found, "timeline must contain 'Resultado corregido'")
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) })
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// AdminOverride: venue set on match with no prior venue (lines 350-351)
// ═══════════════════════════════════════════════════════════════════════

func TestAdminOverrideNewVenue(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /match/{id}/admin-override sets new venue",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := handlers.MakeAdminUserTB(tb, app)
		p1 := handlers.MakePairTB(tb, app, "AV A")
		p2 := handlers.MakePairTB(tb, app, "AV B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = m.Id
		require.NoError(tb, app.Save(m))
		insertMatchReminder(tb, app, m.Id, p1.GetString("player1"))
		venue := handlers.MakeVenueTB(tb, app, "Padel Test")
		s.URL = "/match/" + m.Id + "/admin-override"
		s.Body = strings.NewReader("venue_id=" + venue.Id)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "Padel Test", m.GetString("club"))
		rows, _ := app.FindRecordsByFilter("match_reminders", "match = {:mid}", "", 0, 0, map[string]any{"mid": matchID})
		assert.Len(tb, rows, 1, "a venue-only change must not clear match reminders — the date didn't change")

		msgs, _ := app.FindRecordsByFilter("match_messages",
			"match = {:id} && type = 'admin_action'", "", 0, 0,
			map[string]any{"id": matchID})
		require.GreaterOrEqual(tb, len(msgs), 1)
		found := false
		for _, msg := range msgs {
			if strings.Contains(msg.GetString("content"), "Club establecido") {
				found = true
			}
		}
		assert.True(tb, found, "timeline must contain 'Club establecido'")
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) })
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// AdminOverride: date set on match with no prior date (line 331)
// ═══════════════════════════════════════════════════════════════════════

func TestAdminOverrideNewDate(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /match/{id}/admin-override sets new date",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := handlers.MakeAdminUserTB(tb, app)
		p1 := handlers.MakePairTB(tb, app, "AD A")
		p2 := handlers.MakePairTB(tb, app, "AD B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = m.Id
		s.URL = "/match/" + m.Id + "/admin-override"
		s.Body = strings.NewReader("date=2026-09-01")
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		msgs, _ := app.FindRecordsByFilter("match_messages",
			"match = {:id} && type = 'admin_action'", "", 0, 0,
			map[string]any{"id": matchID})
		require.GreaterOrEqual(tb, len(msgs), 1)
		found := false
		for _, msg := range msgs {
			if strings.Contains(msg.GetString("content"), "Fecha establecida") {
				found = true
			}
		}
		assert.True(tb, found, "timeline must contain 'Fecha establecida'")
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) })
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// handlers.BuildShareText: final match shows correct winner (lines 364, 371)
// ═══════════════════════════════════════════════════════════════════════

func TestBuildShareTextFinalMatch(t *testing.T) {
	t.Parallel()
	app := testapp.New(t)

	p1 := handlers.MakePairTB(t, app, "Pair Alpha")
	p2 := handlers.MakePairTB(t, app, "Pair Beta")
	comp := handlers.MakeCompetitionTB(t, app, "league", []*core.Record{p1, p2})

	t.Run("pair1 wins", func(t *testing.T) {
		m := handlers.MakeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "final")
		m.Set("scores", "6-3 6-4")
		m.Set("winner", p1.Id)
		require.NoError(t, app.Save(m))
		text, shareURL := handlers.BuildShareText(app, m, "https://example.com", "/match/"+m.Id)
		assert.NotEmpty(t, text)
		assert.Contains(t, text, "Pair+Alpha")
		assert.Contains(t, text, "Ganador")
		assert.Contains(t, text, "Pair+Alpha%21")
		assert.Contains(t, text, "https%3A%2F%2Fexample.com%2Fmatch%2F"+m.Id)
		assert.Equal(t, "https://example.com/match/"+m.Id, shareURL)
	})

	t.Run("pair2 wins", func(t *testing.T) {
		m := handlers.MakeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "final")
		m.Set("scores", "3-6 4-6")
		m.Set("winner", p2.Id)
		require.NoError(t, app.Save(m))
		text, shareURL := handlers.BuildShareText(app, m, "https://example.com", "/match/"+m.Id)
		assert.Contains(t, text, "Pair+Beta%21")
		assert.NotContains(t, text, "Pair+Alpha%21")
		assert.Equal(t, "https://example.com/match/"+m.Id, shareURL)
	})

	t.Run("non-final returns empty", func(t *testing.T) {
		m := handlers.MakeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "pending")
		text, shareURL := handlers.BuildShareText(app, m, "https://example.com", "/match/"+m.Id)
		assert.Empty(t, text)
		assert.Empty(t, shareURL)
	})
}

// ═══════════════════════════════════════════════════════════════════════
// MatchSubmit: rival notification goes to correct team (line 226)
// ═══════════════════════════════════════════════════════════════════════

func TestMatchSubmitNotifiesRival(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /match/{id}/submit notifies opponent pair",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var pair2Player1ID, pair2Player2ID, pair1Player1ID, pair1Player2ID, matchID, adminID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := handlers.MakeAdminUserTB(tb, app)
		adminID = admin.Id
		p1 := handlers.MakePairTB(tb, app, "Sub A")
		p2 := handlers.MakePairTB(tb, app, "Sub B")
		pair1Player1ID = p1.GetString("player1")
		pair1Player2ID = p1.GetString("player2")
		pair2Player1ID = p2.GetString("player1")
		pair2Player2ID = p2.GetString("player2")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		m.Set("date", "2026-09-01")
		m.Set("club", "Padel 360")
		require.NoError(tb, app.Save(m))
		matchID = m.Id

		submitter, err := app.FindRecordById("users", pair1Player1ID)
		require.NoError(tb, err)
		s.URL = "/match/" + m.Id + "/submit"
		s.Body = strings.NewReader("scores=6-3+6-4")
		hdrs := handlers.AuthHeaders(tb, submitter)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		want := league.Notification{
			Type:     "quorum_request",
			Title:    "Resultado enviado",
			Body:     "Sub A P1 (Sub A) ha enviado 6-3 6-4. Confirma o contrapropón.",
			MatchID:  matchID,
			CompName: "Test Competition",
		}
		assertNotified(tb, app, pair2Player1ID, want)
		assertNotified(tb, app, pair2Player2ID, want)
		assertNotNotified(tb, app, pair1Player1ID, want.Title)
		assertNotNotified(tb, app, pair1Player2ID, want.Title)

		adminWant := league.Notification{
			Type:    "match_progress",
			Title:   "Progreso de partido",
			Body:    "Resultado propuesto: 6-3 6-4",
			MatchID: matchID,
		}
		assertNotified(tb, app, adminID, adminWant)
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) })
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// MatchDetail: competition name shown (lines 156, 158)
// ═══════════════════════════════════════════════════════════════════════

func TestMatchDetailShowsCompName(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /match/{id} shows competition name",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Liga Visible"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := handlers.MakeAdminUserTB(tb, app)
		p1 := handlers.MakePairTB(tb, app, "MD A")
		p2 := handlers.MakePairTB(tb, app, "MD B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("name", "Liga Visible")
		require.NoError(tb, app.Save(comp))
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + m.Id
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		comps, err := app.FindRecordsByFilter("competitions", "name = 'Liga Visible'", "", 1, 0, nil)
		require.NoError(tb, err)
		require.Len(tb, comps, 1)
		assert.Contains(tb, body, `href="/competition/`+comps[0].Id+`"`, "breadcrumb competition name must link via competitionIdentity")
	}
	s.Test(t)
}

func TestMatchDetail_DraftCalendarReturns404ForPlayer(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /match/{id} returns 404 for a non-admin when the competition's calendar is draft",
		Method:          http.MethodGet,
		ExpectedStatus:  404,
		ExpectedContent: []string{"Partido no encontrado"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "DraftMatch A")
		p2 := handlers.MakePairTB(tb, app, "DraftMatch B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("calendar_status", "draft")
		require.NoError(tb, app.Save(comp))
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + m.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestMatchDetail_DraftCalendarStillVisibleToAdmin(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /match/{id} stays visible to an admin when the competition's calendar is draft",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"DraftAdmin"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := handlers.MakeAdminUserTB(tb, app)
		p1 := handlers.MakePairTB(tb, app, "DraftAdmin A")
		p2 := handlers.MakePairTB(tb, app, "DraftAdmin B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("calendar_status", "draft")
		require.NoError(tb, app.Save(comp))
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + m.Id
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.Test(t)
}

func TestMatchDetail_OGImageDefaultsToIcon(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /match/{id} without a competition logo falls back to the default icon for og:image",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{`og:image" content="http://`, `/logo/league"`},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := handlers.MakeAdminUserTB(tb, app)
		p1 := handlers.MakePairTB(tb, app, "OG A")
		p2 := handlers.MakePairTB(tb, app, "OG B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + m.Id
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.Test(t)
}

func TestMatchDetail_ShowsPrecedentesStrip(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /match/{id} shows the precedentes strip from a prior meeting",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Precedentes", "6-3 6-4"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := handlers.MakeAdminUserTB(tb, app)
		p1 := handlers.MakePairTB(tb, app, "Prec A")
		p2 := handlers.MakePairTB(tb, app, "Prec B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		// First meeting: finalized, p1 wins.
		past := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")
		past.Set("scores", "6-3 6-4")
		past.Set("winner", p1.Id)
		require.NoError(tb, app.Save(past))

		// Second match: the one whose page we view.
		current := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		s.URL = "/match/" + current.Id
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.Test(t)
}

func TestMatchDetail_NoPriorMeetings_HidesStrip(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /match/{id} hides the precedentes strip when the pairs never met before",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"NoPrec A"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := handlers.MakeAdminUserTB(tb, app)
		p1 := handlers.MakePairTB(tb, app, "NoPrec A")
		p2 := handlers.MakePairTB(tb, app, "NoPrec B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + m.Id
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, resp *http.Response) {
		body := handlers.ReadBody(tb, resp)
		assert.NotContains(tb, body, "Precedentes")
	}
	s.Test(t)
}

func TestMatchDetailAdminShowsResolveForm(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /match/{id}/thread admin view shows resolve form for disputed match",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Resolver"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "Res A")
		p2 := handlers.MakePairTB(tb, app, "Res B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		m.Set("scores", "6-3 6-4")
		m.Set("submitted_by", p1.GetString("player1"))
		m.Set("disputed_by", p2.GetString("player1"))
		m.Set("disputed_scores", "6-4 6-3")
		require.NoError(tb, app.Save(m))
		// The result surface (resolve form) now lives in the thread fragment.
		s.URL = "/match/" + m.Id + "/thread"
		s.ExpectedContent = append(s.ExpectedContent, `hx-post="/admin/disputes/`+m.Id+`/resolve"`)
		admin := handlers.MakeAdminUserTB(tb, app)
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// handlers.PlayerNameIfSet: empty returns empty, non-empty returns name (line 357)
// ═══════════════════════════════════════════════════════════════════════

func TestPlayerNameIfSet(t *testing.T) {
	t.Parallel()
	app := testapp.New(t)

	t.Run("empty returns empty", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "", handlers.PlayerNameIfSet(app, ""))
	})

	t.Run("valid user returns name", func(t *testing.T) {
		t.Parallel()
		u := handlers.MakeUserTB(t, app, "NameTest", "nametest@test.local")
		name := handlers.PlayerNameIfSet(app, u.Id)
		assert.Equal(t, "NameTest", name)
	})
}

// HTML markers rendered when each flag is true.
const (
	markerCanSubmit             = "Registrar resultado"
	markerCanRequestArbitration = "Solicitar arbitraje"
	markerCanCorrect            = "Corregir resultado"
	markerDateGate              = "Primero propón una fecha y lugar"
)

type matchViewCase struct {
	name            string
	status          string
	viewer          string // "submitter", "opponent", "outsider", "admin"
	submitted       bool
	recentSubmit    bool
	hasDate         bool
	openArbitration bool
	httpStatus      int // 0 means 200
	want            []string
	deny            []string
}

func TestBuildMatchViewFlags(t *testing.T) {
	t.Parallel()
	cases := []matchViewCase{
		// Pending without date → submit gated; arbitration doesn't need a
		// date (it also covers "we never agreed a date" scenarios).
		{
			name: "pending/no-date/submitter", status: "pending", viewer: "submitter",
			want: []string{markerDateGate, markerCanRequestArbitration},
			deny: []string{markerCanSubmit, markerCanCorrect},
		},
		// Pending with date+place → submit visible
		{
			name: "pending/with-date/submitter", status: "pending", viewer: "submitter",
			hasDate: true,
			want:    []string{markerCanSubmit, markerCanRequestArbitration},
			deny:    []string{markerCanCorrect, markerDateGate},
		},
		{
			name: "pending/with-date/opponent", status: "pending", viewer: "opponent",
			hasDate: true,
			want:    []string{markerCanSubmit, markerCanRequestArbitration},
			deny:    []string{markerCanCorrect, markerDateGate},
		},
		{
			name: "scheduled/submitter-team", status: "scheduled", viewer: "submitter",
			hasDate: true,
			want:    []string{markerCanSubmit, markerCanRequestArbitration},
			deny:    []string{markerCanCorrect},
		},
		{
			name: "pending/outsider", status: "pending", viewer: "outsider",
			deny: []string{markerCanSubmit, markerCanRequestArbitration, markerCanCorrect},
		},
		{
			name: "pending/admin", status: "pending", viewer: "admin",
			deny: []string{markerCanSubmit, markerCanRequestArbitration, markerCanCorrect},
		},

		// Confirmed with submitter set (legacy status — no confirm/dispute buttons)
		{
			name: "confirmed/submitter/recent", status: "confirmed", viewer: "submitter",
			submitted: true, recentSubmit: true, hasDate: true,
			want: []string{markerCanCorrect, markerCanRequestArbitration},
			deny: []string{markerCanSubmit},
		},
		{
			name: "confirmed/submitter/expired", status: "confirmed", viewer: "submitter",
			submitted: true, recentSubmit: false, hasDate: true,
			want: []string{markerCanRequestArbitration},
			deny: []string{markerCanSubmit, markerCanCorrect},
		},
		{
			name: "confirmed/opponent", status: "confirmed", viewer: "opponent",
			submitted: true, hasDate: true,
			want: []string{markerCanRequestArbitration},
			deny: []string{markerCanSubmit, markerCanCorrect},
		},
		{
			name: "confirmed/outsider", status: "confirmed", viewer: "outsider",
			submitted: true, hasDate: true,
			deny: []string{markerCanSubmit, markerCanCorrect},
		},
		{
			name: "confirmed/admin-nonparticipant", status: "confirmed", viewer: "admin",
			submitted: true, hasDate: true,
			deny: []string{markerCanSubmit, markerCanCorrect, markerCanRequestArbitration},
		},
		{
			name: "confirmed/no-submitter/opponent", status: "confirmed", viewer: "opponent",
			submitted: false, hasDate: true,
			want: []string{markerCanRequestArbitration},
			deny: []string{markerCanSubmit, markerCanCorrect},
		},
		{
			name: "confirmed/no-date/opponent", status: "confirmed", viewer: "opponent",
			submitted: true,
			want:      []string{markerCanRequestArbitration},
			deny:      []string{markerCanSubmit, markerCanCorrect},
		},

		// Disputed — arbitration is already open (that's how the match got
		// here), so the request button is gone; only the admin/deadlock
		// resolution UI shows, not a fresh arbitration request.
		{
			name: "disputed/submitter", status: "disputed", viewer: "submitter",
			submitted: true, openArbitration: true,
			deny: []string{markerCanSubmit, markerCanCorrect, markerCanRequestArbitration},
		},
		{
			name: "disputed/opponent", status: "disputed", viewer: "opponent",
			submitted: true, openArbitration: true,
			deny: []string{markerCanSubmit, markerCanCorrect, markerCanRequestArbitration},
		},

		// Final
		{
			name: "final/submitter", status: "final", viewer: "submitter",
			submitted: true,
			deny:      []string{markerCanSubmit, markerCanCorrect, markerCanRequestArbitration},
		},
		{
			name: "final/opponent", status: "final", viewer: "opponent",
			submitted: true,
			deny:      []string{markerCanSubmit, markerCanCorrect, markerCanRequestArbitration},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expectedStatus := tc.httpStatus
			if expectedStatus == 0 {
				expectedStatus = 200
			}
			s := &tests.ApiScenario{
				TestAppFactory:     handlers.TestAppFactory,
				Name:               tc.name,
				Method:             http.MethodGet,
				ExpectedStatus:     expectedStatus,
				ExpectedContent:    tc.want,
				NotExpectedContent: tc.deny,
			}

			s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				setupProductionRoutes(tb, app, e)

				p1 := handlers.MakePairTB(tb, app, fmt.Sprintf("MV%s A", tc.name[:3]))
				p2 := handlers.MakePairTB(tb, app, fmt.Sprintf("MV%s B", tc.name[:3]))
				comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
				match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, tc.status)

				submitterUserID := p1.GetString("player1")

				if tc.submitted {
					match.Set("scores", "6-3 6-4")
					match.Set("submitted_by", submitterUserID)
					if tc.recentSubmit {
						match.SetRaw("submitted_at", time.Now().Add(-1*time.Hour).UTC().Format(time.RFC3339))
					} else {
						match.SetRaw("submitted_at", time.Now().Add(-25*time.Hour).UTC().Format(time.RFC3339))
					}
				}

				if tc.hasDate {
					match.Set("date", "2099-06-15")
					match.Set("club", "Padel 360")
				}

				// Final status needs a winner to render properly
				if tc.status == "final" {
					match.Set("winner", p1.Id)
					match.Set("scores", "6-3 6-4")
				}

				if tc.openArbitration {
					match.Set("arbitration", "result")
					match.Set("arbitration_by", submitterUserID)
				}

				require.NoError(tb, app.Save(match))

				// Access-control cases (non-200) test the page guard; content cases
				// test the thread fragment where the single result panel renders.
				if tc.httpStatus != 0 && tc.httpStatus != 200 {
					s.URL = "/match/" + match.Id
				} else {
					s.URL = "/match/" + match.Id + "/thread"
				}

				var viewerUser *core.Record
				switch tc.viewer {
				case "submitter":
					viewerUser, _ = app.FindRecordById("users", submitterUserID)
				case "opponent":
					opponentID := p2.GetString("player1")
					viewerUser, _ = app.FindRecordById("users", opponentID)
				case "outsider":
					viewerUser = handlers.MakeUserTB(tb, app, "MV Outsider", "")
				case "admin":
					viewerUser = handlers.MakeAdminUserTB(tb, app)
				}
				s.Headers = handlers.AuthHeaders(tb, viewerUser)
			}

			s.Test(t)
		})
	}
}

func TestAdminOverride(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /match/{id}/admin-override changes score and finalizes",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, p1ID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "Override A")
		p2 := handlers.MakePairTB(tb, app, "Override B")
		p1ID = p1.Id
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		matchID = m.Id
		m.Set("scores", "6-3 6-4")
		m.Set("submitted_by", p1.GetString("player1"))
		require.NoError(tb, app.Save(m))

		s.URL = "/match/" + m.Id + "/admin-override"
		s.Body = strings.NewReader("scores=6-4+6-3&dispute_notes=Corregido+por+admin")
		admin := handlers.MakeAdminUserTB(tb, app)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "final", m.GetString("status"))
		assert.Equal(tb, "6-4 6-3", m.GetString("scores"))
		assert.Equal(tb, p1ID, m.GetString("winner"))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) })
	s.Test(t)
}

func TestAdminOverrideWithDateChange(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /match/{id}/admin-override changes date and time",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "OvrDate A")
		p2 := handlers.MakePairTB(tb, app, "OvrDate B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = m.Id
		m.Set("date", "2026-09-01")
		m.Set("time", "18:00")
		m.Set("club", "Old Club")
		require.NoError(tb, app.Save(m))
		insertMatchReminder(tb, app, m.Id, p1.GetString("player1"))

		venue := handlers.MakeVenueTB(tb, app, "New Club")

		s.URL = "/match/" + m.Id + "/admin-override"
		s.Body = strings.NewReader("date=2026-09-15&time=20:00&venue_id=" + venue.Id)
		admin := handlers.MakeAdminUserTB(tb, app)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "20:00", m.GetString("time"))
		assert.Equal(tb, "New Club", m.GetString("club"))
		rows, _ := app.FindRecordsByFilter("match_reminders", "match = {:mid}", "", 0, 0, map[string]any{"mid": matchID})
		assert.Empty(tb, rows, "changing the date must clear match reminders")
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) })
	s.Test(t)
}

// AdminOverride: posting a prefilled date equal to the stored value must not
// produce a spurious "Fecha cambiada" entry — only actual changes appear.
// This pins the B9 handlers.DetectFieldChange normalization fix.
func TestAdminOverridePrefillDateNoSpuriousChange(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /match/{id}/admin-override with same date produces exactly one change entry",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "PF A")
		p2 := handlers.MakePairTB(tb, app, "PF B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = m.Id
		// Store an existing date and club
		m.Set("date", "2026-11-01")
		m.Set("club", "Padel 360")
		require.NoError(tb, app.Save(m))

		// Post with the same date (as a prefill would) but a different club
		venue := handlers.MakeVenueTB(tb, app, "Wurko")
		s.URL = "/match/" + m.Id + "/admin-override"
		s.Body = strings.NewReader("date=2026-11-01&venue_id=" + venue.Id)
		admin := handlers.MakeAdminUserTB(tb, app)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		msgs, _ := app.FindRecordsByFilter("match_messages",
			"match = {:id} && type = 'admin_action'", "", 0, 0,
			map[string]any{"id": matchID})
		require.Len(tb, msgs, 1, "exactly one admin_action entry expected")
		content := msgs[0].GetString("content")
		// The venue changed — that entry must be present
		assert.Contains(tb, content, "Club")
		// The date did NOT change — must not appear as a change
		assert.NotContains(tb, content, "Fecha", "same date must not produce a Fecha change entry")
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) })
	s.Test(t)
}

func TestAdminOverrideNoChanges(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /match/{id}/admin-override with no changes warns",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"alert-warning"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "OvrNone A")
		p2 := handlers.MakePairTB(tb, app, "OvrNone B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		s.URL = "/match/" + m.Id + "/admin-override"
		s.Body = strings.NewReader("")
		admin := handlers.MakeAdminUserTB(tb, app)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestAdminOverrideNonAdmin(t *testing.T) {
	t.Parallel()
	// RequireAppAdmin redirects non-admins to "/" — a plain 302 for a regular
	// request, or 204 + HX-Redirect for HTMX (middleware.redirectOrHX).
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /match/{id}/admin-override as player redirects home",
		Method:         http.MethodPost,
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "OvrNoAdm A")
		p2 := handlers.MakePairTB(tb, app, "OvrNoAdm B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		s.URL = "/match/" + m.Id + "/admin-override"
		s.Body = strings.NewReader("scores=6-3+6-4")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := handlers.AuthHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/", res.Header.Get("Location"))
	}
	s.Test(t)
}

func TestAdminOverrideNonAdmin_HXRedirect(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /match/{id}/admin-override as player, HTMX request gets HX-Redirect",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "OvrNoAdmHX A")
		p2 := handlers.MakePairTB(tb, app, "OvrNoAdmHX B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		s.URL = "/match/" + m.Id + "/admin-override"
		s.Body = strings.NewReader("scores=6-3+6-4")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := handlers.AuthHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		hdrs["HX-Request"] = "true"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/", res.Header.Get("HX-Redirect"))
	}
	s.Test(t)
}

func TestMatchSubmitAlreadyScored(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /match/{id}/submit on non-pending fails",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"ya tiene"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "SubDup A")
		p2 := handlers.MakePairTB(tb, app, "SubDup B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")

		s.URL = "/match/" + m.Id + "/submit"
		s.Body = strings.NewReader("scores=6-3+6-4")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := handlers.AuthHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestMatchSubmitRejectsWithoutDateOrPlace(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /match/{id}/submit rejects when no date/place agreed",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Primero acuerda"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "NoDate A")
		p2 := handlers.MakePairTB(tb, app, "NoDate B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		s.URL = "/match/" + m.Id + "/submit"
		s.Body = strings.NewReader("scores=6-3+6-4")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := handlers.AuthHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

// R-5: an admin must not be able to submit a score for a playoff match whose
// pairs are not yet assigned (round 2+ before the previous round finishes).
func TestMatchSubmitRejectsWithoutBothPairs(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /match/{id}/submit rejects when pairs are not assigned",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"parejas asignadas"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "Empty A")
		p2 := handlers.MakePairTB(tb, app, "Empty B")
		comp := handlers.MakeCompetitionTB(tb, app, "playoff", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, "", "", "pending")
		m.Set("date", "2026-09-01")
		m.Set("club", "Padel 360")
		require.NoError(tb, app.Save(m))

		s.URL = "/match/" + m.Id + "/submit"
		s.Body = strings.NewReader("scores=6-3+6-4")
		admin := handlers.MakeAdminUserTB(tb, app)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestAdminOverrideCourtNumber(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /match/{id}/admin-override sets court_number",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "Court A")
		p2 := handlers.MakePairTB(tb, app, "Court B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = m.Id

		s.URL = "/match/" + m.Id + "/admin-override"
		s.Body = strings.NewReader("court_number=5")
		admin := handlers.MakeAdminUserTB(tb, app)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "5", m.GetString("court_number"))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) })
	s.Test(t)
}

func TestPlayerSubmitBlockedOnFinalizedComp(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /match/{id}/submit blocked for player on finalized competition",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"finalizada o archivada"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "BF A")
		p2 := handlers.MakePairTB(tb, app, "BF B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("finalized", true)
		require.NoError(tb, app.Save(comp))
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + m.Id + "/submit"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := handlers.AuthHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
		s.Body = strings.NewReader("scores=6-3+6-4")
	}
	s.Test(t)
}

func TestPlayerSubmitBlockedOnInactiveComp(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /match/{id}/submit blocked for player on inactive competition",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"finalizada o archivada"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "BI A")
		p2 := handlers.MakePairTB(tb, app, "BI B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("active", false)
		require.NoError(tb, app.Save(comp))
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + m.Id + "/submit"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := handlers.AuthHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
		s.Body = strings.NewReader("scores=6-3+6-4")
	}
	s.Test(t)
}

func TestAdminSubmitAllowedOnFinalizedComp(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /match/{id}/admin-override allowed on finalized competition",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := handlers.MakeAdminUserTB(tb, app)
		p1 := handlers.MakePairTB(tb, app, "AF A")
		p2 := handlers.MakePairTB(tb, app, "AF B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("finalized", true)
		require.NoError(tb, app.Save(comp))
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + m.Id + "/admin-override"
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
		s.Body = strings.NewReader("scores=6-3+6-4")
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) })
	s.Test(t)
}

func TestReadOnlyCompGuard_AllHandlers(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status string
		path   string
		body   string
	}{
		{"correct", "scheduled", "/correct", "scores=6-4+6-3"},
		{"arbitration", "pending", "/arbitration", "category=other&notes=test"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := &tests.ApiScenario{
				TestAppFactory:  handlers.TestAppFactory,
				Name:            "player " + tc.name + " blocked on finalized comp",
				Method:          http.MethodPost,
				ExpectedStatus:  200,
				ExpectedContent: []string{"finalizada o archivada"},
			}
			s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				setupProductionRoutes(tb, app, e)
				p1 := handlers.MakePairTB(tb, app, "RO-"+tc.name+" A")
				p2 := handlers.MakePairTB(tb, app, "RO-"+tc.name+" B")
				comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
				comp.Set("finalized", true)
				require.NoError(tb, app.Save(comp))
				m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, tc.status)
				if tc.name == "correct" {
					m.Set("submitted_by", p1.GetString("player1"))
					m.Set("submitted_at", time.Now().UTC().Format(time.RFC3339))
					require.NoError(tb, app.Save(m))
					handlers.MakeResultProposal(tb, app, m.Id, p1.GetString("player1"), "6-3 6-4")
				}
				s.URL = "/match/" + m.Id + tc.path
				user, _ := app.FindRecordById("users", p2.GetString("player1"))
				hdrs := handlers.AuthHeaders(tb, user)
				hdrs["Content-Type"] = "application/x-www-form-urlencoded"
				s.Headers = hdrs
				if tc.body != "" {
					s.Body = strings.NewReader(tc.body)
				}
			}
			s.Test(t)
		})
	}
}

func TestReadOnlyCompGuard_ThreadHandlers(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		path string
		body string
	}{
		{"respond-proposal", "/thread/proposal/%s/respond", "decision=accepted"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := &tests.ApiScenario{
				TestAppFactory:  handlers.TestAppFactory,
				Name:            "player " + tc.name + " blocked on finalized comp",
				Method:          http.MethodPost,
				ExpectedStatus:  200,
				ExpectedContent: []string{"finalizada o archivada"},
			}
			s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				setupProductionRoutes(tb, app, e)
				p1 := handlers.MakePairTB(tb, app, "RO-"+tc.name+" A")
				p2 := handlers.MakePairTB(tb, app, "RO-"+tc.name+" B")
				comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
				comp.Set("finalized", true)
				require.NoError(tb, app.Save(comp))
				m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
				msgCol, _ := app.FindCollectionByNameOrId("match_messages")
				msg := core.NewRecord(msgCol)
				msg.Set("match", m.Id)
				msg.Set("author", p1.GetString("player1"))
				msg.Set("type", "scheduling_proposal")
				msg.Set("proposal_data", `{"date":"2026-10-01","time":"20:00","venue_id":"v1","venue_name":"Test"}`)
				msg.Set("proposal_status", "pending")
				require.NoError(tb, app.Save(msg))
				s.URL = "/match/" + m.Id + fmt.Sprintf(tc.path, msg.Id)
				user, _ := app.FindRecordById("users", p2.GetString("player1"))
				hdrs := handlers.AuthHeaders(tb, user)
				hdrs["Content-Type"] = "application/x-www-form-urlencoded"
				s.Headers = hdrs
				if tc.body != "" {
					s.Body = strings.NewReader(tc.body)
				}
			}
			s.Test(t)
		})
	}
}

func TestMatchSubmitCreatesResultProposal(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /match/{id}/submit creates result_submission proposal",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, submitterID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "RP A")
		p2 := handlers.MakePairTB(tb, app, "RP B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
		m.Set("date", "2026-09-01")
		m.Set("club", "Padel 360")
		require.NoError(tb, app.Save(m))

		submitter, err := app.FindRecordById("users", p1.GetString("player1"))
		require.NoError(tb, err)
		submitterID = submitter.Id
		matchID = m.Id
		s.URL = "/match/" + m.Id + "/submit"
		s.Body = strings.NewReader("scores=6-3+6-4")
		hdrs := handlers.AuthHeaders(tb, submitter)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "scheduled", m.GetString("status"),
			"match must stay in pre-score status, not confirmed")
		assert.Equal(tb, submitterID, m.GetString("submitted_by"),
			"submitted_by must be set to the submitter")
		assert.NotEmpty(tb, m.GetString("submitted_at"),
			"submitted_at must be set")
		assert.False(tb, m.GetBool("confirm_reminded"),
			"confirm_reminded must be reset to false")

		proposals, err := app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'result_submission' && author = {:uid}",
			"-created", 0, 0,
			map[string]any{"mid": matchID, "uid": submitterID})
		require.NoError(tb, err)
		require.Len(tb, proposals, 1, "exactly one result_submission must exist")

		prop := proposals[0]
		assert.Equal(tb, "pending", prop.GetString("proposal_status"))
		pd := handlers.ParseProposalData(prop.GetString("proposal_data"))
		require.NotNil(tb, pd, "proposal_data must be parseable")
		assert.Equal(tb, "6-3 6-4", pd.Scores)
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) })
	s.Test(t)
}

func TestMatchSubmitSupersedesPreviousProposal(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /match/{id}/submit supersedes previous pending proposal from same pair",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, submitterID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "SS A")
		p2 := handlers.MakePairTB(tb, app, "SS B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
		m.Set("date", "2026-09-01")
		m.Set("club", "Padel 360")
		require.NoError(tb, app.Save(m))
		matchID = m.Id

		submitter, err := app.FindRecordById("users", p1.GetString("player1"))
		require.NoError(tb, err)
		submitterID = submitter.Id

		col, err := app.FindCollectionByNameOrId("match_messages")
		require.NoError(tb, err)
		old := core.NewRecord(col)
		old.Set("match", matchID)
		old.Set("author", submitterID)
		old.Set("type", "result_submission")
		old.Set("proposal_status", "pending")
		old.Set("proposal_data", `{"scores":"6-0 6-0"}`)
		old.Set("content", "6-0 6-0")
		require.NoError(tb, app.Save(old))

		s.URL = "/match/" + m.Id + "/submit"
		s.Body = strings.NewReader("scores=6-3+6-4")
		hdrs := handlers.AuthHeaders(tb, submitter)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		pending, err := app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'result_submission' && author = {:uid} && proposal_status = 'pending'",
			"", 0, 0,
			map[string]any{"mid": matchID, "uid": submitterID})
		require.NoError(tb, err)
		assert.Len(tb, pending, 1, "only one pending proposal must remain")
		assert.Equal(tb, "6-3 6-4", handlers.ParseProposalData(pending[0].GetString("proposal_data")).Scores)

		superseded, _ := app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'result_submission' && author = {:uid} && proposal_status = 'superseded'",
			"", 0, 0,
			map[string]any{"mid": matchID, "uid": submitterID})
		assert.Len(tb, superseded, 1, "old proposal must be superseded")
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) })
	s.Test(t)
}

func TestMatchSubmitRejectsWhenRivalHasPending(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /match/{id}/submit rejects when rival has pending result",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Ya hay una propuesta de resultado del rival pendiente"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "DL A")
		p2 := handlers.MakePairTB(tb, app, "DL B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
		m.Set("date", "2026-09-01")
		m.Set("club", "Padel 360")
		require.NoError(tb, app.Save(m))

		p2Player1, err := app.FindRecordById("users", p2.GetString("player1"))
		require.NoError(tb, err)

		col, err := app.FindCollectionByNameOrId("match_messages")
		require.NoError(tb, err)
		opposing := core.NewRecord(col)
		opposing.Set("match", m.Id)
		opposing.Set("author", p2Player1.Id)
		opposing.Set("type", "result_submission")
		opposing.Set("proposal_status", "pending")
		opposing.Set("proposal_data", `{"scores":"6-4 6-3"}`)
		opposing.Set("content", "6-4 6-3")
		require.NoError(tb, app.Save(opposing))

		submitter, err := app.FindRecordById("users", p1.GetString("player1"))
		require.NoError(tb, err)
		s.URL = "/match/" + m.Id + "/submit"
		s.Body = strings.NewReader("scores=6-3+6-4")
		hdrs := handlers.AuthHeaders(tb, submitter)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestMatchSubmitNoDeadlockNoAdminNotif(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /match/{id}/submit without opposing proposal sends no admin notification",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "NDL A")
		p2 := handlers.MakePairTB(tb, app, "NDL B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
		m.Set("date", "2026-09-01")
		m.Set("club", "Padel 360")
		require.NoError(tb, app.Save(m))

		handlers.MakeAdminUserTB(tb, app)

		submitter, err := app.FindRecordById("users", p1.GetString("player1"))
		require.NoError(tb, err)
		s.URL = "/match/" + m.Id + "/submit"
		s.Body = strings.NewReader("scores=6-3+6-4")
		hdrs := handlers.AuthHeaders(tb, submitter)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		admins, _ := app.FindRecordsByFilter("users", "roles ?~ 'admin'", "", 0, 0, nil)
		require.NotEmpty(tb, admins)
		notifs, _ := app.FindRecordsByFilter("notifications",
			"user = {:uid} && type = 'admin_message'", "", 0, 0,
			map[string]any{"uid": admins[0].Id})
		for _, n := range notifs {
			assert.NotContains(tb, strings.ToLower(n.GetString("title")), "discrepancia",
				"no deadlock notification expected when no opposing proposal exists")
		}
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) })
	s.Test(t)
}

func TestCancelDateAsParticipant(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /match/{id}/cancel-date reverts scheduled match to pending",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, rivalPlayerID, adminID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := handlers.MakeAdminUserTB(tb, app)
		adminID = admin.Id
		p1 := handlers.MakePairTB(tb, app, "CD A")
		p2 := handlers.MakePairTB(tb, app, "CD B")
		rivalPlayerID = p2.GetString("player1")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
		m.Set("date", "2026-12-01")
		m.Set("time", "18:00")
		m.Set("club", "Padel 360")
		require.NoError(tb, app.Save(m))
		matchID = m.Id

		canceller, err := app.FindRecordById("users", p1.GetString("player1"))
		require.NoError(tb, err)
		s.URL = "/match/" + m.Id + "/cancel-date"
		s.Body = strings.NewReader("reason=Viaje+de+trabajo")
		hdrs := handlers.AuthHeaders(tb, canceller)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "pending", m.GetString("status"))
		assert.Empty(tb, m.GetString("date"))
		assert.Empty(tb, m.GetString("time"))
		assert.Empty(tb, m.GetString("club"))
		assert.Equal(tb, 0, m.GetInt("last_warn_level"))

		msgs, _ := app.FindRecordsByFilter("match_messages",
			"match = {:id} && type = 'scheduling_response'", "", 0, 0,
			map[string]any{"id": matchID})
		require.GreaterOrEqual(tb, len(msgs), 1)
		assert.Contains(tb, msgs[0].GetString("content"), "canceló la fecha")

		wantRival := league.Notification{
			Type:     "scheduling",
			Title:    "Partido cancelado",
			Body:     "CD A P1 (CD A) ha cancelado la fecha: Viaje de trabajo",
			MatchID:  matchID,
			CompName: "Test Competition",
		}
		assertNotified(tb, app, rivalPlayerID, wantRival)

		wantAdmin := league.Notification{
			Type:     "dispute",
			Title:    "Cancelación de partido",
			Body:     "CD A vs CD B: CD A P1 (CD A) ha cancelado la fecha. Motivo: Viaje de trabajo",
			MatchID:  matchID,
			CompName: "Test Competition",
		}
		assertNotified(tb, app, adminID, wantAdmin)
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) })
	s.Test(t)
}

func TestCancelDateNonParticipant(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /match/{id}/cancel-date rejects non-participant",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"No eres participante"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "CDN A")
		p2 := handlers.MakePairTB(tb, app, "CDN B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
		m.Set("date", "2026-12-01")
		m.Set("time", "18:00")
		m.Set("club", "Padel 360")
		require.NoError(tb, app.Save(m))

		outsider := handlers.MakeUserTB(tb, app, "Outsider", "outsider-cd@test.local")
		s.URL = "/match/" + m.Id + "/cancel-date"
		s.Body = strings.NewReader("reason=Test")
		hdrs := handlers.AuthHeaders(tb, outsider)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestCancelDateOnPendingMatch(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /match/{id}/cancel-date rejects non-scheduled match",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"no tiene fecha confirmada"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "CDP A")
		p2 := handlers.MakePairTB(tb, app, "CDP B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		player, err := app.FindRecordById("users", p1.GetString("player1"))
		require.NoError(tb, err)
		s.URL = "/match/" + m.Id + "/cancel-date"
		s.Body = strings.NewReader("reason=Test")
		hdrs := handlers.AuthHeaders(tb, player)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestCancelDateWithin24h(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /match/{id}/cancel-date within 24h mentions it in timeline",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "CD24 A")
		p2 := handlers.MakePairTB(tb, app, "CD24 B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
		tomorrow := time.Now().Add(6 * time.Hour)
		m.Set("date", tomorrow.Format("2006-01-02"))
		m.Set("time", tomorrow.Format("15:04"))
		m.Set("club", "Padel 360")
		require.NoError(tb, app.Save(m))
		matchID = m.Id

		canceller, err := app.FindRecordById("users", p1.GetString("player1"))
		require.NoError(tb, err)
		s.URL = "/match/" + m.Id + "/cancel-date"
		s.Body = strings.NewReader("reason=Urgencia")
		hdrs := handlers.AuthHeaders(tb, canceller)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		msgs, _ := app.FindRecordsByFilter("match_messages",
			"match = {:id} && type = 'scheduling_response'", "", 0, 0,
			map[string]any{"id": matchID})
		require.GreaterOrEqual(tb, len(msgs), 1)
		assert.Contains(tb, msgs[0].GetString("content"), "menos de 24h")
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) })
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// Round-0 breadcrumb: leveled-league matches must not show "Jornada 0"
// ═══════════════════════════════════════════════════════════════════════

func TestMatchDetail_Round0_NoBreadcrumbJornada(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "GET /match/{id} round-0 match has no Jornada label in breadcrumb",
		Method:         http.MethodGet,
		ExpectedStatus: 200,
		// round_number=0 must never produce "Jornada 0" in the page
		NotExpectedContent: []string{"Jornada 0"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "R0A")
		p2 := handlers.MakePairTB(tb, app, "R0B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		m.Set("round_number", 0)
		require.NoError(tb, app.Save(m))
		s.URL = "/match/" + m.Id
		user, err := app.FindRecordById("users", p1.GetString("player1"))
		require.NoError(tb, err)
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestMatchDetail_Round1_ShowsJornada1(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /match/{id} round-1 match shows Jornada 1 in breadcrumb",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Jornada 1"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "R1A")
		p2 := handlers.MakePairTB(tb, app, "R1B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending") // round_number=1 by default
		s.URL = "/match/" + m.Id
		user, err := app.FindRecordById("users", p1.GetString("player1"))
		require.NoError(tb, err)
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

// makeMatchNotification creates a notification with related_match pointing to matchID.
func makeMatchNotification(t testing.TB, app core.App, userID, matchID string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("notifications")
	require.NoError(t, err)
	rec := core.NewRecord(col)
	rec.Set("user", userID)
	rec.Set("type", "general")
	rec.Set("title", "Test")
	rec.Set("body", "test body")
	rec.Set("related_match", matchID)
	require.NoError(t, app.Save(rec))
	return rec
}

func TestAdminRelease_OK(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /match/{id}/release admin deletes a pending leveled match",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "RelA")
		p2 := handlers.MakePairTB(tb, app, "RelB")
		p3 := handlers.MakePairTB(tb, app, "RelC")
		// 3 pairs, target=1 < 3-1=2 → IsLeveled=true
		comp := makeLeveledCompTB(tb, app, []*core.Record{p1, p2, p3})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		m.Set("round_number", 0)
		require.NoError(tb, app.Save(m))
		matchID = m.Id
		compID = comp.Id
		// attach a notification to the match
		user1, _ := app.FindRecordById("users", p1.GetString("player1"))
		makeMatchNotification(tb, app, user1.Id, matchID)
		admin := handlers.MakeAdminUserTB(tb, app)
		s.URL = "/match/" + matchID + "/release"
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		// match must be deleted
		_, err := app.FindRecordById("matches", matchID)
		assert.Error(tb, err, "match must be deleted after release")
		// notification must be deleted
		notifs, _ := app.FindRecordsByFilter("notifications", "related_match = {:m}", "", 0, 0, map[string]any{"m": matchID})
		assert.Empty(tb, notifs, "notifications for the match must be deleted")
		// competition event logged
		events, _ := app.FindRecordsByFilter("competition_events", "kind = 'assignment_released'", "", 0, 0, nil)
		assert.NotEmpty(tb, events, "assignment_released event must be logged")
	}
	handlers.ExpectRedirect(s, func(core.App) string { return "/competition/" + compID })
	s.Test(t)
}

func TestAdminRelease_PlayerForbidden(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /match/{id}/release player gets redirect (not admin)",
		Method:         http.MethodPost,
		ExpectedStatus: 302,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "FrbA")
		p2 := handlers.MakePairTB(tb, app, "FrbB")
		p3 := handlers.MakePairTB(tb, app, "FrbC")
		comp := makeLeveledCompTB(tb, app, []*core.Record{p1, p2, p3})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		m.Set("round_number", 0)
		require.NoError(tb, app.Save(m))
		matchID = m.Id
		s.URL = "/match/" + matchID + "/release"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		_, err := app.FindRecordById("matches", matchID)
		assert.NoError(tb, err, "match must still exist when non-admin tries to release")
	}
	s.Test(t)
}

func TestAdminRelease_ConfirmedMatch(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /match/{id}/release confirmed match returns error",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Solo se puede liberar un partido sin resultado de una liga nivelada"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "CfmA")
		p2 := handlers.MakePairTB(tb, app, "CfmB")
		p3 := handlers.MakePairTB(tb, app, "CfmC")
		comp := makeLeveledCompTB(tb, app, []*core.Record{p1, p2, p3})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		m.Set("round_number", 0)
		require.NoError(tb, app.Save(m))
		s.URL = "/match/" + m.Id + "/release"
		admin := handlers.MakeAdminUserTB(tb, app)
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.Test(t)
}

func TestAdminRelease_RoundRobinMatch(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /match/{id}/release round-robin match returns error",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Solo se puede liberar un partido sin resultado de una liga nivelada"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "RRA")
		p2 := handlers.MakePairTB(tb, app, "RRB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + m.Id + "/release"
		admin := handlers.MakeAdminUserTB(tb, app)
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.Test(t)
}

func TestAdminRelease_ButtonVisibility(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /match/{id} release button shown to admin on pending leveled match only",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Liberar partido"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "BtnA")
		p2 := handlers.MakePairTB(tb, app, "BtnB")
		p3 := handlers.MakePairTB(tb, app, "BtnC")
		comp := makeLeveledCompTB(tb, app, []*core.Record{p1, p2, p3})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		m.Set("round_number", 0)
		require.NoError(tb, app.Save(m))
		s.URL = "/match/" + m.Id
		admin := handlers.MakeAdminUserTB(tb, app)
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.Test(t)
}
