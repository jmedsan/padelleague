package handlers_test

import (
	"fmt"
	"net/http"
	"padelleague/handlers"
	"padelleague/league"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostMessageClampsType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, sent, want string
	}{
		{"chat is kept", "chat", "chat"},
		{"score discussion is kept", "score_discussion", "score_discussion"},
		{"proposal type is refused and becomes chat", "scheduling_proposal", "chat"},
		{"unknown type becomes chat", "banana", "chat"},
		{"empty type becomes chat", "", "chat"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var matchID string
			s := &tests.ApiScenario{
				TestAppFactory: handlers.TestAppFactory,
				Name:           "POST thread message type=" + tc.sent,
				Method:         http.MethodPost,
				Body:           strings.NewReader("content=hola&type=" + tc.sent),
				ExpectedStatus: 204,
			}
			s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				setupProductionRoutes(tb, app, e)
				p1 := handlers.MakePairTB(tb, app, "Clamp A "+tc.sent)
				p2 := handlers.MakePairTB(tb, app, "Clamp B "+tc.sent)
				comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
				match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
				matchID = match.Id
				s.URL = "/match/" + match.Id + "/thread/message"
				author, _ := app.FindRecordById("users", p1.GetString("player1"))
				hdrs := handlers.AuthHeaders(tb, author)
				hdrs["Content-Type"] = "application/x-www-form-urlencoded"
				s.Headers = hdrs
			}
			s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
				msgs, err := app.FindRecordsByFilter("match_messages",
					"match = {:m}", "", 0, 0, map[string]any{"m": matchID})
				require.NoError(tb, err)
				require.Len(tb, msgs, 1)
				assert.Equal(tb, tc.want, msgs[0].GetString("type"))
			}
			handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) + "#mensajes" })
			s.Test(t)
		})
	}
}

// Normal case: accepting a proposal supersedes the others, no admin notification

func TestAcceptProposalSupersedesOthers(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "accepting proposal supersedes other pending proposals",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var otherMsgID, respondentID, matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "Sup A")
		p2 := handlers.MakePairTB(tb, app, "Sup B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id

		proposer := p1.GetString("player1")
		// Create two proposals from the same proposer
		prop1 := handlers.MakeProposal(tb, app, match.Id, proposer)
		prop2 := handlers.MakeProposal(tb, app, match.Id, proposer)
		otherMsgID = prop2.Id

		// Respondent (from p2) accepts prop1
		respondentID = p2.GetString("player1")
		respondent, _ := app.FindRecordById("users", respondentID)
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", match.Id, prop1.Id)
		s.Body = strings.NewReader("action=accept")
		hdrs := handlers.AuthHeaders(tb, respondent)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		// The other proposal must be superseded
		other, err := app.FindRecordById("match_messages", otherMsgID)
		require.NoError(tb, err)
		assert.Equal(tb, "superseded", other.GetString("proposal_status"),
			"non-accepted pending proposal must be superseded")

		// Acceptance must create a scheduling_response timeline entry
		responses, _ := app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'scheduling_response'", "", 0, 0,
			map[string]any{"mid": matchID})
		require.Len(tb, responses, 2, "accept must create scheduling_response for accept + supersede")
		var acceptEntry *core.Record
		for _, r := range responses {
			if strings.Contains(r.GetString("content"), "aceptó la propuesta") {
				acceptEntry = r
			}
		}
		require.NotNil(tb, acceptEntry, "must have accept timeline entry")
		assert.Equal(tb, respondentID, acceptEntry.GetString("author"))

		// No admin notification should be created (normal case, no failures)
		admins, _ := app.FindRecordsByFilter("users", "roles ~ 'admin'", "", 0, 0, nil)
		for _, admin := range admins {
			notifs, _ := app.FindRecordsByFilter("notifications",
				"user = {:uid} && title = 'Error al superseder propuestas'",
				"", 0, 0, map[string]any{"uid": admin.Id})
			assert.Equal(tb, 0, len(notifs),
				"no admin notification expected when supersede succeeds")
		}
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) + "?scroll=mensajes" })
	s.Test(t)
}

// Failure case: supersede save fails → acceptance succeeds, admin notified
// Note: this test targets the S-4 fix. Before the fix, the error is swallowed
// and no admin notification is created. After the fix, supersedePending returns
// the failed IDs and the caller calls NotifyAdmins.
// The OnRecordUpdate hook fails only for the specific proposal being superseded,
// not for the acceptance save.

func TestAcceptProposalSupersedeFailureNotifiesAdmin(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "supersede failure still accepts and notifies admin",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var acceptedMsgID, failMsgID, matchID, adminID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		// Create an admin user so NotifyAdmins has someone to notify
		admin := handlers.MakeAdminUserTB(tb, app)
		adminID = admin.Id

		p1 := handlers.MakePairTB(tb, app, "SupFail A")
		p2 := handlers.MakePairTB(tb, app, "SupFail B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id

		proposer := p1.GetString("player1")
		prop1 := handlers.MakeProposal(tb, app, match.Id, proposer)
		prop2 := handlers.MakeProposal(tb, app, match.Id, proposer)
		acceptedMsgID = prop1.Id
		failMsgID = prop2.Id

		// Hook: fail save only for the proposal being superseded (prop2)
		app.OnRecordUpdate("match_messages").BindFunc(func(ev *core.RecordEvent) error {
			if ev.Record.Id == failMsgID &&
				ev.Record.GetString("proposal_status") == "superseded" {
				return fmt.Errorf("simulated DB failure for supersede")
			}
			return ev.Next()
		})

		respondentID := p2.GetString("player1")
		respondent, _ := app.FindRecordById("users", respondentID)
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", matchID, prop1.Id)
		s.Body = strings.NewReader("action=accept")
		hdrs := handlers.AuthHeaders(tb, respondent)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		// The accepted proposal must still be accepted
		accepted, err := app.FindRecordById("match_messages", acceptedMsgID)
		require.NoError(tb, err)
		assert.Equal(tb, "accepted", accepted.GetString("proposal_status"),
			"the acceptance must succeed even when supersede fails")

		// The failed proposal stays pending (save was blocked)
		failed, err := app.FindRecordById("match_messages", failMsgID)
		require.NoError(tb, err)
		assert.Equal(tb, "pending", failed.GetString("proposal_status"),
			"supersede-failed proposal must remain pending")

		// Admin notification must exist about the failure
		adminWant := league.Notification{
			Type:     "admin_message",
			Title:    "Propuestas pendientes no actualizadas",
			Body:     "El partido SupFail A vs SupFail B tiene propuestas que no se pudieron marcar como superadas. Revisa el hilo.",
			MatchID:  matchID,
			CompName: "Test Competition",
		}
		assertNotified(tb, app, adminID, adminWant)
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) + "?scroll=mensajes" })
	s.Test(t)
}

// Multiple proposals: only non-accepted ones are superseded

func TestAcceptProposalOnlySupersedesPending(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "accepting proposal only supersedes pending, not rejected ones",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var rejectedMsgID, pendingMsgID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "SupMix A")
		p2 := handlers.MakePairTB(tb, app, "SupMix B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		proposer := p1.GetString("player1")
		propToAccept := handlers.MakeProposal(tb, app, match.Id, proposer)
		propPending := handlers.MakeProposal(tb, app, match.Id, proposer)
		propRejected := handlers.MakeProposal(tb, app, match.Id, proposer)
		pendingMsgID = propPending.Id

		// Manually reject one proposal before accepting
		propRejected.Set("proposal_status", "rejected")
		require.NoError(tb, app.Save(propRejected))
		rejectedMsgID = propRejected.Id

		respondentID := p2.GetString("player1")
		respondent, _ := app.FindRecordById("users", respondentID)
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", match.Id, propToAccept.Id)
		s.Body = strings.NewReader("action=accept")
		hdrs := handlers.AuthHeaders(tb, respondent)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		// The pending one must be superseded
		pending, err := app.FindRecordById("match_messages", pendingMsgID)
		require.NoError(tb, err)
		assert.Equal(tb, "superseded", pending.GetString("proposal_status"))

		// The rejected one must remain rejected (not touched by supersede)
		rejected, err := app.FindRecordById("match_messages", rejectedMsgID)
		require.NoError(tb, err)
		assert.Equal(tb, "rejected", rejected.GetString("proposal_status"),
			"already-rejected proposal must not change status")
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) + "?scroll=mensajes" })
	s.Test(t)
}

func TestRejectProposalCreatesSchedulingResponse(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "rejecting proposal creates scheduling_response entry",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, respondentID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "Rej A")
		p2 := handlers.MakePairTB(tb, app, "Rej B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id

		proposer := p1.GetString("player1")
		prop := handlers.MakeProposal(tb, app, match.Id, proposer)

		respondentID = p2.GetString("player1")
		respondent, _ := app.FindRecordById("users", respondentID)
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", match.Id, prop.Id)
		s.Body = strings.NewReader("action=reject&rejection_reason=No+puedo+ese+d%C3%ADa")
		hdrs := handlers.AuthHeaders(tb, respondent)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		responses, _ := app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'scheduling_response'", "", 0, 0,
			map[string]any{"mid": matchID})
		require.Len(tb, responses, 1, "reject must create one scheduling_response")
		assert.Equal(tb, respondentID, responses[0].GetString("author"))
		assert.Contains(tb, responses[0].GetString("content"), "rechazó la propuesta")
		assert.Contains(tb, responses[0].GetString("content"), "No puedo ese día")
		// The reason must live on THIS entry's own record (rejection_text),
		// not just the original proposal's — timelineNote/mc.msg reads the
		// entry being rendered, and the original proposal is a different
		// match_messages record than this response entry.
		assert.Equal(tb, "No puedo ese día", responses[0].GetString("rejection_text"),
			"the response entry itself must carry the note shown in the timeline")
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) + "?scroll=mensajes" })
	s.Test(t)
}

// TestRejectProposal_TimelineRendersRejectionNote verifies the rendered
// timeline HTML shows the rejection reason as a Note under the frozen
// "Rechazada" entry — the same dateBox/resultBox Note pattern already used
// for a disputed result's rejection reason (component-modes.md).
func TestRejectProposal_TimelineRendersRejectionNote(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "rejected scheduling proposal shows its reason as a timeline note",
		Method:         http.MethodGet,
		ExpectedStatus: 200,
		ExpectedContent: []string{
			"Rechazada",
			"No puedo ese día",
		},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "RejNote A")
		p2 := handlers.MakePairTB(tb, app, "RejNote B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		col, _ := app.FindCollectionByNameOrId("match_messages")
		msg := core.NewRecord(col)
		msg.Set("match", match.Id)
		msg.Set("author", p2.GetString("player1"))
		msg.Set("type", "scheduling_response")
		msg.Set("content", "rechazó la propuesta de Test Player: No puedo ese día")
		msg.Set("rejection_text", "No puedo ese día")
		msg.Set("proposal_data", map[string]any{
			"action": "reject", "date": "2027-09-20", "time": "19:00", "venue_name": "Club Test",
		})
		require.NoError(tb, app.Save(msg))

		s.URL = "/match/" + match.Id + "/thread-messages"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestWithdrawProposal_AuthorWithdraws(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "author withdraws own pending proposal",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var propID, matchID, authorID, rival1, rival2 string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "Wd A")
		p2 := handlers.MakePairTB(tb, app, "Wd B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id

		authorID = p1.GetString("player1")
		rival1 = p2.GetString("player1")
		rival2 = p2.GetString("player2")
		prop := handlers.MakeProposal(tb, app, match.Id, authorID)
		propID = prop.Id

		author, _ := app.FindRecordById("users", authorID)
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/withdraw", match.Id, prop.Id)
		hdrs := handlers.AuthHeaders(tb, author)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		msg, err := app.FindRecordById("match_messages", propID)
		require.NoError(tb, err)
		assert.Equal(tb, "withdrawn", msg.GetString("proposal_status"))

		responses, _ := app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'scheduling_response'", "", 0, 0,
			map[string]any{"mid": matchID})
		require.Len(tb, responses, 1, "withdraw must create a timeline entry")
		assert.Contains(tb, responses[0].GetString("content"), "retiró su propuesta")

		want := league.Notification{
			Type:     "scheduling",
			Title:    "Propuesta retirada",
			Body:     "Wd A P1 (Wd A) ha retirado su propuesta de fecha",
			MatchID:  matchID,
			CompName: "Test Competition",
		}
		assertNotified(tb, app, rival1, want)
		assertNotified(tb, app, rival2, want)
		assertNotNotified(tb, app, authorID, want.Title)
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) + "?scroll=mensajes" })
	s.Test(t)
}

func TestWithdrawProposal_RivalFails(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "rival cannot withdraw proposal",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Solo tu pareja puede retirar esta propuesta"},
	}
	var propID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "WdNA A")
		p2 := handlers.MakePairTB(tb, app, "WdNA B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		authorID := p1.GetString("player1")
		prop := handlers.MakeProposal(tb, app, match.Id, authorID)
		propID = prop.Id

		nonAuthorID := p2.GetString("player1")
		nonAuthor, _ := app.FindRecordById("users", nonAuthorID)
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/withdraw", match.Id, prop.Id)
		hdrs := handlers.AuthHeaders(tb, nonAuthor)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		msg, err := app.FindRecordById("match_messages", propID)
		require.NoError(tb, err)
		assert.Equal(tb, "pending", msg.GetString("proposal_status"),
			"proposal must remain pending when non-author tries to withdraw")
	}
	s.Test(t)
}

func TestWithdrawProposal_AcceptedFails(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "cannot withdraw already-accepted proposal",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Solo se pueden retirar propuestas pendientes"},
	}
	var propID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "WdAcc A")
		p2 := handlers.MakePairTB(tb, app, "WdAcc B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		authorID := p1.GetString("player1")
		prop := handlers.MakeProposal(tb, app, match.Id, authorID)
		prop.Set("proposal_status", "accepted")
		require.NoError(tb, app.Save(prop))
		propID = prop.Id

		author, _ := app.FindRecordById("users", authorID)
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/withdraw", match.Id, prop.Id)
		hdrs := handlers.AuthHeaders(tb, author)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		msg, err := app.FindRecordById("match_messages", propID)
		require.NoError(tb, err)
		assert.Equal(tb, "accepted", msg.GetString("proposal_status"),
			"proposal must remain accepted when withdraw is attempted")
	}
	s.Test(t)
}

func TestWithdrawProposal_PartnerWithdraws(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "proposer partner can withdraw the proposal",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var propID, matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "WdPart A")
		p2 := handlers.MakePairTB(tb, app, "WdPart B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id

		authorID := p1.GetString("player1")
		prop := handlers.MakeProposal(tb, app, match.Id, authorID)
		propID = prop.Id

		partnerID := p1.GetString("player2")
		partner, _ := app.FindRecordById("users", partnerID)
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/withdraw", match.Id, prop.Id)
		hdrs := handlers.AuthHeaders(tb, partner)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		msg, err := app.FindRecordById("match_messages", propID)
		require.NoError(tb, err)
		assert.Equal(tb, "withdrawn", msg.GetString("proposal_status"),
			"partner must be able to withdraw their team's proposal")

		responses, _ := app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'scheduling_response'", "", 0, 0,
			map[string]any{"mid": matchID})
		require.Len(tb, responses, 1, "withdraw must create a timeline entry")
		assert.Contains(tb, responses[0].GetString("content"), "retiró su propuesta")
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) + "?scroll=mensajes" })
	s.Test(t)
}

func TestThreadWithMessages(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /match/{id}/thread with messages renders them",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"thread-messages"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "ThrMsg A")
		p2 := handlers.MakePairTB(tb, app, "ThrMsg B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		// Add chat messages
		col, _ := app.FindCollectionByNameOrId("match_messages")
		for i, content := range []string{"Hola equipo", "Cuando jugamos?"} {
			msg := core.NewRecord(col)
			msg.Set("match", match.Id)
			msg.Set("author", p1.GetString("player1"))
			msg.Set("type", "chat")
			msg.Set("content", content)
			require.NoError(tb, app.Save(msg))
			_ = i
		}

		// Add a scheduling proposal
		proposal := core.NewRecord(col)
		proposal.Set("match", match.Id)
		proposal.Set("author", p1.GetString("player1"))
		proposal.Set("type", "scheduling_proposal")
		proposal.Set("proposal_data", map[string]any{
			"date": "2027-09-20", "time": "19:00", "venue_name": "Club Padel",
		})
		proposal.Set("proposal_status", "pending")
		require.NoError(tb, app.Save(proposal))

		s.URL = "/match/" + match.Id + "/thread"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestThreadMessagesWithData(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /match/{id}/thread-messages with messages",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Hola equipo"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "ThrData A")
		p2 := handlers.MakePairTB(tb, app, "ThrData B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		col, _ := app.FindCollectionByNameOrId("match_messages")
		msg := core.NewRecord(col)
		msg.Set("match", match.Id)
		msg.Set("author", p1.GetString("player1"))
		msg.Set("type", "chat")
		msg.Set("content", "Hola equipo")
		require.NoError(tb, app.Save(msg))

		// Add a proposal too
		proposal := core.NewRecord(col)
		proposal.Set("match", match.Id)
		proposal.Set("author", p2.GetString("player1"))
		proposal.Set("type", "scheduling_proposal")
		proposal.Set("proposal_data", map[string]any{
			"date": "2027-09-20", "time": "19:00", "venue_name": "Club",
		})
		proposal.Set("proposal_status", "accepted")
		require.NoError(tb, app.Save(proposal))

		s.URL = "/match/" + match.Id + "/thread-messages"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestThreadMessages_ResultEventRendersAsSystemLine(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "result_event renders as system line, not chat bubble",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"thread-messages"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "SysLine A")
		p2 := handlers.MakePairTB(tb, app, "SysLine B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")

		col, _ := app.FindCollectionByNameOrId("match_messages")
		msg := core.NewRecord(col)
		msg.Set("match", match.Id)
		msg.Set("author", p1.GetString("player1"))
		msg.Set("type", "result_event")
		msg.Set("content", "registró el resultado")
		require.NoError(tb, app.Save(msg))

		s.URL = "/match/" + match.Id + "/thread-messages"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)

		s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
			body := handlers.ReadBody(tb, res)
			// Flat timeline: the event is a plain read-only row (not a chat bubble),
			// inside the timeline log.
			assert.Contains(tb, body, `id="thread-timeline"`, "event renders in the timeline log")
			assert.Contains(tb, body, "registró el resultado", "event line content")
			assert.NotContains(tb, body, `chat-bubble`, "result_event must not render as chat bubble")
		}
	}
	s.Test(t)
}

func TestThreadMessages_SystemAuthorRendersAsSistema(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "empty-author message renders author as Sistema with full result line",
		Method:         http.MethodGet,
		ExpectedStatus: 200,
		ExpectedContent: []string{
			"Sistema",
			"confirmó resultado",
			"Confirmado",
			"6-3 6-4",
		},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "SysAuth A")
		p2 := handlers.MakePairTB(tb, app, "SysAuth B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")

		col, _ := app.FindCollectionByNameOrId("match_messages")
		msg := core.NewRecord(col)
		msg.Set("match", match.Id)
		msg.Set("type", "result_response")
		msg.Set("content", "Resultado aceptado: 6-3 6-4")
		msg.Set("proposal_data", map[string]any{"action": "accept", "scores": "6-3 6-4"})
		require.NoError(tb, app.Save(msg))

		s.URL = "/match/" + match.Id + "/thread-messages"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

// Cluster: Playoff thread (T6b)

func TestThread_PlayoffHidesProposal(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "playoff thread hides propose-date and shows admin-set date",
		Method:         http.MethodGet,
		ExpectedStatus: 200,
		ExpectedContent: []string{
			"15/10/2027",
			"20:00",
			"Padel 360",
		},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "POThr1")
		p2 := handlers.MakePairTB(tb, app, "POThr2")
		comp := handlers.MakeCompetitionTB(tb, app, "playoff", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		match.Set("date", "2027-10-15")
		match.Set("time", "20:00")
		match.Set("club", "Padel 360")
		require.NoError(tb, app.Save(match))

		s.URL = "/match/" + match.Id + "/thread"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.NotContains(tb, body, "Proponer fecha", "playoff must not show propose-date form")
		assert.Contains(tb, body, "15/10/2027", "playoff must show admin-set date")
	}
	s.Test(t)
}

func TestThread_PlayoffNoDateShowsPending(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "playoff thread with no date shows pending message",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"pendiente de asignación"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "PONoDt1")
		p2 := handlers.MakePairTB(tb, app, "PONoDt2")
		comp := handlers.MakeCompetitionTB(tb, app, "playoff", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		s.URL = "/match/" + match.Id + "/thread"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.NotContains(tb, body, "Proponer fecha", "playoff must not show propose-date form")
	}
	s.Test(t)
}

func TestAcceptProposalBlocksSecondAcceptance(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "accepting a second proposal is blocked when one is already accepted",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Ya hay una propuesta aceptada"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "Dup A")
		p2 := handlers.MakePairTB(tb, app, "Dup B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		prop1 := handlers.MakeProposal(tb, app, match.Id, p1.GetString("player1"))
		prop1.Set("proposal_status", "accepted")
		require.NoError(tb, app.Save(prop1))

		prop2 := handlers.MakeProposal(tb, app, match.Id, p1.GetString("player1"))

		respondent, _ := app.FindRecordById("users", p2.GetString("player1"))
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", match.Id, prop2.Id)
		s.Body = strings.NewReader("action=accept")
		hdrs := handlers.AuthHeaders(tb, respondent)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestAcceptProposal_SetsStatusScheduled(t *testing.T) {
	t.Parallel()
	var matchID string
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "accepting a proposal sets match status to scheduled",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "SchedA")
		p2 := handlers.MakePairTB(tb, app, "SchedB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id

		prop := handlers.MakeProposal(tb, app, match.Id, p1.GetString("player1"))
		respondent, _ := app.FindRecordById("users", p2.GetString("player1"))
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", match.Id, prop.Id)
		s.Body = strings.NewReader("action=accept")
		hdrs := handlers.AuthHeaders(tb, respondent)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		match, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, league.StatusScheduled, match.GetString("status"))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) + "?scroll=mensajes" })
	s.Test(t)
}

func TestAcceptProposal_ClearsMatchReminders(t *testing.T) {
	t.Parallel()
	var matchID string
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "accepting a proposal clears match reminders",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "RemA")
		p2 := handlers.MakePairTB(tb, app, "RemB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		require.NoError(tb, app.Save(match))
		matchID = match.Id
		insertMatchReminder(tb, app, match.Id, p1.GetString("player1"))

		prop := handlers.MakeProposal(tb, app, match.Id, p1.GetString("player1"))
		respondent, _ := app.FindRecordById("users", p2.GetString("player1"))
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", match.Id, prop.Id)
		s.Body = strings.NewReader("action=accept")
		hdrs := handlers.AuthHeaders(tb, respondent)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		rows, _ := app.FindRecordsByFilter("match_reminders", "match = {:mid}", "", 0, 0, map[string]any{"mid": matchID})
		assert.Empty(tb, rows, "accepting a new date must clear stale match reminders")
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) + "?scroll=mensajes" })
	s.Test(t)
}

func TestReschedule_SupersedesOldAccepted(t *testing.T) {
	t.Parallel()
	var matchID, oldPropID string
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "rescheduling a scheduled match supersedes the old accepted proposal",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "Resched A")
		p2 := handlers.MakePairTB(tb, app, "Resched B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
		matchID = match.Id

		oldProp := handlers.MakeProposal(tb, app, match.Id, p1.GetString("player1"))
		oldProp.Set("proposal_status", "accepted")
		require.NoError(tb, app.Save(oldProp))
		oldPropID = oldProp.Id

		newProp := handlers.MakeProposal(tb, app, match.Id, p2.GetString("player1"))
		respondent, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", match.Id, newProp.Id)
		s.Body = strings.NewReader("action=accept")
		hdrs := handlers.AuthHeaders(tb, respondent)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		old, err := app.FindRecordById("match_messages", oldPropID)
		require.NoError(tb, err)
		assert.Equal(tb, "superseded", old.GetString("proposal_status"))

		match, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, league.StatusScheduled, match.GetString("status"))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) + "?scroll=mensajes" })
	s.Test(t)
}

func TestScheduledMatch_AllowsNewProposal(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "scheduled match allows posting a new proposal",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "SchdPrA")
		p2 := handlers.MakePairTB(tb, app, "SchdPrB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")

		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.URL = fmt.Sprintf("/match/%s/thread/proposal", match.Id)
		s.Body = strings.NewReader("date=2026-12-01&time=20:00&venue_id=")
		hdrs := handlers.AuthHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) + "?scroll=mensajes" })
	s.Test(t)
}

func TestPostMessage_AdminNonParticipant_Succeeds(t *testing.T) {
	t.Parallel()
	var matchID string
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "admin non-participant can post a thread message",
		Method:         http.MethodPost,
		Body:           strings.NewReader("content=Mensaje+del+admin&type=chat"),
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := handlers.MakeAdminUserTB(tb, app)
		p1 := handlers.MakePairTB(tb, app, "AdmMsg A")
		p2 := handlers.MakePairTB(tb, app, "AdmMsg B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id
		s.URL = "/match/" + match.Id + "/thread/message"
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		msgs, err := app.FindRecordsByFilter("match_messages",
			"match = {:m}", "", 0, 0, map[string]any{"m": matchID})
		require.NoError(tb, err)
		require.Len(tb, msgs, 1)
		assert.Equal(tb, "Mensaje del admin", msgs[0].GetString("content"))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) + "#mensajes" })
	s.Test(t)
}

func TestThread_AdminNonParticipant_SeesComposeBox(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:     handlers.TestAppFactory,
		Name:               "admin non-participant sees compose box and admin note",
		Method:             http.MethodGet,
		ExpectedStatus:     200,
		ExpectedContent:    []string{"Escribe un mensaje...", "Escribiendo como administrador"},
		NotExpectedContent: []string{"Proponer fecha", "solo lectura"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := handlers.MakeAdminUserTB(tb, app)
		p1 := handlers.MakePairTB(tb, app, "AdmThr A")
		p2 := handlers.MakePairTB(tb, app, "AdmThr B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + match.Id + "/thread"
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.Test(t)
}

func TestPostMessage_NonParticipantNonAdmin_Rejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "non-participant non-admin cannot post",
		Method:          http.MethodPost,
		Body:            strings.NewReader("content=intruso&type=chat"),
		ExpectedStatus:  200,
		ExpectedContent: []string{"No eres participante"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		outsider := handlers.MakeUserTB(tb, app, "Outsider Msg", "")
		p1 := handlers.MakePairTB(tb, app, "OutMsg A")
		p2 := handlers.MakePairTB(tb, app, "OutMsg B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + match.Id + "/thread/message"
		hdrs := handlers.AuthHeaders(tb, outsider)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestPostMessage_AdminNotifiesBothPairs(t *testing.T) {
	t.Parallel()
	var matchID string
	var p1Player1, p1Player2, p2Player1, p2Player2 string
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "admin message notifies both pairs",
		Method:         http.MethodPost,
		Body:           strings.NewReader("content=Aviso+importante&type=chat"),
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := handlers.MakeAdminUserTB(tb, app)
		p1 := handlers.MakePairTB(tb, app, "BothN A")
		p2 := handlers.MakePairTB(tb, app, "BothN B")
		p1Player1 = p1.GetString("player1")
		p1Player2 = p1.GetString("player2")
		p2Player1 = p2.GetString("player1")
		p2Player2 = p2.GetString("player2")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id
		s.URL = "/match/" + match.Id + "/thread/message"
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		want := league.Notification{
			Type:     "message",
			Title:    "Nuevo mensaje",
			Body:     "Admin escribió: Aviso importante",
			MatchID:  matchID,
			CompName: "Test Competition",
		}
		assertNotified(tb, app, p1Player1, want)
		assertNotified(tb, app, p1Player2, want)
		assertNotified(tb, app, p2Player1, want)
		assertNotified(tb, app, p2Player2, want)
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) + "#mensajes" })
	s.Test(t)
}

func TestPostMessage_PlayerEmailsRivalsAndOwnPartner(t *testing.T) {
	t.Parallel()
	var wantEmails []string
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "player chat emails both rivals and own partner, never the author",
		Method:         http.MethodPost,
		Body:           strings.NewReader("content=Hola+a+todos&type=chat"),
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		app.Settings().SMTP.Enabled = true
		app.Settings().SMTP.Host = "smtp.test.local"
		app.Settings().SMTP.Port = 587
		require.NoError(tb, app.Save(app.Settings()))

		p1 := handlers.MakePairTB(tb, app, "ChatMail A")
		p2 := handlers.MakePairTB(tb, app, "ChatMail B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		author, err := app.FindRecordById("users", p1.GetString("player1"))
		require.NoError(tb, err)
		for _, id := range []string{p1.GetString("player2"), p2.GetString("player1"), p2.GetString("player2")} {
			u, err := app.FindRecordById("users", id)
			require.NoError(tb, err)
			wantEmails = append(wantEmails, u.Email())
		}
		s.URL = "/match/" + match.Id + "/thread/message"
		hdrs := handlers.AuthHeaders(tb, author)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		var got []string
		for _, msg := range app.TestMailer.Messages() {
			require.Len(tb, msg.To, 1)
			got = append(got, msg.To[0].Address)
		}
		assert.ElementsMatch(tb, wantEmails, got)
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) + "#mensajes" })
	s.Test(t)
}

func TestFinalMatchThreadAcceptsPost(t *testing.T) {
	t.Parallel()

	t.Run("participant can post in final match thread", func(t *testing.T) {
		t.Parallel()
		s := &tests.ApiScenario{
			TestAppFactory: handlers.TestAppFactory,
			Name:           "participant posts in final match thread",
			Method:         http.MethodPost,
			ExpectedStatus: 204,
		}
		s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
			setupProductionRoutes(tb, app, e)
			p1 := handlers.MakePairTB(tb, app, "FinalT A")
			p2 := handlers.MakePairTB(tb, app, "FinalT B")
			comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
			m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")
			m.Set("scores", "6-3 6-4")
			m.Set("winner", p1.Id)
			require.NoError(tb, app.Save(m))
			s.URL = "/match/" + m.Id + "/thread/message"
			player, err := app.FindRecordById("users", p1.GetString("player1"))
			require.NoError(tb, err)
			s.Body = strings.NewReader("content=Post+after+final")
			hdrs := handlers.AuthHeaders(tb, player)
			hdrs["Content-Type"] = "application/x-www-form-urlencoded"
			s.Headers = hdrs
		}
		s.Test(t)
	})

	t.Run("admin can post in final match thread", func(t *testing.T) {
		t.Parallel()
		s := &tests.ApiScenario{
			TestAppFactory: handlers.TestAppFactory,
			Name:           "admin posts in final match thread",
			Method:         http.MethodPost,
			ExpectedStatus: 204,
		}
		s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
			setupProductionRoutes(tb, app, e)
			admin := handlers.MakeAdminUserTB(tb, app)
			p1 := handlers.MakePairTB(tb, app, "FinalTA A")
			p2 := handlers.MakePairTB(tb, app, "FinalTA B")
			comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
			m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")
			m.Set("scores", "6-3 6-4")
			m.Set("winner", p1.Id)
			require.NoError(tb, app.Save(m))
			s.URL = "/match/" + m.Id + "/thread/message"
			s.Body = strings.NewReader("content=Admin+post+after+final")
			hdrs := handlers.AuthHeaders(tb, admin)
			hdrs["Content-Type"] = "application/x-www-form-urlencoded"
			s.Headers = hdrs
		}
		handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) + "#mensajes" })
		s.Test(t)
	})
}

func TestThread_VenueNotPreSelected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "proposal form does not pre-select any venue",
		Method:          http.MethodGet,
		URL:             "/placeholder",
		ExpectedStatus:  200,
		ExpectedContent: []string{"thread-messages"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		handlers.MakeVenueTB(tb, app, "SomeVenue")
		p1 := handlers.MakePairTB(tb, app, "NoPreA")
		p2 := handlers.MakePairTB(tb, app, "NoPreB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		current := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + current.Id + "/thread"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(_ testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(t, res)
		assert.NotContains(t, body, "selected>SomeVenue")
	}
	s.Test(t)
}

func TestThreadMessages_AdminActionRendersAsSystemLine(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "admin_action renders via resultEventLine, not chatMessage",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"thread-messages"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "AdminAct A")
		p2 := handlers.MakePairTB(tb, app, "AdminAct B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")

		col, _ := app.FindCollectionByNameOrId("match_messages")
		msg := core.NewRecord(col)
		msg.Set("match", match.Id)
		msg.Set("author", p1.GetString("player1"))
		msg.Set("type", "admin_action")
		msg.Set("content", "Admin corrigió el resultado")
		require.NoError(tb, app.Save(msg))

		s.URL = "/match/" + match.Id + "/thread-messages"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.Contains(tb, body, `id="thread-timeline"`, "admin_action renders in the flat timeline")
		assert.Contains(tb, body, "Admin corrigió el resultado")
		assert.NotContains(tb, body, "chat-bubble", "admin_action must not render as chat")
	}
	s.Test(t)
}

func TestThreadMessages_AllTypesRenderCorrectSubDefine(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "each message type dispatches to its sub-define",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"thread-messages"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "SubDef A")
		p2 := handlers.MakePairTB(tb, app, "SubDef B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		col, _ := app.FindCollectionByNameOrId("match_messages")

		chat := core.NewRecord(col)
		chat.Set("match", match.Id)
		chat.Set("author", p1.GetString("player1"))
		chat.Set("type", "chat")
		chat.Set("content", "Hola desde chat")
		require.NoError(tb, app.Save(chat))

		event := core.NewRecord(col)
		event.Set("match", match.Id)
		event.Set("author", p1.GetString("player1"))
		event.Set("type", "result_event")
		event.Set("content", "registró resultado: 6-2 6-3")
		require.NoError(tb, app.Save(event))

		proposal := core.NewRecord(col)
		proposal.Set("match", match.Id)
		proposal.Set("author", p2.GetString("player1"))
		proposal.Set("type", "scheduling_proposal")
		proposal.Set("proposal_data", map[string]any{
			"date": "2027-10-05", "time": "20:00", "venue_name": "Padel 360",
		})
		proposal.Set("proposal_status", "pending")
		require.NoError(tb, app.Save(proposal))

		s.URL = "/match/" + match.Id + "/thread-messages"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		// Flat timeline: every type renders as a plain read-only row in the log —
		// no chat bubbles, no centered pills, no nested boxes.
		assert.Contains(tb, body, "Padel 360", "proposal shows venue name")
		assert.Contains(tb, body, "20:00", "proposal shows time")
		assert.Regexp(tb, `\w+ \(SubDef B\)`, body, "proposal author shows PlayerName (PairName)")
		assert.Contains(tb, body, `id="thread-timeline"`, "entries render in the flat timeline")
		assert.Contains(tb, body, "registró resultado: 6-2 6-3", "event content shown")
		assert.Contains(tb, body, "Hola desde chat", "chat content shown as a flat row")
		assert.NotContains(tb, body, "chat-bubble", "no chat bubbles in the flat timeline")
	}
	s.Test(t)
}

func TestPlayerProposalBlockedOnFinalizedComp(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /match/{id}/propose blocked for player on finalized competition",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"finalizada o archivada"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "PB A")
		p2 := handlers.MakePairTB(tb, app, "PB B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("finalized", true)
		require.NoError(tb, app.Save(comp))
		v := handlers.MakeVenueTB(tb, app, "Blocked Venue")
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + m.Id + "/thread/proposal"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := handlers.AuthHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
		s.Body = strings.NewReader("date=2027-10-01&time=20:00&venue=" + v.Id)
	}
	s.Test(t)
}

func TestAcceptResultProposalFinalizesMatch(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "accepting result proposal finalizes the match",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, proposalID, proposerID, proposerPartnerID, respondentID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "RA A")
		p2 := handlers.MakePairTB(tb, app, "RA B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
		matchID = match.Id

		proposerID = p1.GetString("player1")
		proposerPartnerID = p1.GetString("player2")
		proposal := handlers.MakeResultProposal(tb, app, match.Id, proposerID, "6-3 6-4")
		proposalID = proposal.Id

		respondentID = p2.GetString("player1")
		respondent, _ := app.FindRecordById("users", respondentID)
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", match.Id, proposal.Id)
		s.Body = strings.NewReader("action=accept")
		hdrs := handlers.AuthHeaders(tb, respondent)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, league.StatusFinal, m.GetString("status"), "match must be finalized")
		assert.Equal(tb, "6-3 6-4", m.GetString("scores"))
		assert.NotEmpty(tb, m.GetString("winner"), "winner must be determined")

		prop, _ := app.FindRecordById("match_messages", proposalID)
		assert.Equal(tb, "accepted", prop.GetString("proposal_status"))

		responses, _ := app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'result_response'", "", 0, 0,
			map[string]any{"mid": matchID})
		require.Len(tb, responses, 1, "one result_response must exist")
		assert.Equal(tb, proposalID, responses[0].GetString("parent"), "response must reference the proposal")

		want := league.Notification{
			Type:     "general",
			Title:    "Resultado confirmado",
			Body:     "RA B P1 (RA B) ha confirmado el resultado",
			MatchID:  matchID,
			CompName: "Test Competition",
		}
		assertNotified(tb, app, proposerID, want)
		assertNotified(tb, app, proposerPartnerID, want)
		assertNotNotified(tb, app, respondentID, want.Title)
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) + "?scroll=mensajes" })
	s.Test(t)
}

func TestAcceptResultSupersedesSiblings(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "accepting result proposal supersedes sibling pending proposals",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, siblingID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "AS A")
		p2 := handlers.MakePairTB(tb, app, "AS B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
		matchID = match.Id

		proposer := p1.GetString("player1")
		proposal := handlers.MakeResultProposal(tb, app, match.Id, proposer, "6-3 6-4")

		col, _ := app.FindCollectionByNameOrId("match_messages")
		sibling := core.NewRecord(col)
		sibling.Set("match", matchID)
		sibling.Set("author", p2.GetString("player1"))
		sibling.Set("type", "result_submission")
		sibling.Set("proposal_status", "pending")
		sibling.Set("proposal_data", `{"scores":"6-4 6-3"}`)
		sibling.Set("content", "6-4 6-3")
		require.NoError(tb, app.Save(sibling))
		siblingID = sibling.Id

		respondent, _ := app.FindRecordById("users", p2.GetString("player1"))
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", match.Id, proposal.Id)
		s.Body = strings.NewReader("action=accept")
		hdrs := handlers.AuthHeaders(tb, respondent)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, _ := app.FindRecordById("matches", matchID)
		assert.Equal(tb, league.StatusFinal, m.GetString("status"))

		sib, _ := app.FindRecordById("match_messages", siblingID)
		assert.Equal(tb, "superseded", sib.GetString("proposal_status"),
			"sibling pending result proposal must be superseded after accept")

		remaining, _ := app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'result_submission' && proposal_status = 'pending'",
			"", 0, 0, map[string]any{"mid": matchID})
		assert.Empty(tb, remaining, "zero pending result proposals must remain after accept")
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) + "?scroll=mensajes" })
	s.Test(t)
}

func TestRejectResultProposalRequiresCounter(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "rejecting result proposal creates counter-proposal",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, proposalID, respondentID, proposerPairID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "RR A")
		p2 := handlers.MakePairTB(tb, app, "RR B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
		matchID = match.Id
		proposerPairID = p1.Id

		proposer := p1.GetString("player1")
		proposal := handlers.MakeResultProposal(tb, app, match.Id, proposer, "6-3 6-4")
		proposalID = proposal.Id

		respondent, _ := app.FindRecordById("users", p2.GetString("player1"))
		respondentID = respondent.Id
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", match.Id, proposal.Id)
		s.Body = strings.NewReader("action=reject&counter_scores=6-4+6-3")
		hdrs := handlers.AuthHeaders(tb, respondent)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "scheduled", m.GetString("status"), "match must stay pre-score")

		prop, _ := app.FindRecordById("match_messages", proposalID)
		assert.Equal(tb, "superseded", prop.GetString("proposal_status"),
			"rejected proposal must be superseded")

		responses, _ := app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'result_response' && parent = {:pid}",
			"", 0, 0,
			map[string]any{"mid": matchID, "pid": proposalID})
		require.Len(tb, responses, 1, "one result_response must exist for the rejection")

		counters, _ := app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'result_submission' && author = {:uid} && proposal_status = 'pending'",
			"", 0, 0,
			map[string]any{"mid": matchID, "uid": respondentID})
		require.Len(tb, counters, 1, "a counter-proposal must exist")
		assert.Equal(tb, "6-4 6-3", handlers.ParseProposalData(counters[0].GetString("proposal_data")).Scores)

		// The counter-proposal itself must be a visible timeline entry, not
		// just a DB record — the project rule is that every result-changing action has
		// to render as a timeline line.
		h := handlers.NewThreadHandler(handlers.ThreadDeps{App: app})
		td := h.BuildThreadData(m, matchID, "", 0, true)
		var found bool
		for _, entry := range td.Timeline {
			if entry.Kind == "proposal" && entry.Score == "6-4 6-3" {
				found = true
				break
			}
		}
		assert.True(tb, found, "counter-proposal must appear in the timeline with its own score")

		// Timeline entry must exist for the rejection
		timeline, _ := app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'result_response' && parent = {:pid}",
			"", 0, 0,
			map[string]any{"mid": matchID, "pid": proposalID})
		require.Len(tb, timeline, 1, "rejection must create a result_response timeline entry")
		assert.Contains(tb, timeline[0].GetString("content"), "Resultado rechazado")

		proposerPair, _ := app.FindRecordById("pairs", proposerPairID)
		want := league.Notification{
			Type:     "quorum_request",
			Title:    "Contrapropuesta recibida",
			Body:     "RR B P1 (RR B) ha propuesto un resultado alternativo",
			MatchID:  matchID,
			CompName: "Test Competition",
		}
		assertNotified(tb, app, proposerPair.GetString("player1"), want)
		assertNotified(tb, app, proposerPair.GetString("player2"), want)
		assertNotNotified(tb, app, respondentID, want.Title)
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) + "?scroll=mensajes" })
	s.Test(t)
}

func TestRejectResultProposalEmptyCounterRejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "rejecting result proposal without counter_scores returns error",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Debes indicar el marcador"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "RE A")
		p2 := handlers.MakePairTB(tb, app, "RE B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")

		proposer := p1.GetString("player1")
		proposal := handlers.MakeResultProposal(tb, app, match.Id, proposer, "6-3 6-4")

		respondent, _ := app.FindRecordById("users", p2.GetString("player1"))
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", match.Id, proposal.Id)
		s.Body = strings.NewReader("action=reject&counter_scores=")
		hdrs := handlers.AuthHeaders(tb, respondent)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

// --- buildThreadData unit tests (match-thread-split spec) ---

func TestRejectAndCounterPropose_InvalidDateLeavesProposalPending(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "reject-and-counter with invalid date leaves original proposal pending",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Fecha y hora son obligatorias"},
	}
	var propID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "RC Inv A")
		p2 := handlers.MakePairTB(tb, app, "RC Inv B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		proposerID := p1.GetString("player1")
		prop := handlers.MakeProposal(tb, app, match.Id, proposerID)
		propID = prop.Id

		// p2 player responds with reject-and-counter but omits date/time
		respondentID := p2.GetString("player1")
		respondent, _ := app.FindRecordById("users", respondentID)
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/reject-and-counter", match.Id, prop.Id)
		s.Body = strings.NewReader("rejection_reason=No+puedo&date=&time=")
		hdrs := handlers.AuthHeaders(tb, respondent)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		// Original proposal must still be pending — no DB write happened
		msg, err := app.FindRecordById("match_messages", propID)
		require.NoError(tb, err)
		assert.Equal(tb, "pending", msg.GetString("proposal_status"), "proposal must remain pending after validation failure")

		// No new proposal must have been created in this match
		newProps, _ := app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'scheduling_proposal' && proposal_status = 'pending'", "", 0, 0,
			map[string]any{"mid": msg.GetString("match")})
		require.Len(tb, newProps, 1, "only the original proposal should exist")
		assert.Equal(tb, propID, newProps[0].Id, "the surviving pending proposal must be the original")
	}
	s.Test(t)
}

func TestRejectAndCounterPropose_ValidDateRejectsAndCreatesNew(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "reject-and-counter with valid date rejects old and creates new proposal",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var propID, matchID, proposerID, proposerPartnerID, respondentID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "RC Val A")
		p2 := handlers.MakePairTB(tb, app, "RC Val B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id

		proposerID = p1.GetString("player1")
		proposerPartnerID = p1.GetString("player2")
		prop := handlers.MakeProposal(tb, app, match.Id, proposerID)
		propID = prop.Id

		respondentID = p2.GetString("player1")
		respondent, _ := app.FindRecordById("users", respondentID)
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/reject-and-counter", match.Id, prop.Id)
		s.Body = strings.NewReader("rejection_reason=No+puedo&date=2030-06-15&time=18:00")
		hdrs := handlers.AuthHeaders(tb, respondent)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		// Original proposal must be rejected
		orig, err := app.FindRecordById("match_messages", propID)
		require.NoError(tb, err)
		assert.Equal(tb, "rejected", orig.GetString("proposal_status"), "original proposal must be rejected")

		// A new pending proposal must exist
		newProps, _ := app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'scheduling_proposal' && proposal_status = 'pending'", "", 0, 0,
			map[string]any{"mid": matchID})
		require.Len(tb, newProps, 1, "one new pending proposal must be created")
		assert.NotEqual(tb, propID, newProps[0].Id, "new proposal must be a different record")

		rejectWant := league.Notification{
			Type:     "scheduling",
			Title:    "Propuesta rechazada",
			Body:     "RC Val B P1 (RC Val B) ha rechazado tu propuesta: No puedo",
			MatchID:  matchID,
			CompName: "Test Competition",
		}
		assertNotified(tb, app, proposerID, rejectWant)
		assertNotified(tb, app, proposerPartnerID, rejectWant)
		assertNotNotified(tb, app, respondentID, rejectWant.Title)

		counterWant := league.Notification{
			Type:     "scheduling",
			Title:    "Propuesta de fecha",
			Body:     "RC Val B P1 (RC Val B) propone jugar el 15/06 a las 18:00",
			MatchID:  matchID,
			CompName: "Test Competition",
		}
		assertNotified(tb, app, proposerID, counterWant)
		assertNotified(tb, app, proposerPartnerID, counterWant)
		assertNotNotified(tb, app, respondentID, counterWant.Title)
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) + "?scroll=mensajes" })
	s.Test(t)
}
