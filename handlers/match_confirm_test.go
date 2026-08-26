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

// Admin bypass: MatchConfirm

func TestMatchConfirmAdminNonParticipant_Succeeds(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "admin non-participant can confirm",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "ACnf A")
		p2 := makePairTB(tb, app, "ACnf B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		matchID = match.Id
		submitter := p1.GetString("player1")
		match.Set("scores", "6-3 6-4")
		match.Set("submitted_by", submitter)
		require.NoError(tb, app.Save(match))
		s.URL = "/match/" + match.Id + "/confirm"
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "final", m.GetString("status"))
	}
	s.Test(t)
}

func TestMatchConfirmNonParticipantPlayer_Refused(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "non-participant player cannot confirm",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"No eres participante"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		outsider := makeUserTB(tb, app, "Outsider Cnf", "")
		p1 := makePairTB(tb, app, "OCnf A")
		p2 := makePairTB(tb, app, "OCnf B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		submitter := p1.GetString("player1")
		match.Set("scores", "6-3 6-4")
		match.Set("submitted_by", submitter)
		require.NoError(tb, app.Save(match))
		s.URL = "/match/" + match.Id + "/confirm"
		s.Headers = authHeaders(tb, outsider)
	}
	s.Test(t)
}

// Admin bypass: MatchDispute

func TestMatchDisputeAdminNonParticipant_Succeeds(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "admin non-participant can dispute",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "ADsp A")
		p2 := makePairTB(tb, app, "ADsp B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		matchID = match.Id
		submitter := p1.GetString("player1")
		match.Set("scores", "6-3 6-4")
		match.Set("submitted_by", submitter)
		require.NoError(tb, app.Save(match))
		s.URL = "/match/" + match.Id + "/dispute"
		s.Body = strings.NewReader("dispute_notes=Admin+override")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "disputed", m.GetString("status"))
	}
	s.Test(t)
}

func TestMatchDisputeNonParticipantPlayer_Refused(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "non-participant player cannot dispute",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"No eres participante"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		outsider := makeUserTB(tb, app, "Outsider Dsp", "")
		p1 := makePairTB(tb, app, "ODsp A")
		p2 := makePairTB(tb, app, "ODsp B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		submitter := p1.GetString("player1")
		match.Set("scores", "6-3 6-4")
		match.Set("submitted_by", submitter)
		require.NoError(tb, app.Save(match))
		s.URL = "/match/" + match.Id + "/dispute"
		s.Body = strings.NewReader("dispute_notes=Outsider+attempt")
		hdrs := authHeaders(tb, outsider)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

// Correction window boundary (line 148)

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
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		matchID = match.Id
		submitter := p1.GetString("player1")
		match.Set("scores", "6-3 6-4")
		match.Set("submitted_by", submitter)
		// 23h59m ago — just under the boundary
		match.SetRaw("submitted_at", time.Now().Add(-23*time.Hour-59*time.Minute).UTC().Format(time.RFC3339))
		require.NoError(tb, app.Save(match))
		s.URL = "/match/" + match.Id + "/correct"
		s.Body = strings.NewReader("scores=6-4+6-3")
		user, _ := app.FindRecordById("users", submitter)
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "6-4 6-3", m.GetString("scores"), "corrected scores must be saved")
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
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		submitter := p1.GetString("player1")
		match.Set("scores", "6-3 6-4")
		match.Set("submitted_by", submitter)
		// 24h01m ago — just over the boundary
		match.SetRaw("submitted_at", time.Now().Add(-24*time.Hour-1*time.Minute).UTC().Format(time.RFC3339))
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

// Exactly 24h → refused (>= means equal is refused)
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
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		submitter := p1.GetString("player1")
		match.Set("scores", "6-3 6-4")
		match.Set("submitted_by", submitter)
		// Exactly 24h — the >= boundary
		match.SetRaw("submitted_at", time.Now().Add(-24*time.Hour).UTC().Format(time.RFC3339))
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

func TestMatchConfirmSameTeam(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /match/{id}/confirm by submitter team fails",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"No puedes confirmar"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "SameConf A")
		p2 := makePairTB(tb, app, "SameConf B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		submitter := p1.GetString("player1")
		m.Set("scores", "6-3 6-4")
		m.Set("submitted_by", submitter)
		require.NoError(tb, app.Save(m))

		s.URL = "/match/" + m.Id + "/confirm"
		user, _ := app.FindRecordById("users", submitter)
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}
