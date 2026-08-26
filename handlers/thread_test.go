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

// canRespondToProposal decides whether the accept/reject buttons and the
// change-decision button appear on a scheduling proposal. Every clause is
// load-bearing: getting any of them wrong either hides a legitimate action or
// offers one that will be refused.
func TestCanRespondToProposal(t *testing.T) {
	t.Parallel()

	const prop = "scheduling_proposal"

	cases := []struct {
		name                 string
		msgType, matchStatus string
		authorTeam, myTeam   int
		proposalStatus       string
		wantRespond, wantChg bool
	}{
		// The ordinary case: opponent sees accept/reject on a pending proposal.
		{"opponent, pending", prop, league.StatusPending, 1, 2, "pending", true, false},
		{"opponent, accepted", prop, league.StatusPending, 1, 2, "accepted", false, true},
		{"opponent, rejected", prop, league.StatusPending, 1, 2, "rejected", false, true},

		// You cannot respond to your own proposal.
		{"own proposal, pending", prop, league.StatusPending, 2, 2, "pending", false, false},
		{"own proposal, accepted", prop, league.StatusPending, 2, 2, "accepted", false, false},

		// Someone in neither pair has myTeam 0 and can do nothing.
		{"outsider", prop, league.StatusPending, 1, 0, "pending", false, false},

		// Scheduling is only actionable while the match is still pending.
		{"confirmed match", prop, league.StatusConfirmed, 1, 2, "pending", false, false},
		{"disputed match", prop, league.StatusDisputed, 1, 2, "pending", false, false},
		{"final match", prop, league.StatusFinal, 1, 2, "pending", false, false},

		// Only proposals are actionable, not chat or score messages.
		{"chat message", "chat", league.StatusPending, 1, 2, "pending", false, false},
		{"score discussion", "score_discussion", league.StatusPending, 1, 2, "pending", false, false},

		// An unknown proposal status offers neither action.
		{"superseded proposal", prop, league.StatusPending, 1, 2, "superseded", false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			respond, change := canRespondToProposal(tc.msgType, tc.matchStatus, tc.authorTeam, tc.myTeam, tc.proposalStatus)
			assert.Equal(t, tc.wantRespond, respond, "canRespond")
			assert.Equal(t, tc.wantChg, change, "canChange")
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

// --- Normal case: accepting a proposal supersedes the others, no admin notification ---

func TestAcceptProposalSupersedesOthers(t *testing.T) {
	t.Parallel()
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

// --- Cluster: Playoff thread (T6b) ---

func TestThread_PlayoffHidesProposal(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "playoff thread hides propose-date and shows admin-set date",
		Method:         http.MethodGet,
		ExpectedStatus: 200,
		ExpectedContent: []string{
			"Fecha:",
			"2026-10-15",
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

// --- Cluster: Availability buttons (T9a) ---

func TestThread_AvailabilityButtons(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "thread shows availability buttons for participants",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Estoy libre", "No puedo"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "AvailA")
		p2 := makePairTB(tb, app, "AvailB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		s.URL = "/match/" + match.Id + "/thread"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestPostAvailability_CreatesMessage(t *testing.T) {
	t.Parallel()
	var matchID string
	var userID string
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST availability creates system message",
		Method:         http.MethodPost,
		Body:           strings.NewReader("available=1"),
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "AvPost1")
		p2 := makePairTB(tb, app, "AvPost2")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id

		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		userID = user.Id
		s.URL = "/match/" + match.Id + "/thread/availability"
		s.Headers = authHeaders(tb, user)
		s.Headers["Content-Type"] = "application/x-www-form-urlencoded"
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		msgs, err := app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'availability'",
			"", 0, 0, map[string]any{"mid": matchID})
		require.NoError(tb, err)
		require.Len(tb, msgs, 1)
		assert.Equal(tb, "Estoy libre", msgs[0].GetString("content"))
		assert.Equal(tb, userID, msgs[0].GetString("author"))
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
