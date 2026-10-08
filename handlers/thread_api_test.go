package handlers_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/handlers"
	"padelleague/league"
)

func TestThreadMessages(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /match/{id}/thread-messages returns partial",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"hilo"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "TMsg A")
		p2 := handlers.MakePairTB(tb, app, "TMsg B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + match.Id + "/thread-messages"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestThreadMessages_DraftCalendarReturns404ForPlayer(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /match/{id}/thread-messages 404s for a player while the calendar is draft",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Partido no encontrado"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "DraftTMsg A")
		p2 := handlers.MakePairTB(tb, app, "DraftTMsg B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("calendar_status", "draft")
		require.NoError(tb, app.Save(comp))
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + match.Id + "/thread-messages"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestThread_DraftCalendarReturns404ForPlayer(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /match/{id}/thread 404s for a player while the calendar is draft",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Partido no encontrado"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "DraftThread A")
		p2 := handlers.MakePairTB(tb, app, "DraftThread B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("calendar_status", "draft")
		require.NoError(tb, app.Save(comp))
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + match.Id + "/thread"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestThreadMessages_DraftCalendarStillVisibleToAdmin(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /match/{id}/thread-messages stays visible to an admin while the calendar is draft",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"hilo"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "DraftAdminTMsg A")
		p2 := handlers.MakePairTB(tb, app, "DraftAdminTMsg B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("calendar_status", "draft")
		require.NoError(tb, app.Save(comp))
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + match.Id + "/thread-messages"
		admin := handlers.MakeAdminUserTB(tb, app)
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.Test(t)
}

func TestThreadPostProposal(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /match/{id}/thread/proposal creates proposal",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, proposerID, rival1, rival2 string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "Prop A")
		p2 := handlers.MakePairTB(tb, app, "Prop B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id
		proposerID = p1.GetString("player1")
		rival1 = p2.GetString("player1")
		rival2 = p2.GetString("player2")
		s.URL = "/match/" + match.Id + "/thread/proposal"
		s.Body = strings.NewReader("date=2027-09-15&time=18:00&venue_text=Club+Test")
		user, _ := app.FindRecordById("users", proposerID)
		hdrs := handlers.AuthHeaders(tb, user)
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

		want := league.Notification{
			Type:     "scheduling",
			Title:    "Propuesta de fecha",
			Body:     "Prop A P1 (Prop A) propone jugar el 15/09 a las 18:00 en Club Test",
			MatchID:  matchID,
			CompName: "Test Competition",
		}
		assertNotified(tb, app, rival1, want)
		assertNotified(tb, app, rival2, want)
		assertNotNotified(tb, app, proposerID, want.Title)
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) + "?scroll=mensajes" })
	s.Test(t)
}

func TestThreadRespondProposal(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /match/{id}/thread/proposal/{msgId}/respond accepts",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var msgID, matchID, proposerID, proposerPartnerID, accepterID, accepterPartnerID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "Resp A")
		p2 := handlers.MakePairTB(tb, app, "Resp B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id

		proposerID = p1.GetString("player1")
		proposerPartnerID = p1.GetString("player2")
		accepterID = p2.GetString("player1")
		accepterPartnerID = p2.GetString("player2")
		col, err := app.FindCollectionByNameOrId("match_messages")
		require.NoError(tb, err)
		msg := core.NewRecord(col)
		msg.Set("match", match.Id)
		msg.Set("author", proposerID)
		msg.Set("type", "scheduling_proposal")
		msg.Set("proposal_data", `{"date":"2027-09-15","time":"18:00","venue_name":"Club Test","venue_id":"","venue_text":""}`)
		msg.Set("proposal_status", "pending")
		require.NoError(tb, app.Save(msg))
		msgID = msg.Id

		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", match.Id, msg.Id)
		s.Body = strings.NewReader("action=accept")
		opponent, _ := app.FindRecordById("users", accepterID)
		hdrs := handlers.AuthHeaders(tb, opponent)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		msg, err := app.FindRecordById("match_messages", msgID)
		require.NoError(tb, err)
		assert.Equal(tb, "accepted", msg.GetString("proposal_status"))

		want := league.Notification{
			Type:     "scheduling",
			Title:    "Propuesta aceptada",
			Body:     "Resp B P1 (Resp B) aceptó tu propuesta para el 15/09 a las 18:00",
			MatchID:  matchID,
			CompName: "Test Competition",
		}
		assertNotified(tb, app, proposerID, want)
		assertNotified(tb, app, proposerPartnerID, want)
		assertNotified(tb, app, accepterPartnerID, want)
		assertNotNotified(tb, app, accepterID, want.Title)
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) + "?scroll=mensajes" })
	s.Test(t)
}

func TestThreadRespondProposalReject(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /match/{id}/thread/proposal/{msgId}/respond rejects",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var msgID, matchID, proposerID, proposerPartnerID, responderID, responderPartnerID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "RejA")
		p2 := handlers.MakePairTB(tb, app, "RejB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id

		proposerID = p1.GetString("player1")
		proposerPartnerID = p1.GetString("player2")
		responderID = p2.GetString("player1")
		responderPartnerID = p2.GetString("player2")
		col, err := app.FindCollectionByNameOrId("match_messages")
		require.NoError(tb, err)
		msg := core.NewRecord(col)
		msg.Set("match", match.Id)
		msg.Set("author", proposerID)
		msg.Set("type", "scheduling_proposal")
		msg.Set("proposal_data", `{"date":"2027-09-15","time":"18:00","venue_name":"Club","venue_id":"","venue_text":""}`)
		msg.Set("proposal_status", "pending")
		require.NoError(tb, app.Save(msg))
		msgID = msg.Id

		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", match.Id, msg.Id)
		s.Body = strings.NewReader("action=reject&rejection_reason=No+puedo&rejection_text=Tengo+trabajo")
		opponent, _ := app.FindRecordById("users", responderID)
		hdrs := handlers.AuthHeaders(tb, opponent)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		msg, err := app.FindRecordById("match_messages", msgID)
		require.NoError(tb, err)
		assert.Equal(tb, "rejected", msg.GetString("proposal_status"))
		assert.Equal(tb, "No puedo", msg.GetString("rejection_reason"))
		assert.Equal(tb, "Tengo trabajo", msg.GetString("rejection_text"))

		want := league.Notification{
			Type:     "scheduling",
			Title:    "Propuesta rechazada",
			Body:     "RejB P1 (RejB) ha rechazado tu propuesta: No puedo",
			MatchID:  matchID,
			CompName: "Test Competition",
		}
		assertNotified(tb, app, proposerID, want)
		assertNotified(tb, app, proposerPartnerID, want)
		assertNotified(tb, app, responderPartnerID, want)
		assertNotNotified(tb, app, responderID, want.Title)
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) + "?scroll=mensajes" })
	s.Test(t)
}

func TestThreadRespondOwnProposalRejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST respond to own proposal returns error",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"propia propuesta"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "OwnA")
		p2 := handlers.MakePairTB(tb, app, "OwnB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		proposer := p1.GetString("player1")
		col, err := app.FindCollectionByNameOrId("match_messages")
		require.NoError(tb, err)
		msg := core.NewRecord(col)
		msg.Set("match", match.Id)
		msg.Set("author", proposer)
		msg.Set("type", "scheduling_proposal")
		msg.Set("proposal_data", `{"date":"2027-09-15","time":"18:00","venue_name":"Club","venue_id":"","venue_text":""}`)
		msg.Set("proposal_status", "pending")
		require.NoError(tb, app.Save(msg))

		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", match.Id, msg.Id)
		s.Body = strings.NewReader("action=accept")
		user, _ := app.FindRecordById("users", proposer)
		hdrs := handlers.AuthHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestThreadEmptyPairsShowsMessage(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /match/{id}/thread with empty pairs shows pending message",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Parejas pendientes"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := handlers.MakeAdminUserTB(tb, app)
		comp := handlers.MakeCompetitionTB(tb, app, "league", nil)

		col, err := app.FindCollectionByNameOrId("matches")
		require.NoError(tb, err)
		match := core.NewRecord(col)
		match.Set("competition", comp.Id)
		match.Set("status", "pending")
		match.Set("round_number", 1)
		require.NoError(tb, app.Save(match))

		s.URL = "/match/" + match.Id + "/thread"
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.Test(t)
}

func TestPostMessageEmptyContentRejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /match/{id}/thread/message with empty content rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"vacío"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "EmptyA")
		p2 := handlers.MakePairTB(tb, app, "EmptyB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + match.Id + "/thread/message"
		s.Body = strings.NewReader("content=&type=chat")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := handlers.AuthHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestPostProposalMissingDateRejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /match/{id}/thread/proposal without date rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"obligatorias"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "NoDtA")
		p2 := handlers.MakePairTB(tb, app, "NoDtB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + match.Id + "/thread/proposal"
		s.Body = strings.NewReader("time=18:00&venue_text=Club")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := handlers.AuthHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}
