package handlers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/league"
	"padelleague/notify"
)

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
	h.notifyProposal(match, 1, author, "2026-09-20", "19:00", "Club Test", match.Id)

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
	h.notifyProposal(match, 2, p2.GetString("player1"), "2026-09-20", "19:00", "Club Test", match.Id)

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
			s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
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
