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

func TestMatchSubmitScore(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /match/{id}/submit with valid score returns 204 + HX-Redirect",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Submit A")
		p2 := makePairTB(tb, app, "Submit B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id
		s.URL = "/match/" + match.Id + "/submit"
		s.Body = strings.NewReader("scores=6-3+6-4")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "confirmed", m.GetString("status"))
		assert.Equal(tb, "6-3 6-4", m.GetString("scores"))
	}
	s.Test(t)
}

func TestMatchConfirm(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /match/{id}/confirm by opponent returns 204",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Confirm A")
		p2 := makePairTB(tb, app, "Confirm B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		matchID = match.Id
		submitter := p1.GetString("player1")
		match.Set("scores", "6-3 6-4")
		match.Set("submitted_by", submitter)
		require.NoError(tb, app.Save(match))
		s.URL = "/match/" + match.Id + "/confirm"
		opponent, _ := app.FindRecordById("users", p2.GetString("player1"))
		s.Headers = authHeaders(tb, opponent)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "final", m.GetString("status"))
	}
	s.Test(t)
}

func TestMatchDispute(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /match/{id}/dispute with notes returns 204",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
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
		hdrs := authHeaders(tb, opponent)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "disputed", m.GetString("status"))
		assert.Equal(tb, "El marcador es incorrecto", m.GetString("dispute_notes"))
	}
	s.Test(t)
}

func TestMatchWalkover(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /match/{id}/walkover returns 204",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "WO A")
		p2 := makePairTB(tb, app, "WO B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + match.Id + "/walkover"
		s.Body = strings.NewReader("absent_team=2")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestMatchCorrect(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /match/{id}/correct within 24h returns 204",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Correct A")
		p2 := makePairTB(tb, app, "Correct B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		submitter := p1.GetString("player1")
		match.Set("scores", "6-3 6-4")
		match.Set("submitted_by", submitter)
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

func TestMatchEdit(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /match/{id}/edit changes date returns 204",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Edit A")
		p2 := makePairTB(tb, app, "Edit B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + match.Id + "/edit"
		s.Body = strings.NewReader("date=2026-09-15&time=18:00")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestMatchThread(t *testing.T) {
	s := &tests.ApiScenario{
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
	s := &tests.ApiScenario{
		Name:           "POST /match/{id}/thread/message sends a message",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Msg A")
		p2 := makePairTB(tb, app, "Msg B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + match.Id + "/thread/message"
		s.Body = strings.NewReader("content=Hola+equipo&type=chat")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}
