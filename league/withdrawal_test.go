package league

import (
	"fmt"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	plannerPairCount = 6 // leveled needs target_matches below pairs-1
)

// plannerFixture is a competition of six pairs; withdrawn is always pair1 of the
// match under test unless the case asks otherwise.
type plannerFixture struct {
	app       core.App
	comp      *core.Record
	withdrawn *core.Record
	opponent  *core.Record
}

func newPlannerFixture(t *testing.T, kind string) plannerFixture {
	t.Helper()
	app := newTestApp(t)
	var pairs []*core.Record
	for i := range plannerPairCount {
		pairs = append(pairs, makePair(t, app, fmt.Sprintf("Planner %d", i)))
	}
	comp := makeCompetition(t, app, pairs)
	switch kind {
	case "leveled":
		comp.Set("target_matches", 3)
	case "playoff":
		comp.Set("type", "playoff")
	}
	require.NoError(t, app.Save(comp))
	return plannerFixture{app: app, comp: comp, withdrawn: pairs[0], opponent: pairs[1]}
}

func (f plannerFixture) match(t *testing.T, status, reviewType, scores string, admin bool, winner *core.Record) *core.Record {
	t.Helper()
	m := makeMatch(t, f.app, f.comp.Id, f.withdrawn.Id, f.opponent.Id, status)
	m.Set("review_type", reviewType)
	m.Set("scores", scores)
	m.Set("admin_result", admin)
	if winner != nil {
		m.Set("winner", winner.Id)
	}
	require.NoError(t, f.app.Save(m))
	return m
}

func TestWithdrawalPlan(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		kind       string
		status     string
		reviewType string
		scores     string
		admin      bool
		winnerOpp  bool // the opponent won (otherwise the withdrawn pair won)
		proposed   bool // the match has a standing result proposal
		noOpp      bool // playoff match whose next-round opponent is not seeded yet
		oppWdrawn  bool // the opponent already withdrew from the competition
		noWinner   bool // a void match: no winner is recorded
		want       WithdrawalAction
	}{
		{name: "standard pending", kind: "standard", status: StatusPending, want: WithdrawalOfficial},
		{name: "standard scheduled", kind: "standard", status: StatusScheduled, want: WithdrawalOfficial},
		{name: "standard pending with a proposal", kind: "standard", status: StatusPending, proposed: true, want: WithdrawalOfficial},
		{name: "standard disputed", kind: "standard", status: StatusDisputed, want: WithdrawalOfficial},
		{
			name: "standard final, admin set the official result", kind: "standard", status: StatusFinal,
			scores: WalkoverScore, admin: true, winnerOpp: true, want: WithdrawalLeave,
		},
		{
			name: "standard final, admin set the official result stored in the other order", kind: "standard",
			status: StatusFinal, scores: "0-6 0-6", admin: true, winnerOpp: true, want: WithdrawalLeave,
		},
		{
			name: "standard final, admin set a different score", kind: "standard", status: StatusFinal,
			scores: "6-1 6-2", admin: true, winnerOpp: true, want: WithdrawalOfficial,
		},
		{
			name: "standard final, player-produced 6-0 6-0", kind: "standard", status: StatusFinal,
			scores: WalkoverScore, winnerOpp: true, want: WithdrawalOfficial,
		},
		{
			name: "standard final, withdrawn pair won", kind: "standard", status: StatusFinal,
			scores: "6-1 6-2", admin: true, want: WithdrawalOfficial,
		},
		{
			name: "standard final, admin set the official score but the withdrawn pair won", kind: "standard",
			status: StatusFinal, scores: WalkoverScore, admin: true, want: WithdrawalOfficial,
		},
		{name: "leveled pending, not proposed", kind: "leveled", status: StatusPending, want: WithdrawalRelease},
		{name: "leveled scheduled, not proposed", kind: "leveled", status: StatusScheduled, want: WithdrawalRelease},
		{name: "leveled pending with a proposal", kind: "leveled", status: StatusPending, proposed: true, want: WithdrawalOfficial},
		{name: "leveled disputed", kind: "leveled", status: StatusDisputed, want: WithdrawalOfficial},
		{name: "leveled final", kind: "leveled", status: StatusFinal, scores: "6-1 6-2", winnerOpp: true, want: WithdrawalOfficial},
		{name: "playoff pending with an opponent", kind: "playoff", status: StatusPending, want: WithdrawalOfficial},
		{name: "playoff pending, opponent not seeded yet", kind: "playoff", status: StatusPending, noOpp: true, want: WithdrawalLeave},
		{name: "playoff final", kind: "playoff", status: StatusFinal, scores: "6-1 6-2", winnerOpp: true, want: WithdrawalLeave},
		{
			name: "playoff walkover final, opponent already withdrew", kind: "playoff", status: StatusFinal,
			reviewType: "walkover", scores: WalkoverScore, oppWdrawn: true, want: WithdrawalLeave,
		},
		{
			name: "playoff void final, opponent already withdrew", kind: "playoff", status: StatusFinal,
			noWinner: true, oppWdrawn: true, want: WithdrawalLeave,
		},
		{
			name: "playoff played final, opponent already withdrew", kind: "playoff", status: StatusFinal,
			scores: "6-1 6-2", winnerOpp: true, oppWdrawn: true, want: WithdrawalLeave,
		},
		{
			name: "standard walkover final, opponent already withdrew, withdrawn pair won", kind: "standard",
			status: StatusFinal, reviewType: "walkover", scores: WalkoverScore, oppWdrawn: true, want: WithdrawalLeave,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newPlannerFixture(t, tc.kind)
			var winner *core.Record
			switch {
			case tc.noWinner:
			case tc.winnerOpp:
				winner = f.opponent
			default:
				winner = f.withdrawn
			}
			m := f.match(t, tc.status, tc.reviewType, tc.scores, tc.admin, winner)
			if tc.noOpp {
				m.Set("pair2", "")
				require.NoError(t, f.app.SaveNoValidate(m))
			}
			if tc.oppWdrawn {
				f.comp.Set("withdrawn_pairs", []string{f.opponent.Id})
				require.NoError(t, f.app.Save(f.comp))
			}
			proposed := map[string]bool{}
			if tc.proposed {
				proposed[m.Id] = true
			}

			got := WithdrawalPlan(f.comp, m, f.withdrawn.Id, proposed)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestIsVoid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		status     string
		withWinner bool
		want       bool
	}{
		{name: "final with no winner", status: StatusFinal, want: true},
		{name: "final with a winner", status: StatusFinal, withWinner: true, want: false},
		{name: "pending with no winner", status: StatusPending, want: false},
		{name: "scheduled with no winner", status: StatusScheduled, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newPlannerFixture(t, "standard")
			var winner *core.Record
			if tc.withWinner {
				winner = f.withdrawn
			}
			m := f.match(t, tc.status, "", "", false, winner)
			assert.Equal(t, tc.want, IsVoid(m))
		})
	}
}

func TestOpponentOf(t *testing.T) {
	t.Parallel()
	f := newPlannerFixture(t, "standard")
	m := f.match(t, StatusPending, "", "", false, nil)

	assert.Equal(t, f.opponent.Id, OpponentOf(m, f.withdrawn.Id), "withdrawn pair is pair1")
	assert.Equal(t, f.withdrawn.Id, OpponentOf(m, f.opponent.Id), "withdrawn pair is pair2")
}

func TestOfficialScore(t *testing.T) {
	t.Parallel()
	f := newPlannerFixture(t, "standard")
	m := f.match(t, StatusPending, "", "", false, nil)

	tests := []struct {
		name     string
		winnerID string
		want     string
	}{
		{name: "pair1 wins, score kept", winnerID: f.withdrawn.Id, want: WalkoverScore},
		{name: "pair2 wins, score flipped to the winner's side", winnerID: f.opponent.Id, want: "0-6 0-6"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, OfficialScore(m, tc.winnerID, WalkoverScore))
		})
	}
}

func TestIsOfficialScore(t *testing.T) {
	t.Parallel()
	f := newPlannerFixture(t, "standard")

	tests := []struct {
		name   string
		scores string
		want   bool
	}{
		{name: "stored as woScore", scores: "6-0 6-0", want: true},
		{name: "stored flipped", scores: "0-6 0-6", want: true},
		{name: "different score", scores: "6-1 6-2", want: false},
		{name: "no score", scores: "", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := f.match(t, StatusFinal, "", tc.scores, true, f.opponent)
			assert.Equal(t, tc.want, isOfficialScore(m, WalkoverScore))
		})
	}
}

func TestRealScore(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status string
		review string
		scores string
		// proposal is the score a player proposed; "" means none.
		proposal string
		// rival is a conflicting proposal from the opponent's player; "" means none.
		rival string
		want  string
	}{
		{name: "final, played", status: StatusFinal, scores: "6-3 6-4", want: "6-3 6-4"},
		{name: "final walkover has no real score", status: StatusFinal, review: "walkover", scores: WalkoverScore, want: ""},
		{name: "pending with a standing proposal", status: StatusPending, proposal: "6-3 6-4", want: "6-3 6-4"},
		{name: "scheduled with an undecided open set", status: StatusScheduled, proposal: "6-3 2-1", want: ""},
		{name: "pending deadlock", status: StatusPending, proposal: "6-3 6-4", rival: "6-4 6-3", want: ""},
		{name: "disputed", status: StatusDisputed, proposal: "6-3 6-4", want: ""},
		{name: "unplayed, no proposal", status: StatusPending, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newPlannerFixture(t, "standard")
			m := f.match(t, tc.status, tc.review, tc.scores, false, f.withdrawn)
			if tc.proposal != "" {
				makeProposal(t, f.app, m.Id, f.withdrawn.GetString("player1"), tc.proposal)
			}
			if tc.rival != "" {
				makeProposal(t, f.app, m.Id, f.opponent.GetString("player1"), tc.rival)
			}

			got, err := RealScore(f.app, m)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
