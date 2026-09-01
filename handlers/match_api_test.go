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

func TestMatchSubmitScore(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/submit with valid score returns 204 + HX-Redirect",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, submitterID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Submit A")
		p2 := makePairTB(tb, app, "Submit B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		match.Set("date", "2026-09-01")
		match.Set("club", "Padel 360")
		require.NoError(tb, app.Save(match))
		matchID = match.Id
		submitterID = p1.GetString("player1")
		s.URL = "/match/" + match.Id + "/submit"
		s.Body = strings.NewReader("scores=6-3+6-4")
		user, _ := app.FindRecordById("users", submitterID)
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "pending", m.GetString("status"), "match stays pre-score after submit")
		assert.Equal(tb, submitterID, m.GetString("submitted_by"))
		assert.Equal(tb, "/match/"+matchID, res.Header.Get("HX-Redirect"))

		proposals, err := app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'result_submission' && proposal_status = 'pending'",
			"", 0, 0, map[string]any{"mid": matchID})
		require.NoError(tb, err)
		require.Len(tb, proposals, 1, "a pending result proposal must exist")
		assert.Equal(tb, "6-3 6-4", ParseProposalData(proposals[0].GetString("proposal_data")).Scores)
	}
	s.Test(t)
}

func TestMatchCorrect(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/correct within 24h returns 204",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Correct A")
		p2 := makePairTB(tb, app, "Correct B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
		matchID = match.Id
		submitter := p1.GetString("player1")
		match.Set("submitted_by", submitter)
		match.Set("submitted_at", time.Now().UTC().Format(time.RFC3339))
		require.NoError(tb, app.Save(match))
		makeResultProposal(tb, app, match.Id, submitter, "6-3 6-4")
		s.URL = "/match/" + match.Id + "/correct"
		s.Body = strings.NewReader("scores=6-4+6-3")
		user, _ := app.FindRecordById("users", submitter)
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "scheduled", m.GetString("status"), "match stays pre-score")

		pending, _ := app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'result_submission' && proposal_status = 'pending'",
			"", 0, 0, map[string]any{"mid": matchID})
		require.Len(tb, pending, 1, "corrected proposal must be pending")
		assert.Equal(tb, "6-4 6-3", ParseProposalData(pending[0].GetString("proposal_data")).Scores)

		superseded, _ := app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'result_submission' && proposal_status = 'superseded'",
			"", 0, 0, map[string]any{"mid": matchID})
		assert.Len(tb, superseded, 1, "old proposal must be superseded")
		assert.Equal(tb, "/match/"+matchID, res.Header.Get("HX-Redirect"))
	}
	s.Test(t)
}

func TestMatchThread(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /match/{id}/thread with auth returns thread",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"proposal-form"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Thread A")
		p2 := makePairTB(tb, app, "Thread B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + match.Id + "/thread"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestMatchThreadPostMessage(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/thread/message sends a message",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Msg A")
		p2 := makePairTB(tb, app, "Msg B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id
		s.URL = "/match/" + match.Id + "/thread/message"
		s.Body = strings.NewReader("content=Hola+equipo&type=chat")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		msgs, err := app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'chat'", "", 0, 0,
			map[string]any{"mid": matchID})
		require.NoError(tb, err)
		assert.Equal(tb, 1, len(msgs))
		assert.Equal(tb, "Hola equipo", msgs[0].GetString("content"))
	}
	s.Test(t)
}

func TestMatchSubmitWORejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /match/{id}/submit with WO score rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"incomparecencia"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "WOSub A")
		p2 := makePairTB(tb, app, "WOSub B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		match.Set("date", "2026-09-01")
		match.Set("club", "Padel 360")
		require.NoError(tb, app.Save(match))
		s.URL = "/match/" + match.Id + "/submit"
		s.Body = strings.NewReader("scores=WO")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestMatchCorrectExpired(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /match/{id}/correct after 24h rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"24 horas"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "CorrExp A")
		p2 := makePairTB(tb, app, "CorrExp B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
		submitter := p1.GetString("player1")
		match.Set("submitted_by", submitter)
		match.SetRaw("submitted_at", time.Now().Add(-25*time.Hour).UTC().Format(time.RFC3339))
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

// Playoff date validation (T6a)

func makeMatchWithRound(t testing.TB, app core.App, compID, p1ID, p2ID string, round int) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("matches")
	require.NoError(t, err)
	r := core.NewRecord(col)
	r.Set("competition", compID)
	r.Set("pair1", p1ID)
	r.Set("pair2", p2ID)
	r.Set("status", "pending")
	r.Set("round_number", round)
	require.NoError(t, app.Save(r))
	return r
}

func TestAdminOverride_PlayoffDateOrderRejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /match/{id}/admin-override rejects invalid playoff date order",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"orden del cuadro"},
	}
	var semifinalID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "PoA")
		p2 := makePairTB(tb, app, "PoB")
		p3 := makePairTB(tb, app, "PoC")
		p4 := makePairTB(tb, app, "PoD")
		comp := makeCompetitionTB(tb, app, "playoff", []*core.Record{p1, p2, p3, p4})

		qf := makeMatchWithRound(tb, app, comp.Id, p1.Id, p2.Id, 1)
		qf.Set("date", "2026-10-15")
		require.NoError(tb, app.Save(qf))

		sf := makeMatchWithRound(tb, app, comp.Id, p3.Id, p4.Id, 2)
		semifinalID = sf.Id

		s.URL = "/match/" + sf.Id + "/admin-override"
		s.Body = strings.NewReader("date=2026-10-10")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", semifinalID)
		require.NoError(tb, err)
		assert.Empty(tb, m.GetString("date"), "date must not be persisted on rejection")
	}
	s.Test(t)
}

func TestAdminOverride_PlayoffDateOrderAccepted(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/admin-override accepts valid playoff date order",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var semifinalID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "PvA")
		p2 := makePairTB(tb, app, "PvB")
		p3 := makePairTB(tb, app, "PvC")
		p4 := makePairTB(tb, app, "PvD")
		comp := makeCompetitionTB(tb, app, "playoff", []*core.Record{p1, p2, p3, p4})

		qf := makeMatchWithRound(tb, app, comp.Id, p1.Id, p2.Id, 1)
		qf.Set("date", "2026-10-15")
		require.NoError(tb, app.Save(qf))

		sf := makeMatchWithRound(tb, app, comp.Id, p3.Id, p4.Id, 2)
		semifinalID = sf.Id

		s.URL = "/match/" + sf.Id + "/admin-override"
		s.Body = strings.NewReader("date=2026-10-20")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", semifinalID)
		require.NoError(tb, err)
		assert.Contains(tb, m.GetString("date"), "2026-10-20")
	}
	s.Test(t)
}

func TestAdminOverride_LeagueDateNoValidation(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/admin-override league ignores date order",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "LgA")
		p2 := makePairTB(tb, app, "LgB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		m1 := makeMatchWithRound(tb, app, comp.Id, p1.Id, p2.Id, 1)
		m1.Set("date", "2026-10-20")
		require.NoError(tb, app.Save(m1))

		m2 := makeMatchWithRound(tb, app, comp.Id, p1.Id, p2.Id, 2)
		matchID = m2.Id

		s.URL = "/match/" + m2.Id + "/admin-override"
		s.Body = strings.NewReader("date=2026-10-10")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Contains(tb, m.GetString("date"), "2026-10-10")
	}
	s.Test(t)
}

func TestMatchCorrectWORejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /match/{id}/correct with WO score rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"incomparecencia"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "CorrWO A")
		p2 := makePairTB(tb, app, "CorrWO B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		submitter := p1.GetString("player1")
		match.Set("scores", "6-3 6-4")
		match.Set("submitted_by", submitter)
		match.Set("submitted_at", time.Now().UTC().Format(time.RFC3339))
		require.NoError(tb, app.Save(match))
		s.URL = "/match/" + match.Id + "/correct"
		s.Body = strings.NewReader("scores=WO")
		user, _ := app.FindRecordById("users", submitter)
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}
