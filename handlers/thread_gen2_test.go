package handlers

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/league"
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
