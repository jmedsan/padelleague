package handlers

import (
	"fmt"
	"net/http"
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
			assert.Equal(t, tt.want, statusClass(tt.status))
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════
// AdminOverride: score on match with no prior score (lines 319-320)
// ═══════════════════════════════════════════════════════════════════════

func TestAdminOverrideNewScore(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/admin-override sets new score",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "AO A")
		p2 := makePairTB(tb, app, "AO B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = m.Id
		s.URL = "/match/" + m.Id + "/admin-override"
		s.Body = strings.NewReader("scores=6-3+6-4")
		hdrs := authHeaders(tb, admin)
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
	}
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// AdminOverride: score correction (existing score → new score) (line 319 negation)
// ═══════════════════════════════════════════════════════════════════════

func TestAdminOverrideCorrectedScore(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/admin-override corrects existing score",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "AC A")
		p2 := makePairTB(tb, app, "AC B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")
		m.Set("scores", "6-3 6-4")
		m.Set("winner", p1.Id)
		require.NoError(tb, app.Save(m))
		matchID = m.Id
		s.URL = "/match/" + m.Id + "/admin-override"
		s.Body = strings.NewReader("scores=6-4+6-3")
		hdrs := authHeaders(tb, admin)
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
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// AdminOverride: venue set on match with no prior venue (lines 350-351)
// ═══════════════════════════════════════════════════════════════════════

func TestAdminOverrideNewVenue(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/admin-override sets new venue",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "AV A")
		p2 := makePairTB(tb, app, "AV B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = m.Id
		venue := makeVenueTB(tb, app, "Padel Test")
		s.URL = "/match/" + m.Id + "/admin-override"
		s.Body = strings.NewReader("venue_id=" + venue.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "Padel Test", m.GetString("club"))

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
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// AdminOverride: date set on match with no prior date (line 331)
// ═══════════════════════════════════════════════════════════════════════

func TestAdminOverrideNewDate(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/admin-override sets new date",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "AD A")
		p2 := makePairTB(tb, app, "AD B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = m.Id
		s.URL = "/match/" + m.Id + "/admin-override"
		s.Body = strings.NewReader("date=2026-09-01")
		hdrs := authHeaders(tb, admin)
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
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// buildShareText: final match shows correct winner (lines 364, 371)
// ═══════════════════════════════════════════════════════════════════════

func TestBuildShareTextFinalMatch(t *testing.T) {
	t.Parallel()
	app, err := tests.NewTestApp(tmplDataDir)
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	p1 := makePairTB(t, app, "Pair Alpha")
	p2 := makePairTB(t, app, "Pair Beta")
	comp := makeCompetitionTB(t, app, "league", []*core.Record{p1, p2})

	t.Run("pair1 wins", func(t *testing.T) {
		m := makeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "final")
		m.Set("scores", "6-3 6-4")
		m.Set("winner", p1.Id)
		require.NoError(t, app.Save(m))
		text, shareURL := buildShareText(app, m, "https://example.com", "/match/"+m.Id)
		assert.NotEmpty(t, text)
		assert.Contains(t, text, "Pair+Alpha")
		assert.Contains(t, text, "Ganador")
		assert.Contains(t, text, "Pair+Alpha%21")
		assert.Contains(t, text, "https%3A%2F%2Fexample.com%2Fmatch%2F"+m.Id)
		assert.Equal(t, "https://example.com/match/"+m.Id, shareURL)
	})

	t.Run("pair2 wins", func(t *testing.T) {
		m := makeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "final")
		m.Set("scores", "3-6 4-6")
		m.Set("winner", p2.Id)
		require.NoError(t, app.Save(m))
		text, shareURL := buildShareText(app, m, "https://example.com", "/match/"+m.Id)
		assert.Contains(t, text, "Pair+Beta%21")
		assert.NotContains(t, text, "Pair+Alpha%21")
		assert.Equal(t, "https://example.com/match/"+m.Id, shareURL)
	})

	t.Run("non-final returns empty", func(t *testing.T) {
		m := makeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "pending")
		text, shareURL := buildShareText(app, m, "https://example.com", "/match/"+m.Id)
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
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/submit notifies opponent pair",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var pair2Player1ID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Sub A")
		p2 := makePairTB(tb, app, "Sub B")
		pair2Player1ID = p2.GetString("player1")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		m.Set("date", "2026-09-01")
		m.Set("club", "Padel 360")
		require.NoError(tb, app.Save(m))

		submitter, err := app.FindRecordById("users", p1.GetString("player1"))
		require.NoError(tb, err)
		s.URL = "/match/" + m.Id + "/submit"
		s.Body = strings.NewReader("scores=6-3+6-4")
		hdrs := authHeaders(tb, submitter)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		notifs, _ := app.FindRecordsByFilter("notifications",
			"user = {:uid}", "", 0, 0,
			map[string]any{"uid": pair2Player1ID})
		assert.GreaterOrEqual(tb, len(notifs), 1,
			"rival player must receive a notification")
	}
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// MatchDetail: competition name shown (lines 156, 158)
// ═══════════════════════════════════════════════════════════════════════

func TestMatchDetailShowsCompName(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /match/{id} shows competition name",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Liga Visible"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "MD A")
		p2 := makePairTB(tb, app, "MD B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("name", "Liga Visible")
		require.NoError(tb, app.Save(comp))
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + m.Id
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestMatchDetailAdminShowsResolveForm(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /match/{id}/thread admin view shows resolve form for disputed match",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Resolver"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Res A")
		p2 := makePairTB(tb, app, "Res B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		m.Set("scores", "6-3 6-4")
		m.Set("submitted_by", p1.GetString("player1"))
		m.Set("disputed_by", p2.GetString("player1"))
		m.Set("disputed_scores", "6-4 6-3")
		require.NoError(tb, app.Save(m))
		// The result surface (resolve form) now lives in the thread fragment.
		s.URL = "/match/" + m.Id + "/thread"
		s.ExpectedContent = append(s.ExpectedContent, `hx-post="/admin/disputes/`+m.Id+`/resolve"`)
		admin := makeAdminUserTB(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// playerNameIfSet: empty returns empty, non-empty returns name (line 357)
// ═══════════════════════════════════════════════════════════════════════

func TestPlayerNameIfSet(t *testing.T) {
	t.Parallel()
	app, err := tests.NewTestApp(tmplDataDir)
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	t.Run("empty returns empty", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "", playerNameIfSet(app, ""))
	})

	t.Run("valid user returns name", func(t *testing.T) {
		t.Parallel()
		u := makeUserTB(t, app, "NameTest", "nametest@test.local")
		name := playerNameIfSet(app, u.Id)
		assert.Equal(t, "NameTest", name)
	})
}

// HTML markers rendered when each flag is true.
const (
	markerCanSubmit   = "Registrar resultado"
	markerCanWalkover = "Reportar partido no jugado"
	markerCanCorrect  = "Corregir marcador"
	markerDateGate    = "Primero propón una fecha y lugar"
)

type matchViewCase struct {
	name         string
	status       string
	viewer       string // "submitter", "opponent", "outsider", "admin"
	submitted    bool
	recentSubmit bool
	hasDate      bool
	httpStatus   int // 0 means 200
	want         []string
	deny         []string
}

func TestBuildMatchViewFlags(t *testing.T) {
	t.Parallel()
	cases := []matchViewCase{
		// Pending without date → submit gated
		{
			name: "pending/no-date/submitter", status: "pending", viewer: "submitter",
			want: []string{markerDateGate, markerCanWalkover},
			deny: []string{markerCanSubmit, markerCanCorrect},
		},
		// Pending with date+place → submit visible
		{
			name: "pending/with-date/submitter", status: "pending", viewer: "submitter",
			hasDate: true,
			want:    []string{markerCanSubmit, markerCanWalkover},
			deny:    []string{markerCanCorrect, markerDateGate},
		},
		{
			name: "pending/with-date/opponent", status: "pending", viewer: "opponent",
			hasDate: true,
			want:    []string{markerCanSubmit, markerCanWalkover},
			deny:    []string{markerCanCorrect, markerDateGate},
		},
		{
			name: "scheduled/submitter-team", status: "scheduled", viewer: "submitter",
			hasDate: true,
			want:    []string{markerCanSubmit, markerCanWalkover},
			deny:    []string{markerCanCorrect},
		},
		{
			name: "pending/outsider", status: "pending", viewer: "outsider",
			deny: []string{markerCanSubmit, markerCanWalkover, markerCanCorrect},
		},
		{
			name: "pending/admin", status: "pending", viewer: "admin",
			deny: []string{markerCanSubmit, markerCanWalkover, markerCanCorrect},
		},

		// Confirmed with submitter set (legacy status — no confirm/dispute buttons)
		{
			name: "confirmed/submitter/recent", status: "confirmed", viewer: "submitter",
			submitted: true, recentSubmit: true,
			want: []string{markerCanCorrect, markerCanWalkover},
			deny: []string{markerCanSubmit},
		},
		{
			name: "confirmed/submitter/expired", status: "confirmed", viewer: "submitter",
			submitted: true, recentSubmit: false,
			want: []string{markerCanWalkover},
			deny: []string{markerCanSubmit, markerCanCorrect},
		},
		{
			name: "confirmed/opponent", status: "confirmed", viewer: "opponent",
			submitted: true,
			want:      []string{markerCanWalkover},
			deny:      []string{markerCanSubmit, markerCanCorrect},
		},
		{
			name: "confirmed/outsider", status: "confirmed", viewer: "outsider",
			submitted: true,
			deny:      []string{markerCanSubmit, markerCanCorrect},
		},
		{
			name: "confirmed/admin-nonparticipant", status: "confirmed", viewer: "admin",
			submitted: true,
			deny:      []string{markerCanSubmit, markerCanCorrect, markerCanWalkover},
		},
		{
			name: "confirmed/no-submitter/opponent", status: "confirmed", viewer: "opponent",
			submitted: false,
			want:      []string{markerCanWalkover},
			deny:      []string{markerCanSubmit, markerCanCorrect},
		},

		// Disputed
		{
			name: "disputed/submitter", status: "disputed", viewer: "submitter",
			submitted: true,
			deny:      []string{markerCanSubmit, markerCanCorrect, markerCanWalkover},
		},
		{
			name: "disputed/opponent", status: "disputed", viewer: "opponent",
			submitted: true,
			deny:      []string{markerCanSubmit, markerCanCorrect, markerCanWalkover},
		},

		// Final
		{
			name: "final/submitter", status: "final", viewer: "submitter",
			submitted: true,
			deny:      []string{markerCanSubmit, markerCanCorrect, markerCanWalkover},
		},
		{
			name: "final/opponent", status: "final", viewer: "opponent",
			submitted: true,
			deny:      []string{markerCanSubmit, markerCanCorrect, markerCanWalkover},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expectedStatus := tc.httpStatus
			if expectedStatus == 0 {
				expectedStatus = 200
			}
			s := &tests.ApiScenario{
				TestAppFactory:     testAppFactory,
				Name:               tc.name,
				Method:             http.MethodGet,
				ExpectedStatus:     expectedStatus,
				ExpectedContent:    tc.want,
				NotExpectedContent: tc.deny,
			}

			s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				setupAllRoutes(tb, app, e)

				p1 := makePairTB(tb, app, fmt.Sprintf("MV%s A", tc.name[:3]))
				p2 := makePairTB(tb, app, fmt.Sprintf("MV%s B", tc.name[:3]))
				comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
				match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, tc.status)

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
					viewerUser = makeUserTB(tb, app, "MV Outsider", "")
				case "admin":
					viewerUser = makeAdminUserTB(tb, app)
				}
				s.Headers = authHeaders(tb, viewerUser)
			}

			s.Test(t)
		})
	}
}

func TestAdminOverride(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/admin-override changes score and finalizes",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, p1ID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Override A")
		p2 := makePairTB(tb, app, "Override B")
		p1ID = p1.Id
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		matchID = m.Id
		m.Set("scores", "6-3 6-4")
		m.Set("submitted_by", p1.GetString("player1"))
		require.NoError(tb, app.Save(m))

		s.URL = "/match/" + m.Id + "/admin-override"
		s.Body = strings.NewReader("scores=6-4+6-3&dispute_notes=Corregido+por+admin")
		admin := makeAdminUserTB(tb, app)
		hdrs := authHeaders(tb, admin)
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
	s.Test(t)
}

func TestAdminOverrideWithDateChange(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/admin-override changes date and time",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "OvrDate A")
		p2 := makePairTB(tb, app, "OvrDate B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = m.Id
		m.Set("date", "2026-09-01")
		m.Set("time", "18:00")
		m.Set("club", "Old Club")
		require.NoError(tb, app.Save(m))

		venue := makeVenueTB(tb, app, "New Club")

		s.URL = "/match/" + m.Id + "/admin-override"
		s.Body = strings.NewReader("date=2026-09-15&time=20:00&venue_id=" + venue.Id)
		admin := makeAdminUserTB(tb, app)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "20:00", m.GetString("time"))
		assert.Equal(tb, "New Club", m.GetString("club"))
	}
	s.Test(t)
}

func TestAdminOverrideNoChanges(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /match/{id}/admin-override with no changes warns",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"alert-warning"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "OvrNone A")
		p2 := makePairTB(tb, app, "OvrNone B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		s.URL = "/match/" + m.Id + "/admin-override"
		s.Body = strings.NewReader("")
		admin := makeAdminUserTB(tb, app)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestAdminOverrideNonAdmin(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /match/{id}/admin-override as player fails",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Solo administradores"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "OvrNoAdm A")
		p2 := makePairTB(tb, app, "OvrNoAdm B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		s.URL = "/match/" + m.Id + "/admin-override"
		s.Body = strings.NewReader("scores=6-3+6-4")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestMatchSubmitAlreadyScored(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /match/{id}/submit on non-pending fails",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"ya tiene"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "SubDup A")
		p2 := makePairTB(tb, app, "SubDup B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")

		s.URL = "/match/" + m.Id + "/submit"
		s.Body = strings.NewReader("scores=6-3+6-4")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestMatchSubmitRejectsWithoutDateOrPlace(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /match/{id}/submit rejects when no date/place agreed",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Primero acuerda"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "NoDate A")
		p2 := makePairTB(tb, app, "NoDate B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		s.URL = "/match/" + m.Id + "/submit"
		s.Body = strings.NewReader("scores=6-3+6-4")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := authHeaders(tb, user)
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
		TestAppFactory:  testAppFactory,
		Name:            "POST /match/{id}/submit rejects when pairs are not assigned",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"parejas asignadas"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Empty A")
		p2 := makePairTB(tb, app, "Empty B")
		comp := makeCompetitionTB(tb, app, "playoff", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, "", "", "pending")
		m.Set("date", "2026-09-01")
		m.Set("club", "Padel 360")
		require.NoError(tb, app.Save(m))

		s.URL = "/match/" + m.Id + "/submit"
		s.Body = strings.NewReader("scores=6-3+6-4")
		admin := makeAdminUserTB(tb, app)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestAdminOverrideCourtNumber(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/admin-override sets court_number",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Court A")
		p2 := makePairTB(tb, app, "Court B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = m.Id

		s.URL = "/match/" + m.Id + "/admin-override"
		s.Body = strings.NewReader("court_number=5")
		admin := makeAdminUserTB(tb, app)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "5", m.GetString("court_number"))
	}
	s.Test(t)
}

func TestPlayerSubmitBlockedOnFinalizedComp(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /match/{id}/submit blocked for player on finalized competition",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"finalizada o archivada"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "BF A")
		p2 := makePairTB(tb, app, "BF B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("finalized", true)
		require.NoError(tb, app.Save(comp))
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + m.Id + "/submit"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
		s.Body = strings.NewReader("scores=6-3+6-4")
	}
	s.Test(t)
}

func TestPlayerSubmitBlockedOnInactiveComp(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /match/{id}/submit blocked for player on inactive competition",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"finalizada o archivada"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "BI A")
		p2 := makePairTB(tb, app, "BI B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("active", false)
		require.NoError(tb, app.Save(comp))
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + m.Id + "/submit"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
		s.Body = strings.NewReader("scores=6-3+6-4")
	}
	s.Test(t)
}

func TestAdminSubmitAllowedOnFinalizedComp(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/admin-override allowed on finalized competition",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "AF A")
		p2 := makePairTB(tb, app, "AF B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("finalized", true)
		require.NoError(tb, app.Save(comp))
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + m.Id + "/admin-override"
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
		s.Body = strings.NewReader("scores=6-3+6-4")
	}
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
		{"report-unplayed", "pending", "/report-unplayed", "reason=test"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := &tests.ApiScenario{
				TestAppFactory:  testAppFactory,
				Name:            "player " + tc.name + " blocked on finalized comp",
				Method:          http.MethodPost,
				ExpectedStatus:  200,
				ExpectedContent: []string{"finalizada o archivada"},
			}
			s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				setupAllRoutes(tb, app, e)
				p1 := makePairTB(tb, app, "RO-"+tc.name+" A")
				p2 := makePairTB(tb, app, "RO-"+tc.name+" B")
				comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
				comp.Set("finalized", true)
				require.NoError(tb, app.Save(comp))
				m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, tc.status)
				if tc.name == "correct" {
					m.Set("submitted_by", p1.GetString("player1"))
					m.Set("submitted_at", time.Now().UTC().Format(time.RFC3339))
					require.NoError(tb, app.Save(m))
					makeResultProposal(tb, app, m.Id, p1.GetString("player1"), "6-3 6-4")
				}
				s.URL = "/match/" + m.Id + tc.path
				user, _ := app.FindRecordById("users", p2.GetString("player1"))
				hdrs := authHeaders(tb, user)
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
		{"change-decision", "/thread/proposal/%s/change-decision", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := &tests.ApiScenario{
				TestAppFactory:  testAppFactory,
				Name:            "player " + tc.name + " blocked on finalized comp",
				Method:          http.MethodPost,
				ExpectedStatus:  200,
				ExpectedContent: []string{"finalizada o archivada"},
			}
			s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				setupAllRoutes(tb, app, e)
				p1 := makePairTB(tb, app, "RO-"+tc.name+" A")
				p2 := makePairTB(tb, app, "RO-"+tc.name+" B")
				comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
				comp.Set("finalized", true)
				require.NoError(tb, app.Save(comp))
				m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
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
				hdrs := authHeaders(tb, user)
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
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/submit creates result_submission proposal",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, submitterID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "RP A")
		p2 := makePairTB(tb, app, "RP B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
		m.Set("date", "2026-09-01")
		m.Set("club", "Padel 360")
		require.NoError(tb, app.Save(m))

		submitter, err := app.FindRecordById("users", p1.GetString("player1"))
		require.NoError(tb, err)
		submitterID = submitter.Id
		matchID = m.Id
		s.URL = "/match/" + m.Id + "/submit"
		s.Body = strings.NewReader("scores=6-3+6-4")
		hdrs := authHeaders(tb, submitter)
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
		pd := ParseProposalData(prop.GetString("proposal_data"))
		require.NotNil(tb, pd, "proposal_data must be parseable")
		assert.Equal(tb, "6-3 6-4", pd.Scores)
	}
	s.Test(t)
}

func TestMatchSubmitSupersedesPreviousProposal(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/submit supersedes previous pending proposal from same pair",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, submitterID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "SS A")
		p2 := makePairTB(tb, app, "SS B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
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
		hdrs := authHeaders(tb, submitter)
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
		assert.Equal(tb, "6-3 6-4", ParseProposalData(pending[0].GetString("proposal_data")).Scores)

		superseded, _ := app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'result_submission' && author = {:uid} && proposal_status = 'superseded'",
			"", 0, 0,
			map[string]any{"mid": matchID, "uid": submitterID})
		assert.Len(tb, superseded, 1, "old proposal must be superseded")
	}
	s.Test(t)
}

func TestMatchSubmitRejectsWhenRivalHasPending(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /match/{id}/submit rejects when rival has pending result",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Ya hay una propuesta de resultado del rival pendiente"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "DL A")
		p2 := makePairTB(tb, app, "DL B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
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
		hdrs := authHeaders(tb, submitter)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestMatchSubmitNoDeadlockNoAdminNotif(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/submit without opposing proposal sends no admin notification",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "NDL A")
		p2 := makePairTB(tb, app, "NDL B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
		m.Set("date", "2026-09-01")
		m.Set("club", "Padel 360")
		require.NoError(tb, app.Save(m))

		makeAdminUserTB(tb, app)

		submitter, err := app.FindRecordById("users", p1.GetString("player1"))
		require.NoError(tb, err)
		s.URL = "/match/" + m.Id + "/submit"
		s.Body = strings.NewReader("scores=6-3+6-4")
		hdrs := authHeaders(tb, submitter)
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
	s.Test(t)
}
