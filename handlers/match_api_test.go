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
		assert.Equal(tb, "confirmed", m.GetString("status"))
		assert.Equal(tb, "6-3 6-4", m.GetString("scores"))
		assert.Equal(tb, submitterID, m.GetString("submitted_by"))
		assert.Equal(tb, "/match/"+matchID, res.Header.Get("HX-Redirect"))
	}
	s.Test(t)
}

func TestMatchConfirm(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/confirm by opponent returns 204",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, opponentID, p1ID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Confirm A")
		p2 := makePairTB(tb, app, "Confirm B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		matchID = match.Id
		p1ID = p1.Id
		submitter := p1.GetString("player1")
		match.Set("scores", "6-3 6-4")
		match.Set("submitted_by", submitter)
		require.NoError(tb, app.Save(match))
		s.URL = "/match/" + match.Id + "/confirm"
		opponent, _ := app.FindRecordById("users", p2.GetString("player1"))
		opponentID = opponent.Id
		s.Headers = authHeaders(tb, opponent)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "final", m.GetString("status"))
		assert.Equal(tb, opponentID, m.GetString("confirmed_by"))
		assert.Equal(tb, p1ID, m.GetString("winner"))
		assert.Equal(tb, "/match/"+matchID, res.Header.Get("HX-Redirect"))
	}
	s.Test(t)
}

func TestMatchDispute(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/dispute with notes returns 204",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, opponentID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Dispute A")
		p2 := makePairTB(tb, app, "Dispute B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		matchID = match.Id
		submitter := p1.GetString("player1")
		match.Set("scores", "6-3 6-4")
		match.Set("submitted_by", submitter)
		require.NoError(tb, app.Save(match))
		s.URL = "/match/" + match.Id + "/dispute"
		s.Body = strings.NewReader("dispute_notes=El+marcador+es+incorrecto")
		opponent, _ := app.FindRecordById("users", p2.GetString("player1"))
		opponentID = opponent.Id
		hdrs := authHeaders(tb, opponent)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "disputed", m.GetString("status"))
		assert.Equal(tb, "El marcador es incorrecto", m.GetString("dispute_notes"))
		assert.Equal(tb, opponentID, m.GetString("disputed_by"))
		assert.Equal(tb, "/match/"+matchID, res.Header.Get("HX-Redirect"))
	}
	s.Test(t)
}

func TestMatchWalkover(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/walkover returns 204",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, p1ID, submitterID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "WO A")
		p2 := makePairTB(tb, app, "WO B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id
		p1ID = p1.Id
		submitterID = p1.GetString("player1")
		s.URL = "/match/" + match.Id + "/walkover"
		s.Body = strings.NewReader("absent_team=2")
		user, _ := app.FindRecordById("users", submitterID)
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "final", m.GetString("status"))
		assert.Equal(tb, "WO", m.GetString("scores"))
		assert.Equal(tb, p1ID, m.GetString("winner"))
		assert.Equal(tb, submitterID, m.GetString("submitted_by"))
		assert.Equal(tb, "/match/"+matchID, res.Header.Get("HX-Redirect"))
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
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		matchID = match.Id
		submitter := p1.GetString("player1")
		match.Set("scores", "6-3 6-4")
		match.Set("submitted_by", submitter)
		match.Set("submitted_at", time.Now().UTC().Format(time.RFC3339))
		require.NoError(tb, app.Save(match))
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
		assert.Equal(tb, "confirmed", m.GetString("status"))
		assert.Equal(tb, "6-4 6-3", m.GetString("scores"))
		assert.Empty(tb, m.GetString("confirmed_by"), "confirmed_by must be cleared on correction")
		assert.NotEmpty(tb, m.GetString("submitted_at"), "submitted_at must be refreshed")
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
		s.URL = "/match/" + match.Id + "/submit"
		s.Body = strings.NewReader("scores=WO")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestMatchConfirmOwnResultRejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /match/{id}/confirm by submitter rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"propio resultado"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "ConfOwn A")
		p2 := makePairTB(tb, app, "ConfOwn B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		submitter := p1.GetString("player1")
		match.Set("scores", "6-3 6-4")
		match.Set("submitted_by", submitter)
		require.NoError(tb, app.Save(match))
		s.URL = "/match/" + match.Id + "/confirm"
		user, _ := app.FindRecordById("users", submitter)
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestMatchDisputeOwnResultRejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /match/{id}/dispute by submitter rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"propio resultado"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "DispOwn A")
		p2 := makePairTB(tb, app, "DispOwn B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		submitter := p1.GetString("player1")
		match.Set("scores", "6-3 6-4")
		match.Set("submitted_by", submitter)
		require.NoError(tb, app.Save(match))
		s.URL = "/match/" + match.Id + "/dispute"
		s.Body = strings.NewReader("dispute_notes=Incorrecto")
		user, _ := app.FindRecordById("users", submitter)
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
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		submitter := p1.GetString("player1")
		match.Set("scores", "6-3 6-4")
		match.Set("submitted_by", submitter)
		match.SetRaw("submitted_at", time.Now().Add(-25*time.Hour).UTC().Format(time.RFC3339))
		require.NoError(tb, app.Save(match))
		s.URL = "/match/" + match.Id + "/correct"
		s.Body = strings.NewReader("scores=6-4+6-3")
		user, _ := app.FindRecordById("users", submitter)
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestMatchWalkoverAbsentTeam1(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/walkover absent_team=1 pair2 wins",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, p2ID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "WO1 A")
		p2 := makePairTB(tb, app, "WO1 B")
		p2ID = p2.Id
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id
		s.URL = "/match/" + match.Id + "/walkover"
		s.Body = strings.NewReader("absent_team=1")
		user, _ := app.FindRecordById("users", p2.GetString("player1"))
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "final", m.GetString("status"))
		assert.Equal(tb, "WO", m.GetString("scores"))
		assert.Equal(tb, p2ID, m.GetString("winner"))
	}
	s.Test(t)
}

// --- Playoff date validation (T6a) ---

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
		ExpectedContent: []string{"las fechas de la ronda"},
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
