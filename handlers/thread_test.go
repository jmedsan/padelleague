package handlers

import (
	"fmt"
	"net/http"
	"padelleague/league"
	"padelleague/notify"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseProposalData_ValidJSON(t *testing.T) {
	t.Parallel()
	raw := `{"date":"2026-10-15","time":"19:30","venue_id":"abc123","venue_name":"Padel 360","venue_text":""}`
	pd := ParseProposalData(raw)
	require.NotNil(t, pd)
	assert.Equal(t, "2026-10-15", pd.Date)
	assert.Equal(t, "19:30", pd.Time)
	assert.Equal(t, "abc123", pd.VenueID)
	assert.Equal(t, "Padel 360", pd.VenueName)
}

func TestParseProposalData_Map(t *testing.T) {
	t.Parallel()
	raw := map[string]any{
		"date":       "2026-11-01",
		"time":       "20:00",
		"venue_id":   "",
		"venue_name": "Mi casa",
		"venue_text": "Mi casa",
	}
	pd := ParseProposalData(raw)
	require.NotNil(t, pd)
	assert.Equal(t, "2026-11-01", pd.Date)
	assert.Equal(t, "Mi casa", pd.VenueName)
}

func TestParseProposalData_Nil(t *testing.T) {
	t.Parallel()
	pd := ParseProposalData(nil)
	assert.Nil(t, pd)
}

func TestParseProposalData_EmptyString(t *testing.T) {
	t.Parallel()
	pd := ParseProposalData("")
	assert.Nil(t, pd)
}

func TestParseProposalData_Malformed(t *testing.T) {
	t.Parallel()
	pd := ParseProposalData("not json")
	assert.Nil(t, pd)
}

func TestProposalActions(t *testing.T) {
	t.Parallel()

	const prop = "scheduling_proposal"

	cases := []struct {
		name                    string
		msgType, matchStatus    string
		sameTeamOrOutsider      bool
		proposalStatus          string
		wantRespond, wantChange bool
	}{
		{"opponent, pending", prop, league.StatusPending, false, "pending", true, false},
		{"opponent, accepted", prop, league.StatusPending, false, "accepted", false, true},
		{"opponent, rejected", prop, league.StatusPending, false, "rejected", false, true},
		{"own proposal, pending", prop, league.StatusPending, true, "pending", false, false},
		{"own proposal, accepted", prop, league.StatusPending, true, "accepted", false, false},
		{"outsider", prop, league.StatusPending, true, "pending", false, false},
		{"confirmed match", prop, league.StatusConfirmed, false, "pending", false, false},
		{"disputed match", prop, league.StatusDisputed, false, "pending", false, false},
		{"final match", prop, league.StatusFinal, false, "pending", false, false},
		{"chat message", "chat", league.StatusPending, false, "pending", false, false},
		{"score discussion", "score_discussion", league.StatusPending, false, "pending", false, false},
		{"superseded proposal", prop, league.StatusPending, false, "superseded", false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			respond, change := proposalActions(tc.msgType, tc.matchStatus, tc.sameTeamOrOutsider, tc.proposalStatus)
			assert.Equal(t, tc.wantRespond, respond, "canRespond")
			assert.Equal(t, tc.wantChange, change, "canChange")
		})
	}
}

// playerTeamOf decides which side of a match a user is on. Returning the
// wrong side, or zero for a real participant, silently removes every action
// that participation grants.
func TestPlayerTeamOf(t *testing.T) {
	t.Parallel()

	p1 := []string{"alice", "bob"}
	p2 := []string{"carol", "dave"}

	cases := []struct {
		name string
		uid  string
		want int
	}{
		{"first player of pair 1", "alice", 1},
		{"second player of pair 1", "bob", 1},
		{"first player of pair 2", "carol", 2},
		{"second player of pair 2", "dave", 2},
		{"not in either pair", "eve", 0},
		{"empty user id", "", 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, playerTeamOf(tc.uid, p1, p2))
		})
	}
}

// A player listed in both pairs should resolve to pair 1, since that is the
// first match found. Pinning it stops the order silently reversing.
func TestPlayerTeamOfPrefersPairOne(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 1, playerTeamOf("alice", []string{"alice"}, []string{"alice"}))
}

// resolveVenue turns a form's venue selection into a stored venue id and a
// display name. The "otro" option means the user typed a free-text venue, so
// no id is stored and their text is kept verbatim.
func TestResolveVenue(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	h := &ThreadHandler{app: app}

	col, err := app.FindCollectionByNameOrId("venues")
	require.NoError(t, err)
	venue := core.NewRecord(col)
	venue.Set("name", "Padel 360")
	require.NoError(t, app.Save(venue))

	t.Run("known venue keeps its id and uses its name", func(t *testing.T) {
		id, name := h.resolveVenue(venue.Id, "ignored text")
		assert.Equal(t, venue.Id, id)
		assert.Equal(t, "Padel 360", name)
	})

	t.Run("otro keeps the typed text and stores no id", func(t *testing.T) {
		id, name := h.resolveVenue("otro", "Club del barrio")
		assert.Empty(t, id)
		assert.Equal(t, "Club del barrio", name)
	})

	t.Run("empty selection keeps the typed text", func(t *testing.T) {
		id, name := h.resolveVenue("", "Otro sitio")
		assert.Empty(t, id)
		assert.Equal(t, "Otro sitio", name)
	})

	t.Run("unknown venue id falls back to the typed text", func(t *testing.T) {
		id, name := h.resolveVenue("nonexistent", "Respaldo")
		assert.Empty(t, id)
		assert.Equal(t, "Respaldo", name)
	})
}

// A scheduling proposal must notify the opposing pair, never the proposer's
// own partner. Getting the side wrong means the person who needs to respond
// is never told, and the proposal sits unanswered.
func TestProposalNotifiesOpposingPair(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	p1 := makePairTB(t, app, "Notif P1")
	p2 := makePairTB(t, app, "Notif P2")
	comp := makeCompetitionTB(t, app, "league", []*core.Record{p1, p2})
	match := makeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "pending")

	notifier := notify.NewNotifier(app, "", "")
	h := &ThreadHandler{app: app, notifier: notifier}

	// A member of pair 1 proposes; pair 2 must hear about it.
	author := p1.GetString("player1")
	h.notifyProposal(match, 1, proposalNotice{AuthorID: author, Date: "2026-09-20", Time: "19:00", VenueName: "Club Test"})

	notifs, err := app.FindRecordsByFilter("notifications",
		"type = 'scheduling'", "", 0, 0, nil)
	require.NoError(t, err)
	require.NotEmpty(t, notifs, "a scheduling notification should exist")

	got := map[string]bool{}
	for _, n := range notifs {
		got[n.GetString("user")] = true
	}

	for _, uid := range league.PlayersForPair(app, p2.Id) {
		assert.True(t, got[uid], "opposing pair member %s should be notified", uid)
	}
	for _, uid := range league.PlayersForPair(app, p1.Id) {
		assert.False(t, got[uid], "proposer's own pair member %s must not be notified", uid)
	}
}

// The mirror case: a member of pair 2 proposes, so pair 1 is notified.
func TestProposalFromPairTwoNotifiesPairOne(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	p1 := makePairTB(t, app, "Mirror P1")
	p2 := makePairTB(t, app, "Mirror P2")
	comp := makeCompetitionTB(t, app, "league", []*core.Record{p1, p2})
	match := makeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "pending")

	h := &ThreadHandler{app: app, notifier: notify.NewNotifier(app, "", "")}
	h.notifyProposal(match, 2, proposalNotice{AuthorID: p2.GetString("player1"), Date: "2026-09-20", Time: "19:00", VenueName: "Club Test"})

	notifs, err := app.FindRecordsByFilter("notifications", "type = 'scheduling'", "", 0, 0, nil)
	require.NoError(t, err)
	got := map[string]bool{}
	for _, n := range notifs {
		got[n.GetString("user")] = true
	}
	for _, uid := range league.PlayersForPair(app, p1.Id) {
		assert.True(t, got[uid], "pair 1 member %s should be notified", uid)
	}
	for _, uid := range league.PlayersForPair(app, p2.Id) {
		assert.False(t, got[uid], "proposer's own pair member %s must not be notified", uid)
	}
}

func TestProposalSendsEmail(t *testing.T) {
	t.Parallel()
	testApp, err := tests.NewTestApp(tmplDataDir)
	require.NoError(t, err)
	t.Cleanup(testApp.Cleanup)

	testApp.Settings().SMTP.Enabled = true
	testApp.Settings().SMTP.Host = "smtp.test.local"
	testApp.Settings().SMTP.Port = 587
	require.NoError(t, testApp.Save(testApp.Settings()))

	p1 := makePairTB(t, testApp, "Email P1")
	p2 := makePairTB(t, testApp, "Email P2")
	comp := makeCompetitionTB(t, testApp, "league", []*core.Record{p1, p2})
	match := makeMatchTB(t, testApp, comp.Id, p1.Id, p2.Id, "pending")

	h := &ThreadHandler{app: testApp, notifier: notify.NewNotifier(testApp, "", "")}
	h.notifyProposal(match, 1, proposalNotice{
		AuthorID: p1.GetString("player1"), Date: "2026-09-20", Time: "19:00", VenueName: "Club Test",
	})

	assert.Greater(t, testApp.TestMailer.TotalSend(), 0, "scheduling proposal must send email")
	found := false
	for _, msg := range testApp.TestMailer.Messages() {
		if msg.Subject == "Propuesta de fecha" {
			found = true
			break
		}
	}
	assert.True(t, found, "email subject should be 'Propuesta de fecha'")
}

// PostMessage accepts only chat and score_discussion; anything else falls
// back to chat. Without that clamp a caller could post a message typed as a
// scheduling proposal, which would render accept/reject buttons on something
// carrying no proposal data.
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
				TestAppFactory: testAppFactory,
				Name:           "POST thread message type=" + tc.sent,
				Method:         http.MethodPost,
				Body:           strings.NewReader("content=hola&type=" + tc.sent),
				ExpectedStatus: 204,
			}
			s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				setupAllRoutes(tb, app, e)
				p1 := makePairTB(tb, app, "Clamp A "+tc.sent)
				p2 := makePairTB(tb, app, "Clamp B "+tc.sent)
				comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
				match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
				matchID = match.Id
				s.URL = "/match/" + match.Id + "/thread/message"
				author, _ := app.FindRecordById("users", p1.GetString("player1"))
				hdrs := authHeaders(tb, author)
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
			s.Test(t)
		})
	}
}

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

// Normal case: accepting a proposal supersedes the others, no admin notification

func TestAcceptProposalSupersedesOthers(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "accepting proposal supersedes other pending proposals",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var otherMsgID, respondentID, matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Sup A")
		p2 := makePairTB(tb, app, "Sup B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id

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

		// Acceptance must create a scheduling_response timeline entry
		responses, _ := app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'scheduling_response'", "", 0, 0,
			map[string]any{"mid": matchID})
		require.Len(tb, responses, 1, "accept must create one scheduling_response")
		assert.Equal(tb, respondentID, responses[0].GetString("author"))
		assert.Contains(tb, responses[0].GetString("content"), "aceptó la propuesta")

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
		admins, _ := app.FindRecordsByFilter("users", "roles ~ 'admin'", "", 0, 0, nil)
		require.NotEmpty(tb, admins, "test requires at least one admin")
		adminNotifs, _ := app.FindRecordsByFilter("notifications",
			"user = {:uid} && type = 'admin_message'",
			"", 0, 0, map[string]any{"uid": admins[0].Id})
		assert.GreaterOrEqual(tb, len(adminNotifs), 1,
			"admin must receive a notification about the supersede failure")
	}
	s.Test(t)
}

// Multiple proposals: only non-accepted ones are superseded

func TestAcceptProposalOnlySupersedesPending(t *testing.T) {
	t.Parallel()
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

func TestRejectProposalCreatesSchedulingResponse(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "rejecting proposal creates scheduling_response entry",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, respondentID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Rej A")
		p2 := makePairTB(tb, app, "Rej B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id

		proposer := p1.GetString("player1")
		prop := makeProposal(tb, app, match.Id, proposer)

		respondentID = p2.GetString("player1")
		respondent, _ := app.FindRecordById("users", respondentID)
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", match.Id, prop.Id)
		s.Body = strings.NewReader("action=reject&rejection_reason=No+puedo+ese+d%C3%ADa")
		hdrs := authHeaders(tb, respondent)
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
	}
	s.Test(t)
}

func TestThreadWithMessages(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /match/{id}/thread with messages renders them",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"thread-messages"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "ThrMsg A")
		p2 := makePairTB(tb, app, "ThrMsg B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

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
			"date": "2026-09-20", "time": "19:00", "venue_name": "Club Padel",
		})
		proposal.Set("proposal_status", "pending")
		require.NoError(tb, app.Save(proposal))

		s.URL = "/match/" + match.Id + "/thread"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestThreadMessagesWithData(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /match/{id}/thread-messages with messages",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Hola equipo"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "ThrData A")
		p2 := makePairTB(tb, app, "ThrData B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

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
			"date": "2026-09-20", "time": "19:00", "venue_name": "Club",
		})
		proposal.Set("proposal_status", "accepted")
		require.NoError(tb, app.Save(proposal))

		s.URL = "/match/" + match.Id + "/thread-messages"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestThreadMessages_ResultEventRendersAsSystemLine(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "result_event renders as system line, not chat bubble",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"thread-messages"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "SysLine A")
		p2 := makePairTB(tb, app, "SysLine B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")

		col, _ := app.FindCollectionByNameOrId("match_messages")
		msg := core.NewRecord(col)
		msg.Set("match", match.Id)
		msg.Set("author", p1.GetString("player1"))
		msg.Set("type", "result_event")
		msg.Set("content", "registró el resultado")
		require.NoError(tb, app.Save(msg))

		s.URL = "/match/" + match.Id + "/thread-messages"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)

		s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
			body := readBody(tb, res)
			assert.Contains(tb, body, `data-type="result"`, "result_event renders as event line")
			assert.Contains(tb, body, "registró el resultado", "event line content")
			assert.NotContains(tb, body, `data-type="message"`, "result_event must not render as chat bubble")
		}
	}
	s.Test(t)
}

func TestProposalChangeDecision(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST change-decision reverts accepted to rejected",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var msgID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "ChgDec A")
		p2 := makePairTB(tb, app, "ChgDec B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		col, _ := app.FindCollectionByNameOrId("match_messages")
		msg := core.NewRecord(col)
		msg.Set("match", match.Id)
		msg.Set("author", p1.GetString("player1"))
		msg.Set("type", "scheduling_proposal")
		msg.Set("proposal_data", map[string]any{
			"date": "2026-09-20", "time": "19:00", "venue_name": "Club",
		})
		msg.Set("proposal_status", "accepted")
		require.NoError(tb, app.Save(msg))
		msgID = msg.Id

		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/change-decision", match.Id, msg.Id)
		opponent, _ := app.FindRecordById("users", p2.GetString("player1"))
		s.Headers = authHeaders(tb, opponent)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("match_messages", msgID)
		require.NoError(tb, err)
		assert.Equal(tb, "rejected", m.GetString("proposal_status"))
	}
	s.Test(t)
}

// Cluster: Playoff thread (T6b)

func TestThread_PlayoffHidesProposal(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "playoff thread hides propose-date and shows admin-set date",
		Method:         http.MethodGet,
		ExpectedStatus: 200,
		ExpectedContent: []string{
			"Fecha:",
			"15/10/2026",
			"20:00",
			"Padel 360",
		},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "POThr1")
		p2 := makePairTB(tb, app, "POThr2")
		comp := makeCompetitionTB(tb, app, "playoff", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		match.Set("date", "2026-10-15")
		match.Set("time", "20:00")
		match.Set("club", "Padel 360")
		require.NoError(tb, app.Save(match))

		s.URL = "/match/" + match.Id + "/thread"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.NotContains(tb, body, "Proponer fecha", "playoff must not show propose-date form")
		assert.Contains(tb, body, "Fecha:", "playoff must show admin-set date")
	}
	s.Test(t)
}

func TestThread_PlayoffNoDateShowsPending(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "playoff thread with no date shows pending message",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"pendiente de asignación"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "PONoDt1")
		p2 := makePairTB(tb, app, "PONoDt2")
		comp := makeCompetitionTB(tb, app, "playoff", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		s.URL = "/match/" + match.Id + "/thread"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.NotContains(tb, body, "Proponer fecha", "playoff must not show propose-date form")
	}
	s.Test(t)
}

func TestAcceptProposalBlocksSecondAcceptance(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "accepting a second proposal is blocked when one is already accepted",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Ya hay una propuesta aceptada"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Dup A")
		p2 := makePairTB(tb, app, "Dup B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		prop1 := makeProposal(tb, app, match.Id, p1.GetString("player1"))
		prop1.Set("proposal_status", "accepted")
		require.NoError(tb, app.Save(prop1))

		prop2 := makeProposal(tb, app, match.Id, p1.GetString("player1"))

		respondent, _ := app.FindRecordById("users", p2.GetString("player1"))
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", match.Id, prop2.Id)
		s.Body = strings.NewReader("action=accept")
		hdrs := authHeaders(tb, respondent)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestAcceptProposal_SetsStatusScheduled(t *testing.T) {
	t.Parallel()
	var matchID string
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "accepting a proposal sets match status to scheduled",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "SchedA")
		p2 := makePairTB(tb, app, "SchedB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id

		prop := makeProposal(tb, app, match.Id, p1.GetString("player1"))
		respondent, _ := app.FindRecordById("users", p2.GetString("player1"))
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", match.Id, prop.Id)
		s.Body = strings.NewReader("action=accept")
		hdrs := authHeaders(tb, respondent)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		match, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, league.StatusScheduled, match.GetString("status"))
	}
	s.Test(t)
}

func TestReschedule_SupersedesOldAccepted(t *testing.T) {
	t.Parallel()
	var matchID, oldPropID string
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "rescheduling a scheduled match supersedes the old accepted proposal",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Resched A")
		p2 := makePairTB(tb, app, "Resched B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
		matchID = match.Id

		oldProp := makeProposal(tb, app, match.Id, p1.GetString("player1"))
		oldProp.Set("proposal_status", "accepted")
		require.NoError(tb, app.Save(oldProp))
		oldPropID = oldProp.Id

		newProp := makeProposal(tb, app, match.Id, p2.GetString("player1"))
		respondent, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", match.Id, newProp.Id)
		s.Body = strings.NewReader("action=accept")
		hdrs := authHeaders(tb, respondent)
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
	s.Test(t)
}

func TestScheduledMatch_AllowsNewProposal(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "scheduled match allows posting a new proposal",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "SchdPrA")
		p2 := makePairTB(tb, app, "SchdPrB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")

		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.URL = fmt.Sprintf("/match/%s/thread/proposal", match.Id)
		s.Body = strings.NewReader("date=2026-12-01&time=20:00&venue_id=")
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestPostMessage_AdminNonParticipant_Succeeds(t *testing.T) {
	t.Parallel()
	var matchID string
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "admin non-participant can post a thread message",
		Method:         http.MethodPost,
		Body:           strings.NewReader("content=Mensaje+del+admin&type=chat"),
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "AdmMsg A")
		p2 := makePairTB(tb, app, "AdmMsg B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id
		s.URL = "/match/" + match.Id + "/thread/message"
		hdrs := authHeaders(tb, admin)
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
	s.Test(t)
}

func TestThread_AdminNonParticipant_SeesComposeBox(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:     testAppFactory,
		Name:               "admin non-participant sees compose box and admin note",
		Method:             http.MethodGet,
		ExpectedStatus:     200,
		ExpectedContent:    []string{"Escribe un mensaje...", "Escribiendo como administrador"},
		NotExpectedContent: []string{"Proponer fecha", "solo lectura"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "AdmThr A")
		p2 := makePairTB(tb, app, "AdmThr B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + match.Id + "/thread"
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestPostMessage_NonParticipantNonAdmin_Rejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "non-participant non-admin cannot post",
		Method:          http.MethodPost,
		Body:            strings.NewReader("content=intruso&type=chat"),
		ExpectedStatus:  200,
		ExpectedContent: []string{"No eres participante"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		outsider := makeUserTB(tb, app, "Outsider Msg", "")
		p1 := makePairTB(tb, app, "OutMsg A")
		p2 := makePairTB(tb, app, "OutMsg B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + match.Id + "/thread/message"
		hdrs := authHeaders(tb, outsider)
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
		TestAppFactory: testAppFactory,
		Name:           "admin message notifies both pairs",
		Method:         http.MethodPost,
		Body:           strings.NewReader("content=Aviso+importante&type=chat"),
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "BothN A")
		p2 := makePairTB(tb, app, "BothN B")
		p1Player1 = p1.GetString("player1")
		p1Player2 = p1.GetString("player2")
		p2Player1 = p2.GetString("player1")
		p2Player2 = p2.GetString("player2")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id
		s.URL = "/match/" + match.Id + "/thread/message"
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		notifs, err := app.FindRecordsByFilter("notifications",
			"related_match = {:m}", "", 0, 0, map[string]any{"m": matchID})
		require.NoError(tb, err)

		notifiedUsers := make(map[string]bool)
		for _, n := range notifs {
			notifiedUsers[n.GetString("user")] = true
		}
		assert.True(tb, notifiedUsers[p1Player1], "pair1 player1 should be notified")
		assert.True(tb, notifiedUsers[p1Player2], "pair1 player2 should be notified")
		assert.True(tb, notifiedUsers[p2Player1], "pair2 player1 should be notified")
		assert.True(tb, notifiedUsers[p2Player2], "pair2 player2 should be notified")
	}
	s.Test(t)
}

func TestFinalMatchThreadAcceptsPost(t *testing.T) {
	t.Parallel()

	t.Run("participant can post in final match thread", func(t *testing.T) {
		t.Parallel()
		s := &tests.ApiScenario{
			TestAppFactory: testAppFactory,
			Name:           "participant posts in final match thread",
			Method:         http.MethodPost,
			ExpectedStatus: 204,
		}
		s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
			setupAllRoutes(tb, app, e)
			p1 := makePairTB(tb, app, "FinalT A")
			p2 := makePairTB(tb, app, "FinalT B")
			comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
			m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")
			m.Set("scores", "6-3 6-4")
			m.Set("winner", p1.Id)
			require.NoError(tb, app.Save(m))
			s.URL = "/match/" + m.Id + "/thread/message"
			player, err := app.FindRecordById("users", p1.GetString("player1"))
			require.NoError(tb, err)
			s.Body = strings.NewReader("content=Post+after+final")
			hdrs := authHeaders(tb, player)
			hdrs["Content-Type"] = "application/x-www-form-urlencoded"
			s.Headers = hdrs
		}
		s.Test(t)
	})

	t.Run("admin can post in final match thread", func(t *testing.T) {
		t.Parallel()
		s := &tests.ApiScenario{
			TestAppFactory: testAppFactory,
			Name:           "admin posts in final match thread",
			Method:         http.MethodPost,
			ExpectedStatus: 204,
		}
		s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
			setupAllRoutes(tb, app, e)
			admin := makeAdminUserTB(tb, app)
			p1 := makePairTB(tb, app, "FinalTA A")
			p2 := makePairTB(tb, app, "FinalTA B")
			comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
			m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")
			m.Set("scores", "6-3 6-4")
			m.Set("winner", p1.Id)
			require.NoError(tb, app.Save(m))
			s.URL = "/match/" + m.Id + "/thread/message"
			s.Body = strings.NewReader("content=Admin+post+after+final")
			hdrs := authHeaders(tb, admin)
			hdrs["Content-Type"] = "application/x-www-form-urlencoded"
			s.Headers = hdrs
		}
		s.Test(t)
	})
}

func TestThread_VenueNotPreSelected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "proposal form does not pre-select any venue",
		Method:          http.MethodGet,
		URL:             "/placeholder",
		ExpectedStatus:  200,
		ExpectedContent: []string{"thread-messages"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		makeVenueTB(tb, app, "SomeVenue")
		p1 := makePairTB(tb, app, "NoPreA")
		p2 := makePairTB(tb, app, "NoPreB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		current := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + current.Id + "/thread"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(_ testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(t, res)
		assert.NotContains(t, body, "selected>SomeVenue")
	}
	s.Test(t)
}

func TestThreadMessages_AdminActionRendersAsSystemLine(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "admin_action renders via resultEventLine, not chatMessage",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"thread-messages"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "AdminAct A")
		p2 := makePairTB(tb, app, "AdminAct B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")

		col, _ := app.FindCollectionByNameOrId("match_messages")
		msg := core.NewRecord(col)
		msg.Set("match", match.Id)
		msg.Set("author", p1.GetString("player1"))
		msg.Set("type", "admin_action")
		msg.Set("content", "Admin corrigió el resultado")
		require.NoError(tb, app.Save(msg))

		s.URL = "/match/" + match.Id + "/thread-messages"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.Contains(tb, body, `data-type="result"`, "admin_action renders as event line")
		assert.Contains(tb, body, "Admin corrigió el resultado")
		assert.NotContains(tb, body, "chat-bubble", "admin_action must not render as chat")
	}
	s.Test(t)
}

func TestThreadMessages_AllTypesRenderCorrectSubDefine(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "each message type dispatches to its sub-define",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"thread-messages"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "SubDef A")
		p2 := makePairTB(tb, app, "SubDef B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

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
			"date": "2026-10-05", "time": "20:00", "venue_name": "Padel 360",
		})
		proposal.Set("proposal_status", "pending")
		require.NoError(tb, app.Save(proposal))

		s.URL = "/match/" + match.Id + "/thread-messages"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.Contains(tb, body, `data-type="schedule"`, "proposal renders via proposalCard")
		assert.Contains(tb, body, "Padel 360", "proposal card shows venue name")
		assert.Contains(tb, body, "20:00", "proposal card shows time")
		assert.Regexp(tb, `\w+ \(SubDef B\)`, body, "proposal author shows PlayerName (PairName)")
		assert.NotContains(tb, body, "Pendiente", "proposal card has no status badge")
		assert.NotContains(tb, body, "Aceptada", "proposal card has no status badge")
		assert.NotContains(tb, body, "Rechazada", "proposal card has no status badge")
		assert.Contains(tb, body, `data-type="result"`, "result_event renders via eventLine")
		assert.Contains(tb, body, "registró resultado: 6-2 6-3", "system line shows event content")
		assert.Contains(tb, body, "chat-bubble", "chat renders via chatMessage sub-define")
		assert.Contains(tb, body, "Hola desde chat", "chat bubble shows message content")
	}
	s.Test(t)
}

func TestPlayerProposalBlockedOnFinalizedComp(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /match/{id}/propose blocked for player on finalized competition",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"finalizada o archivada"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "PB A")
		p2 := makePairTB(tb, app, "PB B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("finalized", true)
		require.NoError(tb, app.Save(comp))
		v := makeVenueTB(tb, app, "Blocked Venue")
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + m.Id + "/thread/proposal"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
		s.Body = strings.NewReader("date=2026-10-01&time=20:00&venue=" + v.Id)
	}
	s.Test(t)
}

func makeResultProposal(tb testing.TB, app core.App, matchID, authorID, scores string) *core.Record { //nolint:unparam
	tb.Helper()
	col, err := app.FindCollectionByNameOrId("match_messages")
	require.NoError(tb, err)
	msg := core.NewRecord(col)
	msg.Set("match", matchID)
	msg.Set("author", authorID)
	msg.Set("type", "result_submission")
	msg.Set("proposal_data", `{"scores":"`+scores+`"}`)
	msg.Set("proposal_status", "pending")
	msg.Set("content", scores)
	require.NoError(tb, app.Save(msg))
	return msg
}

func TestAcceptResultProposalFinalizesMatch(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "accepting result proposal finalizes the match",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, proposalID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "RA A")
		p2 := makePairTB(tb, app, "RA B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
		matchID = match.Id

		proposer := p1.GetString("player1")
		proposal := makeResultProposal(tb, app, match.Id, proposer, "6-3 6-4")
		proposalID = proposal.Id

		respondent, _ := app.FindRecordById("users", p2.GetString("player1"))
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", match.Id, proposal.Id)
		s.Body = strings.NewReader("action=accept")
		hdrs := authHeaders(tb, respondent)
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
	}
	s.Test(t)
}

func TestAcceptResultSupersedesSiblings(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "accepting result proposal supersedes sibling pending proposals",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, siblingID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "AS A")
		p2 := makePairTB(tb, app, "AS B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
		matchID = match.Id

		proposer := p1.GetString("player1")
		proposal := makeResultProposal(tb, app, match.Id, proposer, "6-3 6-4")

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
		hdrs := authHeaders(tb, respondent)
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
	s.Test(t)
}

func TestRejectResultProposalRequiresCounter(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "rejecting result proposal creates counter-proposal",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, proposalID, respondentID, proposerPairID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "RR A")
		p2 := makePairTB(tb, app, "RR B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
		matchID = match.Id
		proposerPairID = p1.Id

		proposer := p1.GetString("player1")
		proposal := makeResultProposal(tb, app, match.Id, proposer, "6-3 6-4")
		proposalID = proposal.Id

		respondent, _ := app.FindRecordById("users", p2.GetString("player1"))
		respondentID = respondent.Id
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", match.Id, proposal.Id)
		s.Body = strings.NewReader("action=reject&counter_scores=6-4+6-3")
		hdrs := authHeaders(tb, respondent)
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
		assert.Equal(tb, "6-4 6-3", ParseProposalData(counters[0].GetString("proposal_data")).Scores)

		// Timeline entry must exist for the rejection
		timeline, _ := app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'result_response' && parent = {:pid}",
			"", 0, 0,
			map[string]any{"mid": matchID, "pid": proposalID})
		require.Len(tb, timeline, 1, "rejection must create a result_response timeline entry")
		assert.Contains(tb, timeline[0].GetString("content"), "Resultado rechazado")

		proposerPair, _ := app.FindRecordById("pairs", proposerPairID)
		for _, field := range []string{"player1", "player2"} {
			uid := proposerPair.GetString(field)
			if uid == "" {
				continue
			}
			notifs, _ := app.FindRecordsByFilter("notifications",
				"user = {:uid} && title = 'Resultado disputado'",
				"", 0, 0, map[string]any{"uid": uid})
			assert.NotEmpty(tb, notifs, "proposer pair member %s must be notified of counter-proposal", uid)
		}
	}
	s.Test(t)
}

func TestRejectResultProposalEmptyCounterRejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "rejecting result proposal without counter_scores returns error",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Debes proponer un marcador alternativo"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "RE A")
		p2 := makePairTB(tb, app, "RE B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")

		proposer := p1.GetString("player1")
		proposal := makeResultProposal(tb, app, match.Id, proposer, "6-3 6-4")

		respondent, _ := app.FindRecordById("users", p2.GetString("player1"))
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", match.Id, proposal.Id)
		s.Body = strings.NewReader("action=reject&counter_scores=")
		hdrs := authHeaders(tb, respondent)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

// --- buildThreadData unit tests (match-thread-split spec) ---

func makeProposalWithStatus(tb testing.TB, app core.App, matchID, authorID, status string) *core.Record {
	tb.Helper()
	msg := makeProposal(tb, app, matchID, authorID)
	msg.Set("proposal_status", status)
	require.NoError(tb, app.Save(msg))
	return msg
}

func makeSchedulingResponse(tb testing.TB, app core.App, matchID, authorID, parentID, content string) *core.Record {
	tb.Helper()
	col, err := app.FindCollectionByNameOrId("match_messages")
	require.NoError(tb, err)
	msg := core.NewRecord(col)
	msg.Set("match", matchID)
	msg.Set("author", authorID)
	msg.Set("type", "scheduling_response")
	msg.Set("content", content)
	msg.Set("parent", parentID)
	require.NoError(tb, app.Save(msg))
	return msg
}

func TestBuildThreadData_TimelineReadOnly(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	p1 := makePairTB(t, app, "TL-RO A")
	p2 := makePairTB(t, app, "TL-RO B")
	comp := makeCompetitionTB(t, app, "league", []*core.Record{p1, p2})
	match := makeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "pending")

	proposer := p1.GetString("player1")
	responder := p2.GetString("player1")

	prop := makeProposal(t, app, match.Id, proposer)
	makeProposalWithStatus(t, app, match.Id, proposer, "rejected")
	accepted := makeProposalWithStatus(t, app, match.Id, proposer, "accepted")
	makeSchedulingResponse(t, app, match.Id, responder, accepted.Id,
		responder+" aceptó la propuesta de "+proposer)

	h := &ThreadHandler{app: app}
	td := h.buildThreadData(match, match.Id, 2, true)

	require.NotEmpty(t, td.Timeline, "timeline must have entries")

	var hasAcceptResponse bool
	for _, entry := range td.Timeline {
		if entry.Kind == "response" {
			hasAcceptResponse = true
			assert.Contains(t, entry.Content, "aceptó la propuesta")
		}
		assert.NotEmpty(t, entry.CreatedAt, "entry must have CreatedAt")
	}
	assert.True(t, hasAcceptResponse, "acceptance response must appear as top-level timeline entry (P7)")

	var proposalCount int
	for _, entry := range td.Timeline {
		if entry.Kind == "proposal" {
			proposalCount++
		}
	}
	assert.GreaterOrEqual(t, proposalCount, 1, "proposals must appear in timeline")

	_ = td.Timeline[0].Kind
	_ = td.Timeline[0].AuthorName
	_ = td.Timeline[0].Content
	_ = td.Timeline[0].CreatedAt
	_ = td.Timeline[0].IsMyTeam

	require.NotEmpty(t, td.SchedProposals)
	found := false
	for _, sp := range td.SchedProposals {
		if sp.RecordID == prop.Id {
			found = true
			assert.Equal(t, "pending", sp.Status)
		}
	}
	assert.True(t, found, "pending proposal must be in SchedProposals")
}

func TestBuildThreadData_HidesRejectedSched(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	p1 := makePairTB(t, app, "HR A")
	p2 := makePairTB(t, app, "HR B")
	comp := makeCompetitionTB(t, app, "league", []*core.Record{p1, p2})
	match := makeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "pending")

	proposer := p1.GetString("player1")

	pending := makeProposal(t, app, match.Id, proposer)
	rejected := makeProposalWithStatus(t, app, match.Id, proposer, "rejected")

	h := &ThreadHandler{app: app}
	td := h.buildThreadData(match, match.Id, 2, true)

	var ids []string
	for _, sp := range td.SchedProposals {
		ids = append(ids, sp.RecordID)
	}
	assert.Contains(t, ids, pending.Id, "pending proposal must appear")
	assert.NotContains(t, ids, rejected.Id, "rejected proposal must be hidden (P2)")
}

func TestBuildThreadData_NoFinalUntilQuorum(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	p1 := makePairTB(t, app, "NF A")
	p2 := makePairTB(t, app, "NF B")
	comp := makeCompetitionTB(t, app, "league", []*core.Record{p1, p2})
	match := makeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "scheduled")

	proposer := p1.GetString("player1")
	makeResultProposal(t, app, match.Id, proposer, "6-3 6-4")

	h := &ThreadHandler{app: app}

	td := h.buildThreadData(match, match.Id, 2, true)
	assert.False(t, td.ResultPanel.HasFinal, "must not show final before quorum (P4)")
	require.Len(t, td.ResultPanel.Live, 1, "one live result proposal")
	assert.Equal(t, "6-3 6-4", td.ResultPanel.Live[0].Score)

	winner, _ := league.DetermineWinner(match, "6-3 6-4")
	match.Set("status", league.StatusFinal)
	match.Set("scores", "6-3 6-4")
	match.Set("winner", winner)
	require.NoError(t, app.Save(match))

	td2 := h.buildThreadData(match, match.Id, 2, true)
	assert.True(t, td2.ResultPanel.HasFinal, "must show final after quorum (P4)")
	assert.Equal(t, "6-3 6-4", td2.ResultPanel.FinalScore)
	assert.NotEmpty(t, td2.ResultPanel.WinnerName)
	assert.Empty(t, td2.ResultPanel.Live, "no live proposals when final")
}

func TestBuildThreadData_BothLiveProposals(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	p1 := makePairTB(t, app, "BL A")
	p2 := makePairTB(t, app, "BL B")
	comp := makeCompetitionTB(t, app, "league", []*core.Record{p1, p2})
	match := makeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "scheduled")

	p1Player := p1.GetString("player1")
	p2Player := p2.GetString("player1")

	makeResultProposal(t, app, match.Id, p1Player, "6-3 6-4")
	makeResultProposal(t, app, match.Id, p2Player, "3-6 6-4 6-3")

	h := &ThreadHandler{app: app}

	td := h.buildThreadData(match, match.Id, 2, true)
	require.Len(t, td.ResultPanel.Live, 2, "deadlock: both proposals live (P5)")

	for _, lp := range td.ResultPanel.Live {
		assert.NotEmpty(t, lp.PairLabel, "each live proposal must have PairLabel")
	}

	awaitingCount := 0
	for _, lp := range td.ResultPanel.Live {
		if lp.AwaitingMe {
			awaitingCount++
		}
	}
	assert.Equal(t, 1, awaitingCount, "exactly one proposal awaiting viewer (P5)")
}
