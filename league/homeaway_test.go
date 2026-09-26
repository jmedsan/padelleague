package league

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHomeQuota(t *testing.T) {
	t.Parallel()
	lo, hi := homeQuota(10)
	assert.Equal(t, [2]int{5, 5}, [2]int{lo, hi})
	lo, hi = homeQuota(9)
	assert.Equal(t, [2]int{4, 5}, [2]int{lo, hi})
}

// TestHFactor covers the orientation lookahead on small graphs. Vertices are
// 0..n-1; avail is the bitmask of allowed partners.
func TestHFactor(t *testing.T) {
	t.Parallel()
	all4 := func(v int) uint32 { return 0b1111 &^ (1 << uint(v)) }
	cases := []struct {
		name             string
		need             []int
		avail            []uint32
		homeMin, homeMax []int
		want             bool
	}{
		{"nothing left", []int{0, 0}, []uint32{0, 0}, []int{0, 0}, []int{0, 0}, true},
		{"two pairs, one match, either side", []int{1, 1}, []uint32{0b10, 0b01}, []int{0, 0}, []int{1, 1}, true},
		{"two pairs, both must be home", []int{1, 1}, []uint32{0b10, 0b01}, []int{1, 1}, []int{1, 1}, false},
		{"two pairs, both must be away", []int{1, 1}, []uint32{0b10, 0b01}, []int{0, 0}, []int{0, 0}, false},
		{"two pairs, one must be away and the other home", []int{1, 1}, []uint32{0b10, 0b01}, []int{0, 1}, []int{0, 1}, true},
		{"forced pairs with clashing sides", []int{1, 1, 1, 1}, []uint32{0b0010, 0b0001, 0b1000, 0b0100}, []int{1, 1, 0, 0}, []int{1, 1, 0, 0}, false},
		{"forced pairs with matching sides", []int{1, 1, 1, 1}, []uint32{0b0010, 0b0001, 0b1000, 0b0100}, []int{1, 0, 1, 0}, []int{1, 0, 1, 0}, true},
		{"complete K4, 3 each, split 1..2", []int{3, 3, 3, 3}, []uint32{all4(0), all4(1), all4(2), all4(3)}, []int{1, 1, 1, 1}, []int{2, 2, 2, 2}, true},
		{"complete K4, 3 each, everyone 2 home: needs 8 homes for 6 matches", []int{3, 3, 3, 3}, []uint32{all4(0), all4(1), all4(2), all4(3)}, []int{2, 2, 2, 2}, []int{2, 2, 2, 2}, false},
		{"over quota already", []int{1, 1}, []uint32{0b10, 0b01}, []int{0, 0}, []int{-1, 1}, false},
		{"done vertex still owing a home", []int{0, 1, 1}, []uint32{0, 0b100, 0b010}, []int{1, 0, 0}, []int{1, 1, 1}, false},
		{"done vertex over quota", []int{0, 1, 1}, []uint32{0, 0b100, 0b010}, []int{0, 0, 0}, []int{-1, 1, 1}, false},
		{"odd total need", []int{1, 1, 1}, []uint32{0b110, 0b101, 0b011}, []int{0, 0, 0}, []int{1, 1, 1}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, hfactor(tc.need, tc.avail, tc.homeMin, tc.homeMax))
		})
	}
}

// TestHFactor_DoesNotMutateInputs guards the copy-on-entry contract.
func TestHFactor_DoesNotMutateInputs(t *testing.T) {
	t.Parallel()
	need := []int{3, 3, 3, 3}
	avail := []uint32{0b1110, 0b1101, 0b1011, 0b0111}
	homeMin := []int{1, 1, 1, 1}
	homeMax := []int{2, 2, 2, 2}
	require.True(t, hfactor(need, avail, homeMin, homeMax))
	assert.Equal(t, []int{3, 3, 3, 3}, need)
	assert.Equal(t, []uint32{0b1110, 0b1101, 0b1011, 0b0111}, avail)
	assert.Equal(t, []int{1, 1, 1, 1}, homeMin)
	assert.Equal(t, []int{2, 2, 2, 2}, homeMax)
}

// homeCounts tallies final matches as pair1 per pair.
func homeCounts(t *testing.T, app core.App, compID string) map[string]int {
	t.Helper()
	finals, err := app.FindRecordsByFilter("matches",
		"competition = {:c} && status = 'final'", "", 0, 0, map[string]any{"c": compID})
	require.NoError(t, err)
	home := map[string]int{}
	for _, m := range finals {
		home[m.GetString("pair1")]++
	}
	return home
}

// TestSeason_ExactHomeAwaySplit: 8 pairs, target 4, open 2 — after a full
// season every pair has played exactly 2 home and 2 away. The balance
// heuristic alone left seasons with a 3/1 pair (see the simulation's
// "Every pair 5/5" column before the lookahead); the orientation lookahead
// guarantees the split.
func TestSeason_ExactHomeAwaySplit(t *testing.T) {
	app := newTestApp(t)
	svc := newDeterministicSvc(app)
	for seed := range 4 {
		pairs := make([]*core.Record, 8)
		for i := range pairs {
			pairs[i] = makePair(t, app, "HA")
		}
		comp := makeLeveledCompetition(t, app, pairs, 4, 2)
		now := time.Now().Add(time.Duration(seed) * time.Hour)
		_, err := svc.GenerateInitialAssignments(app, comp, now)
		require.NoError(t, err)
		assert.Equal(t, 16, playSeason(t, app, svc, comp.Id, now))
		home := homeCounts(t, app, comp.Id)
		for _, p := range pairs {
			assert.Equal(t, 2, home[p.Id], "season %d: pair %s home count", seed, p.Id)
		}
	}
}

// TestSeason_OddTargetHomeAwaySplit: target 3 with 6 pairs → every pair ends
// on 1 or 2 home matches.
func TestSeason_OddTargetHomeAwaySplit(t *testing.T) {
	app := newTestApp(t)
	svc := newDeterministicSvc(app)
	pairs := make([]*core.Record, 6)
	for i := range pairs {
		pairs[i] = makePair(t, app, "HAodd")
	}
	comp := makeLeveledCompetition(t, app, pairs, 3, 2)
	now := time.Now()
	_, err := svc.GenerateInitialAssignments(app, comp, now)
	require.NoError(t, err)
	assert.Equal(t, 9, playSeason(t, app, svc, comp.Id, now))
	home := homeCounts(t, app, comp.Id)
	for _, p := range pairs {
		assert.Contains(t, []int{1, 2}, home[p.Id], "pair %s home count", p.Id)
	}
}

// TestSeason_HomeAwayAfterWithdrawal: two survivors hold a walkover against
// the withdrawn pair. Walkovers were never played, so they count toward the
// target but not toward the home/away quota (which then covers 3 real
// matches: 1 or 2 at home); the roster still has an exact completion
// (remaining need 3+3+5×4 = 26). Every active pair ends on its exact split. Repeated over several trials because
// record ids randomize the finalization order.
func TestSeason_HomeAwayAfterWithdrawal(t *testing.T) {
	app := newTestApp(t)
	svc := newDeterministicSvc(app)
	for trial := range 5 {
		pairs := make([]*core.Record, 8)
		for i := range pairs {
			pairs[i] = makePair(t, app, "HAwd")
		}
		comp := makeLeveledCompetition(t, app, pairs, 4, 2)
		now := time.Now().Add(time.Duration(trial) * time.Hour)
		for i, at := range []time.Duration{-2 * time.Hour, -time.Hour} {
			wo := makeLeveledMatch(t, app, comp.Id, pairs[i].Id, pairs[7].Id, "6-0 6-0", pairs[i].Id, "final", now.Add(at))
			wo.Set("review_type", "walkover")
			require.NoError(t, app.Save(wo))
		}
		comp.Set("withdrawn_pairs", []string{pairs[7].Id})
		require.NoError(t, app.Save(comp))
		st, err := buildLeveledState(app, comp, nil, now)
		require.NoError(t, err)
		require.False(t, st.rematch, "trial %d: an exact completion must exist for this roster", trial)
		require.True(t, st.orient, "trial %d: the split must be attainable at the start", trial)

		_, err = svc.TopUpAssignments(comp.Id, now)
		require.NoError(t, err)
		playSeason(t, app, svc, comp.Id, now)
		home := homeCounts(t, app, comp.Id)
		for i, p := range pairs[:7] {
			if i < 2 {
				// 3 real matches (the walkover home is excluded): 1 or 2 at home.
				assert.Contains(t, []int{2, 3}, home[p.Id], "trial %d: survivor %s home count incl. walkover", trial, p.Id)
				continue
			}
			assert.Equal(t, 2, home[p.Id], "trial %d: pair %s home count", trial, p.Id)
		}
	}
}
