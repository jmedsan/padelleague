package handlers

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Correction window boundary

// Just under 24h → correction allowed
func TestMatchCorrectBoundary_Under24h_Allowed(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "correction at 23h59m allowed",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Bnd A")
		p2 := makePairTB(tb, app, "Bnd B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
		matchID = match.Id
		submitter := p1.GetString("player1")
		match.Set("submitted_by", submitter)
		match.SetRaw("submitted_at", time.Now().Add(-23*time.Hour-59*time.Minute).UTC().Format(time.RFC3339))
		require.NoError(tb, app.Save(match))
		makeResultProposal(tb, app, match.Id, submitter, "6-3 6-4")
		s.URL = "/match/" + match.Id + "/correct"
		s.Body = strings.NewReader("scores=6-4+6-3")
		user, _ := app.FindRecordById("users", submitter)
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		pending, _ := app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'result_submission' && proposal_status = 'pending'",
			"", 0, 0, map[string]any{"mid": matchID})
		require.Len(tb, pending, 1)
		assert.Equal(tb, "6-4 6-3", ParseProposalData(pending[0].GetString("proposal_data")).Scores,
			"corrected scores must be in the new proposal")
	}
	s.Test(t)
}

// Just over 24h → correction refused
func TestMatchCorrectBoundary_Over24h_Refused(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "correction at 24h01m refused",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"24 horas"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "BndO A")
		p2 := makePairTB(tb, app, "BndO B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
		submitter := p1.GetString("player1")
		match.Set("submitted_by", submitter)
		match.SetRaw("submitted_at", time.Now().Add(-24*time.Hour-1*time.Minute).UTC().Format(time.RFC3339))
		require.NoError(tb, app.Save(match))
		makeResultProposal(tb, app, match.Id, submitter, "6-3 6-4")
		s.URL = "/match/" + match.Id + "/correct"
		s.Body = strings.NewReader("scores=6-4+6-3")
		user, _ := app.FindRecordById("users", submitter)
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestMatchCorrectBoundary_Exact24h_Refused(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "correction at exactly 24h refused",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"24 horas"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "BndE A")
		p2 := makePairTB(tb, app, "BndE B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
		submitter := p1.GetString("player1")
		match.Set("submitted_by", submitter)
		match.SetRaw("submitted_at", time.Now().Add(-24*time.Hour).UTC().Format(time.RFC3339))
		require.NoError(tb, app.Save(match))
		makeResultProposal(tb, app, match.Id, submitter, "6-3 6-4")
		s.URL = "/match/" + match.Id + "/correct"
		s.Body = strings.NewReader("scores=6-4+6-3")
		user, _ := app.FindRecordById("users", submitter)
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

// Rival team cannot correct (kills myTeam != submitterTeam negation mutant)
func TestMatchCorrectRivalTeamRejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "rival team correction rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Solo el equipo que envió"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "RivA")
		p2 := makePairTB(tb, app, "RivB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
		submitter := p1.GetString("player1")
		match.Set("submitted_by", submitter)
		match.SetRaw("submitted_at", time.Now().Add(-1*time.Hour).UTC().Format(time.RFC3339))
		require.NoError(tb, app.Save(match))
		makeResultProposal(tb, app, match.Id, submitter, "6-3 6-4")
		s.URL = "/match/" + match.Id + "/correct"
		s.Body = strings.NewReader("scores=6-4+6-3")
		rival, _ := app.FindRecordById("users", p2.GetString("player1"))
		hdrs := authHeaders(tb, rival)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

// Admin can correct any team's submission (kills isAdmin negation mutant)
func TestMatchCorrectAdminBypass(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "admin correction bypasses team check",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "AdmA")
		p2 := makePairTB(tb, app, "AdmB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
		matchID = match.Id
		submitter := p1.GetString("player1")
		match.Set("submitted_by", submitter)
		match.SetRaw("submitted_at", time.Now().Add(-1*time.Hour).UTC().Format(time.RFC3339))
		require.NoError(tb, app.Save(match))
		makeResultProposal(tb, app, match.Id, submitter, "6-3 6-4")
		s.URL = "/match/" + match.Id + "/correct"
		s.Body = strings.NewReader("scores=6-4+6-3")
		admin := makeAdminUserTB(tb, app)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		pending, _ := app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'result_submission' && proposal_status = 'pending'",
			"-created", 0, 0, map[string]any{"mid": matchID})
		require.Len(tb, pending, 2, "original + admin correction both pending")
		assert.Equal(tb, "6-4 6-3", ParseProposalData(pending[0].GetString("proposal_data")).Scores,
			"newest pending is admin's correction")
	}
	s.Test(t)
}
