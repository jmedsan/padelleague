package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"
)

func TestThreadMessages(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "GET /match/{id}/thread-messages returns partial",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"hilo"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "TMsg A")
		p2 := makePairTB(tb, app, "TMsg B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + match.Id + "/thread-messages"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestThreadPostProposal(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /match/{id}/thread/proposal creates proposal",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Prop A")
		p2 := makePairTB(tb, app, "Prop B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + match.Id + "/thread/proposal"
		s.Body = strings.NewReader("date=2026-09-15&time=18:00&venue_text=Club+Test")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestThreadRespondProposal(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /match/{id}/thread/proposal/{msgId}/respond accepts",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Resp A")
		p2 := makePairTB(tb, app, "Resp B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		// Create a proposal message from p1's player
		proposer := p1.GetString("player1")
		col, err := app.FindCollectionByNameOrId("match_messages")
		require.NoError(tb, err)
		msg := core.NewRecord(col)
		msg.Set("match", match.Id)
		msg.Set("author", proposer)
		msg.Set("type", "scheduling_proposal")
		msg.Set("proposal_data", `{"date":"2026-09-15","time":"18:00","venue_name":"Club Test","venue_id":"","venue_text":""}`)
		msg.Set("proposal_status", "pending")
		require.NoError(tb, app.Save(msg))

		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", match.Id, msg.Id)
		s.Body = strings.NewReader("action=accept")
		opponent, _ := app.FindRecordById("users", p2.GetString("player1"))
		hdrs := authHeaders(tb, opponent)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}
