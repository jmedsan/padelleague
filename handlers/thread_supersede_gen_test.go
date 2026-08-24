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

// makeProposal creates a scheduling proposal on a match. Returns the message record.
func makeProposal(tb testing.TB, app core.App, matchID, authorID string) *core.Record {
	tb.Helper()
	col, err := app.FindCollectionByNameOrId("match_messages")
	require.NoError(tb, err)
	msg := core.NewRecord(col)
	msg.Set("match", matchID)
	msg.Set("author", authorID)
	msg.Set("type", "scheduling_proposal")
	msg.Set("proposal_data", `{"date":"2026-09-20","time":"19:00","venue_name":"Club Test","venue_id":"","venue_text":""}`)
	msg.Set("proposal_status", "pending")
	require.NoError(tb, app.Save(msg))
	return msg
}

// --- Normal case: accepting a proposal supersedes the others, no admin notification ---

func TestAcceptProposalSupersedesOthers(t *testing.T) {
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "accepting proposal supersedes other pending proposals",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var otherMsgID, respondentID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Sup A")
		p2 := makePairTB(tb, app, "Sup B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		proposer := p1.GetString("player1")
		// Create two proposals from the same proposer
		prop1 := makeProposal(tb, app, match.Id, proposer)
		prop2 := makeProposal(tb, app, match.Id, proposer)
		otherMsgID = prop2.Id

		// Respondent (from p2) accepts prop1
		respondentID = p2.GetString("player1")
		respondent, _ := app.FindRecordById("users", respondentID)
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", match.Id, prop1.Id)
		s.Body = strings.NewReader("action=accept")
		hdrs := authHeaders(tb, respondent)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		// The other proposal must be superseded
		other, err := app.FindRecordById("match_messages", otherMsgID)
		require.NoError(tb, err)
		assert.Equal(tb, "superseded", other.GetString("proposal_status"),
			"non-accepted pending proposal must be superseded")

		// No admin notification should be created (normal case, no failures)
		admins, _ := app.FindRecordsByFilter("users", "role = 'admin'", "", 0, 0, nil)
		for _, admin := range admins {
			notifs, _ := app.FindRecordsByFilter("notifications",
				"user = {:uid} && title = 'Error al superseder propuestas'",
				"", 0, 0, map[string]any{"uid": admin.Id})
			assert.Equal(tb, 0, len(notifs),
				"no admin notification expected when supersede succeeds")
		}
	}
	s.Test(t)
}

// --- Failure case: supersede save fails → acceptance succeeds, admin notified ---
// Note: this test targets the S-4 fix. Before the fix, the error is swallowed
// and no admin notification is created. After the fix, supersedePending returns
// the failed IDs and the caller calls NotifyAdmins.
// The OnRecordUpdate hook fails only for the specific proposal being superseded,
// not for the acceptance save.

func TestAcceptProposalSupersedeFailureNotifiesAdmin(t *testing.T) {
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "supersede failure still accepts and notifies admin",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var acceptedMsgID, failMsgID, matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		// Create an admin user so NotifyAdmins has someone to notify
		makeAdminUserTB(tb, app)

		p1 := makePairTB(tb, app, "SupFail A")
		p2 := makePairTB(tb, app, "SupFail B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id

		proposer := p1.GetString("player1")
		prop1 := makeProposal(tb, app, match.Id, proposer)
		prop2 := makeProposal(tb, app, match.Id, proposer)
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
		hdrs := authHeaders(tb, respondent)
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
		// The exact title depends on the S-4 fix implementation. Check for
		// any admin notification that references the failure.
		admins, _ := app.FindRecordsByFilter("users", "role = 'admin'", "", 0, 0, nil)
		require.NotEmpty(tb, admins, "test requires at least one admin")
		adminNotifs, _ := app.FindRecordsByFilter("notifications",
			"user = {:uid} && type = 'admin_message'",
			"", 0, 0, map[string]any{"uid": admins[0].Id})
		assert.GreaterOrEqual(tb, len(adminNotifs), 1,
			"admin must receive a notification about the supersede failure")
	}
	s.Test(t)
}

// --- Multiple proposals: only non-accepted ones are superseded ---

func TestAcceptProposalOnlySupersedesPending(t *testing.T) {
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "accepting proposal only supersedes pending, not rejected ones",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var rejectedMsgID, pendingMsgID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "SupMix A")
		p2 := makePairTB(tb, app, "SupMix B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		proposer := p1.GetString("player1")
		propToAccept := makeProposal(tb, app, match.Id, proposer)
		propPending := makeProposal(tb, app, match.Id, proposer)
		propRejected := makeProposal(tb, app, match.Id, proposer)
		pendingMsgID = propPending.Id

		// Manually reject one proposal before accepting
		propRejected.Set("proposal_status", "rejected")
		require.NoError(tb, app.Save(propRejected))
		rejectedMsgID = propRejected.Id

		respondentID := p2.GetString("player1")
		respondent, _ := app.FindRecordById("users", respondentID)
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", match.Id, propToAccept.Id)
		s.Body = strings.NewReader("action=accept")
		hdrs := authHeaders(tb, respondent)
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
	s.Test(t)
}
