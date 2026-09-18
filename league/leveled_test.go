package league

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeLeveledCompetition creates a competition with leveled-league fields set.
func makeLeveledCompetition(t *testing.T, app core.App, pairs []*core.Record, target, open int) *core.Record {
	t.Helper()
	comp := makeCompetition(t, app, pairs)
	comp.Set("target_matches", target)
	comp.Set("open_assignments", open)
	require.NoError(t, app.Save(comp))
	return comp
}

// -- TestIsLeveled ----------------------------------------------------------

func TestIsLeveled(t *testing.T) {
	app := newTestApp(t)
	pairs := make([]*core.Record, 6)
	for i := range pairs {
		pairs[i] = makePair(t, app, "P")
	}

	cases := []struct {
		name   string
		target int
		pairs  []*core.Record
		typ    string // "" = league, "playoff" = playoff
		want   bool
	}{
		{"target 0 is round-robin", 0, pairs, "", false},
		{"3 of 6 is leveled", 3, pairs, "", true},
		{"4 of 6 is leveled", 4, pairs, "", true},
		{"5 of 6 is boundary (pairs-1), not leveled", 5, pairs, "", false},
		{"7 of 6 exceeds pairs-1, not leveled", 7, pairs, "", false},
		{"playoff is never leveled", 3, pairs, "playoff", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			comp := makeCompetition(t, app, tc.pairs)
			comp.Set("target_matches", tc.target)
			if tc.typ == "playoff" {
				comp.Set("type", "playoff")
			}
			require.NoError(t, app.Save(comp))
			assert.Equal(t, tc.want, IsLeveled(comp))
		})
	}
}

// -- TestEligible -----------------------------------------------------------

func TestEligible(t *testing.T) {
	// Build a pure leveledState for testing.
	st := &leveledState{
		target: 5,
		open:   3,
		met: map[string]map[string]struct{}{
			"A": {"B": {}},
		},
		played:   map[string]int{"A": 2, "B": 0, "C": 0, "D": 4},
		pending:  map[string]int{"A": 1, "B": 3, "C": 2, "D": 1},
		position: map[string]int{"A": 0, "B": 1, "C": 2, "D": 3},
	}
	// load(p) = played[p] + pending[p]
	// A: 2+1=3, B: 0+3=3, C: 0+2=2, D: 4+1=5 (== target)

	cases := []struct {
		name string
		p    string
		q    string
		want bool
	}{
		{"self is not eligible", "A", "A", false},
		{"already met is not eligible", "A", "B", false},
		{"at target load is not eligible as candidate", "A", "D", false},
		{"pending == open is eligible as candidate", "A", "C", true},  // C pending=2 == open-1 but ≤ open
		{"pending == open (B pending=3 == open)", "A", "B", false},    // also met
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, st.eligible(tc.p, tc.q))
		})
	}

	// Extra case: pending == open+1 → not eligible
	st2 := &leveledState{
		target:  5,
		open:    3,
		met:     map[string]map[string]struct{}{},
		played:  map[string]int{"A": 0, "B": 0},
		pending: map[string]int{"A": 0, "B": 4}, // B has open+1 = 4 pending
		position: map[string]int{"A": 0, "B": 1},
	}
	t.Run("pending == open+1 is not eligible", func(t *testing.T) {
		assert.False(t, st2.eligible("A", "B"))
	})

	// pending == open is eligible
	st3 := &leveledState{
		target:  5,
		open:    3,
		met:     map[string]map[string]struct{}{},
		played:  map[string]int{"A": 0, "B": 0},
		pending: map[string]int{"A": 0, "B": 3}, // B has exactly open=3 pending
		position: map[string]int{"A": 0, "B": 1},
	}
	t.Run("pending == open exactly is eligible", func(t *testing.T) {
		assert.True(t, st3.eligible("A", "B"))
	})
}

// -- TestChooser_RandomInsideZone -------------------------------------------

func TestChooser_RandomInsideZone(t *testing.T) {
	// 4 pairs at positions 0,1,2,3. Requester is position 0. target=4, open=2.
	// comfort = ceil(4/2) = 2. Zone for pos 0: d <= 2 → positions 1,2.
	// Position 3 is outside the zone.
	st := &leveledState{
		target:  4,
		open:    2,
		comfort: 2,
		pairs:   []string{"A", "B", "C", "D"},
		met:     map[string]map[string]struct{}{"A": {}, "B": {}, "C": {}, "D": {}},
		played:  map[string]int{"A": 0, "B": 0, "C": 0, "D": 0},
		pending: map[string]int{"A": 0, "B": 0, "C": 0, "D": 0},
		position: map[string]int{"A": 0, "B": 1, "C": 2, "D": 3},
	}

	// identity shuffle: inside-zone candidates keep order [B(1), C(2)]
	// first candidate inside zone should be chosen.
	identityShuffle := func(n int, swap func(i, j int)) {
		// no-op: keeps original order
	}
	svc := &Service{shuffle: identityShuffle}
	got := svc.chooseOpponent(st, "A")
	assert.Equal(t, "B", got, "identity shuffle should pick first in-zone candidate")

	// reversed shuffle: inside-zone candidates reversed → [C(2), B(1)]
	reversedShuffle := func(n int, swap func(i, j int)) {
		for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
			swap(i, j)
		}
	}
	svc2 := &Service{shuffle: reversedShuffle}
	got2 := svc2.chooseOpponent(st, "A")
	assert.Equal(t, "C", got2, "reversed shuffle should pick last in-zone candidate")
}

// -- TestChooser_NearestOutsideZone -----------------------------------------

func TestChooser_NearestOutsideZone(t *testing.T) {
	// Requester A at pos 0. target=2, open=2. comfort=ceil(2/2)=1.
	// Zone: d<=1 → only B at pos 1. Mark B as met, so B is ineligible.
	// Outside zone: C(pos 2, d=2), D(pos 3, d=3). Nearest is C.
	st := &leveledState{
		target:  2,
		open:    2,
		comfort: 1,
		pairs:   []string{"A", "B", "C", "D"},
		met: map[string]map[string]struct{}{
			"A": {"B": {}},
		},
		played:  map[string]int{"A": 0, "B": 0, "C": 0, "D": 0},
		pending: map[string]int{"A": 0, "B": 0, "C": 0, "D": 0},
		position: map[string]int{"A": 0, "B": 1, "C": 2, "D": 3},
	}

	identityShuffle := func(n int, swap func(i, j int)) {}
	svc := &Service{shuffle: identityShuffle}
	got := svc.chooseOpponent(st, "A")
	assert.Equal(t, "C", got, "when no eligible pair inside zone, pick nearest outside (C at d=2)")
}

// -- TestCompletable --------------------------------------------------------

func TestCompletable(t *testing.T) {
	cases := []struct {
		name  string
		need  map[string]int
		met   map[string]map[string]struct{}
		slack int
		want  bool
	}{
		{
			name: "4 pairs needing 1 with A-B and C-D met → true (can do A-C, B-D)",
			need: map[string]int{"A": 1, "B": 1, "C": 1, "D": 1},
			met: map[string]map[string]struct{}{
				"A": {"B": {}},
				"B": {"A": {}},
				"C": {"D": {}},
				"D": {"C": {}},
			},
			slack: 0,
			want:  true,
		},
		{
			name: "2 pairs needing 1 that met → false (no valid match)",
			need: map[string]int{"A": 1, "B": 1},
			met: map[string]map[string]struct{}{
				"A": {"B": {}},
				"B": {"A": {}},
			},
			slack: 0,
			want:  false,
		},
		{
			name: "slack 1 still false (both pairs met each other, need 1 each, 1 slot can't be filled)",
			need: map[string]int{"A": 1, "B": 1},
			met: map[string]map[string]struct{}{
				"A": {"B": {}},
				"B": {"A": {}},
			},
			slack: 1,
			want:  false,
		},
		{
			name: "slack 2 allows 2 unfillable slots → true",
			need: map[string]int{"A": 1, "B": 1},
			met: map[string]map[string]struct{}{
				"A": {"B": {}},
				"B": {"A": {}},
			},
			slack: 2,
			want:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := completable(tc.need, tc.met, tc.slack, 100)
			assert.Equal(t, tc.want, got)
		})
	}
}

// -- TestMinSlack -----------------------------------------------------------

func TestMinSlack(t *testing.T) {
	cases := []struct {
		name string
		need map[string]int
		met  map[string]map[string]struct{}
		want int
	}{
		{
			name: "completable with 0 slack → 0",
			need: map[string]int{"A": 1, "B": 1, "C": 1, "D": 1},
			met: map[string]map[string]struct{}{
				"A": {"B": {}}, "B": {"A": {}},
				"C": {"D": {}}, "D": {"C": {}},
			},
			want: 0,
		},
		{
			name: "odd total (3 pairs needing 1) → min slack 1",
			need: map[string]int{"A": 1, "B": 1, "C": 1},
			met:  map[string]map[string]struct{}{"A": {}, "B": {}, "C": {}},
			want: 1,
		},
		{
			name: "two pairs needing 1 that met → min slack 2",
			need: map[string]int{"A": 1, "B": 1},
			met: map[string]map[string]struct{}{
				"A": {"B": {}},
				"B": {"A": {}},
			},
			want: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := minSlack(tc.need, tc.met)
			assert.Equal(t, tc.want, got)
		})
	}
}

// -- TestPlan_SteadyState ---------------------------------------------------

func TestPlan_SteadyState(t *testing.T) {
	// 6 pairs, target=4, open=2. All pairs have load=0.
	// After plan: every pair should have pending >= open (or be at target).
	// Nobody should exceed open+1 pending.
	pairs := []string{"A", "B", "C", "D", "E", "F"}
	st := &leveledState{
		target:  4,
		open:    2,
		comfort: 2, // ceil(4/2)
		pairs:   pairs,
		met:     map[string]map[string]struct{}{"A": {}, "B": {}, "C": {}, "D": {}, "E": {}, "F": {}},
		played:  map[string]int{"A": 0, "B": 0, "C": 0, "D": 0, "E": 0, "F": 0},
		pending: map[string]int{"A": 0, "B": 0, "C": 0, "D": 0, "E": 0, "F": 0},
		position: map[string]int{"A": 0, "B": 1, "C": 2, "D": 3, "E": 4, "F": 5},
	}

	svc := &Service{shuffle: func(n int, swap func(i, j int)) {}} // identity

	pairings := plan(svc, st)

	// Tally pending per pair from the plan output.
	pendingCount := map[string]int{}
	for _, pair := range pairs {
		pendingCount[pair] = 0
	}
	for _, p := range pairings {
		pendingCount[p.A]++
		pendingCount[p.B]++
	}

	for _, pair := range pairs {
		cnt := pendingCount[pair]
		assert.GreaterOrEqual(t, cnt, st.open,
			"pair %s should have >= open (%d) pending, got %d", pair, st.open, cnt)
		assert.LessOrEqual(t, cnt, st.open+1,
			"pair %s should not exceed open+1 (%d) pending, got %d", pair, st.open+1, cnt)
		assert.LessOrEqual(t, cnt, st.target,
			"pair %s should not exceed target (%d) pending, got %d", pair, st.target, cnt)
	}
}
