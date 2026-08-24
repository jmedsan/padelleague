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
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Prop A")
		p2 := makePairTB(tb, app, "Prop B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id
		s.URL = "/match/" + match.Id + "/thread/proposal"
		s.Body = strings.NewReader("date=2026-09-15&time=18:00&venue_text=Club+Test")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		msgs, err := app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'scheduling_proposal'", "", 0, 0,
			map[string]any{"mid": matchID})
		require.NoError(tb, err)
		assert.Equal(tb, 1, len(msgs))
		assert.Equal(tb, "pending", msgs[0].GetString("proposal_status"))
	}
	s.Test(t)
}

func TestThreadRespondProposal(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /match/{id}/thread/proposal/{msgId}/respond accepts",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var msgID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Resp A")
		p2 := makePairTB(tb, app, "Resp B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

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
		msgID = msg.Id

		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", match.Id, msg.Id)
		s.Body = strings.NewReader("action=accept")
		opponent, _ := app.FindRecordById("users", p2.GetString("player1"))
		hdrs := authHeaders(tb, opponent)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		msg, err := app.FindRecordById("match_messages", msgID)
		require.NoError(tb, err)
		assert.Equal(tb, "accepted", msg.GetString("proposal_status"))
	}
	s.Test(t)
}

func TestThreadRespondProposalReject(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /match/{id}/thread/proposal/{msgId}/respond rejects",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var msgID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "RejA")
		p2 := makePairTB(tb, app, "RejB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		proposer := p1.GetString("player1")
		col, err := app.FindCollectionByNameOrId("match_messages")
		require.NoError(tb, err)
		msg := core.NewRecord(col)
		msg.Set("match", match.Id)
		msg.Set("author", proposer)
		msg.Set("type", "scheduling_proposal")
		msg.Set("proposal_data", `{"date":"2026-09-15","time":"18:00","venue_name":"Club","venue_id":"","venue_text":""}`)
		msg.Set("proposal_status", "pending")
		require.NoError(tb, app.Save(msg))
		msgID = msg.Id

		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", match.Id, msg.Id)
		s.Body = strings.NewReader("action=reject&rejection_reason=No+puedo&rejection_text=Tengo+trabajo")
		opponent, _ := app.FindRecordById("users", p2.GetString("player1"))
		hdrs := authHeaders(tb, opponent)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		msg, err := app.FindRecordById("match_messages", msgID)
		require.NoError(tb, err)
		assert.Equal(tb, "rejected", msg.GetString("proposal_status"))
		assert.Equal(tb, "No puedo", msg.GetString("rejection_reason"))
		assert.Equal(tb, "Tengo trabajo", msg.GetString("rejection_text"))
	}
	s.Test(t)
}

func TestThreadRespondOwnProposalRejected(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "POST respond to own proposal returns error",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"propia propuesta"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "OwnA")
		p2 := makePairTB(tb, app, "OwnB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		proposer := p1.GetString("player1")
		col, err := app.FindCollectionByNameOrId("match_messages")
		require.NoError(tb, err)
		msg := core.NewRecord(col)
		msg.Set("match", match.Id)
		msg.Set("author", proposer)
		msg.Set("type", "scheduling_proposal")
		msg.Set("proposal_data", `{"date":"2026-09-15","time":"18:00","venue_name":"Club","venue_id":"","venue_text":""}`)
		msg.Set("proposal_status", "pending")
		require.NoError(tb, app.Save(msg))

		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", match.Id, msg.Id)
		s.Body = strings.NewReader("action=accept")
		user, _ := app.FindRecordById("users", proposer)
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestThreadEmptyPairsShowsMessage(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "GET /match/{id}/thread with empty pairs shows pending message",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Parejas pendientes"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		comp := makeCompetitionTB(tb, app, "league", nil)

		col, err := app.FindCollectionByNameOrId("matches")
		require.NoError(tb, err)
		match := core.NewRecord(col)
		match.Set("competition", comp.Id)
		match.Set("status", "pending")
		match.Set("round_number", 1)
		require.NoError(tb, app.Save(match))

		s.URL = "/match/" + match.Id + "/thread"
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestPostMessageEmptyContentRejected(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "POST /match/{id}/thread/message with empty content rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"vacío"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "EmptyA")
		p2 := makePairTB(tb, app, "EmptyB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + match.Id + "/thread/message"
		s.Body = strings.NewReader("content=&type=chat")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestPostProposalMissingDateRejected(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "POST /match/{id}/thread/proposal without date rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"obligatorias"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "NoDtA")
		p2 := makePairTB(tb, app, "NoDtB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + match.Id + "/thread/proposal"
		s.Body = strings.NewReader("time=18:00&venue_text=Club")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}
