package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/league"
)

// --- Dispute resolve with manual winner = pair2 ---

func TestDisputeResolveManualWinnerPair2(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/disputes/{id}/resolve manual winner pair2",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, p2ID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "DispP2A")
		p2 := makePairTB(tb, app, "DispP2B")
		p2ID = p2.Id
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		matchID = match.Id
		s.URL = "/admin/disputes/" + match.Id + "/resolve"
		s.Body = strings.NewReader("score=6-3+6-4&winner=" + p2.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "final", m.GetString("status"))
		assert.Equal(tb, "6-3 6-4", m.GetString("scores"))
		assert.Equal(tb, p2ID, m.GetString("winner"))
	}
	s.Test(t)
}

// --- Dispute resolve with manual winner = invalid pair (not in match) ---

func TestDisputeResolveManualWinnerInvalid(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/disputes/{id}/resolve rejects winner not in match",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"El ganador debe ser una de las dos parejas"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "DispInvA")
		p2 := makePairTB(tb, app, "DispInvB")
		outsider := makePairTB(tb, app, "Outsider")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2, outsider})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		s.URL = "/admin/disputes/" + match.Id + "/resolve"
		s.Body = strings.NewReader("score=6-3+6-4&winner=" + outsider.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

// --- Dispute resolve with manual winner + invalid score → rejected ---

func TestDisputeResolveManualWinnerInvalidScore(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/disputes/{id}/resolve rejects invalid score with manual winner",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Marcador inválido"},
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "DispBadScA")
		p2 := makePairTB(tb, app, "DispBadScB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		matchID = match.Id
		s.URL = "/admin/disputes/" + match.Id + "/resolve"
		s.Body = strings.NewReader("score=99-99&winner=" + p1.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "disputed", m.GetString("status"), "status must remain disputed")
		assert.Empty(tb, m.GetString("scores"), "scores must not be set")
	}
	s.Test(t)
}

// --- Dispute resolve without manual winner (auto-determine from score) ---
// Covers the else branch at line 71-77 where DetermineWinner is called.

func TestDisputeResolveAutoWinner(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/disputes/{id}/resolve auto-determines winner from score",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, p1ID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "DispAutoA")
		p2 := makePairTB(tb, app, "DispAutoB")
		p1ID = p1.Id
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		matchID = match.Id
		s.URL = "/admin/disputes/" + match.Id + "/resolve"
		// No winner param — score determines winner (pair1 wins 6-3 6-4)
		s.Body = strings.NewReader("score=6-3+6-4")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "final", m.GetString("status"))
		assert.Equal(tb, p1ID, m.GetString("winner"))
	}
	s.Test(t)
}

// --- Report unplayed (walkover request) ---

func TestReportUnplayed(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/report-unplayed sets walkover review",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, userID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "RptA")
		p2 := makePairTB(tb, app, "RptB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		userID = user.Id
		s.URL = "/match/" + match.Id + "/report-unplayed"
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "disputed", m.GetString("status"))
		assert.Equal(tb, "walkover", m.GetString("review_type"))
		assert.Equal(tb, userID, m.GetString("walkover_requested_by"))
		assert.Contains(tb, m.GetString("dispute_notes"), "[No jugado]")
		assert.Empty(tb, m.GetString("winner"), "reporting unplayed must not declare a winner")
		assert.Empty(tb, m.GetString("scores"), "reporting unplayed must not set a score")
	}
	s.Test(t)
}

func TestReportUnplayed_Idempotent(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/report-unplayed is idempotent",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "IdpA")
		p2 := makePairTB(tb, app, "IdpB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		match.Set("review_type", "walkover")
		match.Set("status", "disputed")
		require.NoError(tb, app.Save(match))
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.URL = "/match/" + match.Id + "/report-unplayed"
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestReportUnplayedWrongStatus_Refused(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /match/{id}/report-unplayed on final fails",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"no puede reportarse"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "RptStatusA")
		p2 := makePairTB(tb, app, "RptStatusB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")

		s.URL = "/match/" + m.Id + "/report-unplayed"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestReportUnplayedNonParticipant_Refused(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "non-participant cannot report unplayed",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"No eres participante"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		outsider := makeUserTB(tb, app, "Outsider Rpt", "")
		p1 := makePairTB(tb, app, "ORptA")
		p2 := makePairTB(tb, app, "ORptB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + match.Id + "/report-unplayed"
		s.Headers = authHeaders(tb, outsider)
	}
	s.Test(t)
}

// --- Walkover approve ---

func TestWalkoverApprove(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/disputes/{id}/walkover-approve finalizes match with penalty",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, p1ID, p2ID, compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "WoApA")
		p2 := makePairTB(tb, app, "WoApB")
		p1ID = p1.Id
		p2ID = p2.Id
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("default_penalty", 5)
		comp.Set("walkover_score", "6-0 6-0")
		require.NoError(tb, app.Save(comp))
		compID = comp.Id
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		match.Set("review_type", "walkover")
		require.NoError(tb, app.Save(match))
		matchID = match.Id
		s.URL = "/admin/disputes/" + match.Id + "/walkover-approve"
		s.Body = strings.NewReader("winner=" + p1.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "final", m.GetString("status"))
		assert.Equal(tb, "6-0 6-0", m.GetString("scores"))
		assert.Equal(tb, p1ID, m.GetString("winner"))

		comp, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		penalties := make(map[string]float64)
		require.NoError(tb, comp.UnmarshalJSONField("penalty_points", &penalties))
		assert.Equal(tb, 5.0, penalties[p2ID], "losing pair must have penalty applied")
	}
	s.Test(t)
}

func TestWalkoverApprove_ZeroPenalty_NoPenaltyApplied(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/disputes/{id}/walkover-approve with default_penalty=0 skips penalty",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, p2ID, compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "WoZeroA")
		p2 := makePairTB(tb, app, "WoZeroB")
		p2ID = p2.Id
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("default_penalty", 0)
		comp.Set("walkover_score", "6-0 6-0")
		require.NoError(tb, app.Save(comp))
		compID = comp.Id
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		match.Set("review_type", "walkover")
		require.NoError(tb, app.Save(match))
		matchID = match.Id
		s.URL = "/admin/disputes/" + match.Id + "/walkover-approve"
		s.Body = strings.NewReader("winner=" + p1.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "final", m.GetString("status"))

		comp, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		penalties := make(map[string]float64)
		require.NoError(tb, comp.UnmarshalJSONField("penalty_points", &penalties))
		_, has := penalties[p2ID]
		assert.False(tb, has, "no penalty entry must be recorded when default_penalty is 0")
	}
	s.Test(t)
}

func TestWalkoverApprove_PenaltyFails_AlertsAdmin(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/disputes/{id}/walkover-approve surfaces penalty save failure",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"no se pudo aplicar la penalización"},
	}
	var matchID, compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "WoFailA")
		p2 := makePairTB(tb, app, "WoFailB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("default_penalty", 5)
		comp.Set("walkover_score", "6-0 6-0")
		require.NoError(tb, app.Save(comp))
		compID = comp.Id
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		match.Set("review_type", "walkover")
		require.NoError(tb, app.Save(match))
		matchID = match.Id

		app.OnRecordUpdate("competitions").BindFunc(func(ev *core.RecordEvent) error {
			if ev.Record.Id == compID {
				return fmt.Errorf("simulated DB failure for penalty save")
			}
			return ev.Next()
		})

		s.URL = "/admin/disputes/" + match.Id + "/walkover-approve"
		s.Body = strings.NewReader("winner=" + p1.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "final", m.GetString("status"), "match must still finalize even if the penalty save fails")
	}
	s.Test(t)
}

func TestWalkoverApprove_AlreadyFinal_RejectedNoDoublePenalty(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/disputes/{id}/walkover-approve on an already-final match is rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"ya está resuelto"},
	}
	var compID, p2ID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "WoRepA")
		p2 := makePairTB(tb, app, "WoRepB")
		p2ID = p2.Id
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("default_penalty", 5)
		comp.Set("walkover_score", "6-0 6-0")
		require.NoError(tb, league.ApplyPenalty(app, comp, p2.Id, 5, true))
		compID = comp.Id
		// Simulates the state right after a first, successful approval.
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")
		match.Set("review_type", "walkover")
		match.Set("scores", "6-0 6-0")
		match.Set("winner", p1.Id)
		require.NoError(tb, app.Save(match))

		s.URL = "/admin/disputes/" + match.Id + "/walkover-approve"
		s.Body = strings.NewReader("winner=" + p1.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		comp, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		penalties := make(map[string]float64)
		require.NoError(tb, comp.UnmarshalJSONField("penalty_points", &penalties))
		assert.Equal(tb, 5.0, penalties[p2ID], "penalty must stay at the original amount, not doubled")
	}
	s.Test(t)
}

func TestWalkoverApprove_NotWalkover(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/disputes/{id}/walkover-approve rejects non-walkover",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"no es una solicitud de walkover"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "WoNonA")
		p2 := makePairTB(tb, app, "WoNonB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		s.URL = "/admin/disputes/" + match.Id + "/walkover-approve"
		s.Body = strings.NewReader("winner=" + p1.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestAdminDisputesPage(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /admin/disputes with disputed match",
		Method:          http.MethodGet,
		URL:             "/admin/disputes",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Disputas"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupFullAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "DispPageA")
		p2 := makePairTB(tb, app, "DispPageB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}
