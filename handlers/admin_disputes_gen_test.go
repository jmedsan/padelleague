package handlers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
