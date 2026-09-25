package league

import (
	"math/rand/v2"
	"slices"
	"sort"
	"testing"
	"time"

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

// newDeterministicSvc returns a Service with a no-op shuffle so leveled
// assignment tests produce stable results regardless of CPU load.
func newDeterministicSvc(app core.App) *Service {
	svc := New(app, nil)
	svc.SetShuffle(func(_ int, _ func(int, int)) {})
	return svc
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

// -- TestOpenAssignments -----------------------------------------------------

func TestOpenAssignments(t *testing.T) {
	app := newTestApp(t)
	pairs := make([]*core.Record, 6)
	for i := range pairs {
		pairs[i] = makePair(t, app, "P")
	}

	cases := []struct {
		name string
		open int
		want int
	}{
		{"unset defaults to 3", 0, 3},
		{"positive value passes through", 5, 5},
		{"one passes through", 1, 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			comp := makeCompetition(t, app, pairs)
			comp.Set("open_assignments", tc.open)
			require.NoError(t, app.Save(comp))
			assert.Equal(t, tc.want, OpenAssignments(comp))
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
		{"pending == open is eligible as candidate", "A", "C", true}, // C pending=2 == open-1 but ≤ open
		{"pending == open (B pending=3 == open)", "A", "B", false},   // also met
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, st.eligible(tc.p, tc.q))
		})
	}

	// Extra case: pending == open+1 → not eligible
	st2 := &leveledState{
		target:   5,
		open:     3,
		met:      map[string]map[string]struct{}{},
		played:   map[string]int{"A": 0, "B": 0},
		pending:  map[string]int{"A": 0, "B": 4}, // B has open+1 = 4 pending
		position: map[string]int{"A": 0, "B": 1},
	}
	t.Run("pending == open+1 is not eligible", func(t *testing.T) {
		assert.False(t, st2.eligible("A", "B"))
	})

	// pending == open is eligible — this is the deliberate "one above open"
	// rule (recipe.md §3.2): only a requester needs a free slot; an opponent
	// may be pushed to open+1. A strict cap here was simulated and rejected
	// (recipe.md §5 "all values" table) — it made pairing quality as uneven
	// as random assignment.
	st3 := &leveledState{
		target:   5,
		open:     3,
		met:      map[string]map[string]struct{}{},
		played:   map[string]int{"A": 0, "B": 0},
		pending:  map[string]int{"A": 0, "B": 3}, // B has exactly open=3 pending
		position: map[string]int{"A": 0, "B": 1},
	}
	t.Run("pending == open exactly is eligible", func(t *testing.T) {
		assert.True(t, st3.eligible("A", "B"))
	})
}

// -- TestChooser_RandomInsideZone -------------------------------------------

func TestChooser_RandomInsideZone(t *testing.T) {
	// 4 pairs at Elo 0,1,2,3 (rank position and Elo coincide here). Requester
	// is A (Elo 0). comfort = 2 Elo points. Zone for A: d <= 2 → B(1), C(2).
	// D(3) is outside the zone.
	st := &leveledState{
		target:   4,
		open:     2,
		comfort:  2,
		pairs:    []string{"A", "B", "C", "D"},
		elo:      map[string]float64{"A": 0, "B": 1, "C": 2, "D": 3},
		met:      map[string]map[string]struct{}{"A": {}, "B": {}, "C": {}, "D": {}},
		played:   map[string]int{"A": 0, "B": 0, "C": 0, "D": 0},
		pending:  map[string]int{"A": 0, "B": 0, "C": 0, "D": 0},
		position: map[string]int{"A": 0, "B": 1, "C": 2, "D": 3},
		occupied: map[string]map[int]bool{"A": {}, "B": {}, "C": {}, "D": {}},
	}

	// identity shuffle: inside-zone candidates keep order [B(1), C(2)]
	// first candidate inside zone should be chosen.
	identityShuffle := func(_ int, _ func(int, int)) {}
	svc := &Service{shuffle: identityShuffle}
	got := svc.chooseOpponentForRound(st, "A", 1)
	assert.Equal(t, "B", got, "identity shuffle should pick first in-zone candidate")

	// reversed shuffle: inside-zone candidates reversed → [C(2), B(1)]
	reversedShuffle := func(n int, swap func(i, j int)) {
		for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
			swap(i, j)
		}
	}
	svc2 := &Service{shuffle: reversedShuffle}
	got2 := svc2.chooseOpponentForRound(st, "A", 1)
	assert.Equal(t, "C", got2, "reversed shuffle should pick last in-zone candidate")
}

// -- TestChooser_NearestOutsideZone -----------------------------------------

func TestChooser_NearestOutsideZone(t *testing.T) {
	// Requester A at Elo 0. target=2, open=2. comfort=1 Elo point.
	// Zone: d<=1 → only B at Elo 1. Mark B as met, so B is ineligible.
	// Outside zone: C(Elo 2, d=2), D(Elo 3, d=3). Nearest is C.
	st := &leveledState{
		target:  2,
		open:    2,
		comfort: 1,
		pairs:   []string{"A", "B", "C", "D"},
		elo:     map[string]float64{"A": 0, "B": 1, "C": 2, "D": 3},
		met: map[string]map[string]struct{}{
			"A": {"B": {}},
		},
		played:   map[string]int{"A": 0, "B": 0, "C": 0, "D": 0},
		pending:  map[string]int{"A": 0, "B": 0, "C": 0, "D": 0},
		position: map[string]int{"A": 0, "B": 1, "C": 2, "D": 3},
		occupied: map[string]map[int]bool{"A": {}, "B": {}, "C": {}, "D": {}},
	}

	identityShuffle := func(_ int, _ func(int, int)) {}
	svc := &Service{shuffle: identityShuffle}
	got := svc.chooseOpponentForRound(st, "A", 1)
	assert.Equal(t, "C", got, "when no eligible pair inside zone, pick nearest outside (C at d=2)")
}

// TestCollectCandidates_OutsideSortOrder pins the tie-break chain
// (dist -> load -> position) with candidates where the criteria actively
// disagree, so a sort key swap or comparator mutation changes the order.
func TestCollectCandidates_OutsideSortOrder(t *testing.T) {
	// Requester A at pos 0, comfort 0 (everyone lands outside).
	// B: pos=2 (dist=2), load=4 (eligible, heaviest) — same dist as C, heavier load.
	// C: pos=-2 (dist=2), load=1 — same dist as B, lighter load → must sort before B.
	// D: pos=3 (dist=3), load=0 — farthest, must sort last despite lightest load.
	// E,F: pos=-10,10 (dist=10, tied), load=2 (tied with each other) — only
	// position (E<F) distinguishes them, so E must sort before F. F precedes
	// E in st.pairs (input order to sort.Slice) so a comparator that treats
	// the load tie as "no preference" would preserve [F, E] — the wrong
	// order — rather than coincidentally landing on the right one.
	st := &leveledState{
		target:  5,
		open:    5,
		comfort: 0,
		pairs:   []string{"A", "B", "C", "D", "F", "E"},
		elo: map[string]float64{
			"A": 0, "B": 2, "C": -2, "D": 3, "E": -10, "F": 10,
		},
		met:     map[string]map[string]struct{}{},
		played:  map[string]int{"A": 0, "B": 4, "C": 1, "D": 0, "E": 2, "F": 2},
		pending: map[string]int{"A": 0, "B": 0, "C": 0, "D": 0, "E": 0, "F": 0},
		position: map[string]int{
			"A": 0, "B": 2, "C": -2, "D": 3, "E": -10, "F": 10,
		},
		occupied: map[string]map[int]bool{"A": {}, "B": {}, "C": {}, "D": {}, "E": {}, "F": {}},
	}
	identityShuffle := func(_ int, _ func(int, int)) {}
	svc := &Service{shuffle: identityShuffle}

	_, outside := svc.collectCandidatesForRound(st, "A", 1)
	require.Len(t, outside, 5)
	got := []string{outside[0].id, outside[1].id, outside[2].id, outside[3].id, outside[4].id}
	assert.Equal(t, []string{"C", "B", "D", "E", "F"}, got,
		"dist ties (B,C) break on load (C<B); D sorts last on dist; "+
			"E,F tie on dist+load, break on position (E<F)")
}

// Note: `a.dist < b.dist` and `a.load < b.load` in collectCandidatesForRound's sort.Slice
// comparator (leveled.go) are equivalent mutants under `<=` — each is only
// reached when the guarding `!=` check on the same field is true (operands
// differ), and <= agrees with < whenever the operands differ. The
// distinguishing case (operands equal) falls through to the next tie-break
// level instead, never reaching these lines.

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
		target:   4,
		open:     2,
		comfort:  2, // ceil(4/2)
		pairs:    pairs,
		met:      map[string]map[string]struct{}{"A": {}, "B": {}, "C": {}, "D": {}, "E": {}, "F": {}},
		played:   map[string]int{"A": 0, "B": 0, "C": 0, "D": 0, "E": 0, "F": 0},
		pending:  map[string]int{"A": 0, "B": 0, "C": 0, "D": 0, "E": 0, "F": 0},
		position: map[string]int{"A": 0, "B": 1, "C": 2, "D": 3, "E": 4, "F": 5},
		occupied: map[string]map[int]bool{"A": {}, "B": {}, "C": {}, "D": {}, "E": {}, "F": {}},
	}

	svc := &Service{shuffle: func(_ int, _ func(int, int)) {}} // identity

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

// TestPlan_EvenRounds_16 pins the round-fill regression: 16 pairs, open=3
// must produce exactly 3 Jornadas of 8 matches each (round-robin per round),
// not the old greedy scatter (7/8/7/1).
func TestPlan_EvenRounds_16(t *testing.T) {
	n := 16
	pairs := make([]string, n)
	played, pending, position := map[string]int{}, map[string]int{}, map[string]int{}
	met := map[string]map[string]struct{}{}
	occupied := map[string]map[int]bool{}
	for i := range n {
		id := string(rune('A' + i))
		pairs[i] = id
		played[id], pending[id], position[id] = 0, 0, i
		met[id] = map[string]struct{}{}
		occupied[id] = map[int]bool{}
	}
	st := &leveledState{
		target: 10, open: 3, comfort: 5,
		pairs: pairs, played: played, pending: pending,
		met: met, position: position, occupied: occupied,
	}
	svc := &Service{shuffle: func(_ int, _ func(int, int)) {}}

	pairings := plan(svc, st)

	bySlot := map[int]int{}
	for _, p := range pairings {
		bySlot[p.Slot]++
	}
	assert.Equal(t, map[int]int{1: 8, 2: 8, 3: 8}, bySlot,
		"16 pairs, open=3 must produce exactly 3 Jornadas of 8 matches each")
}

// TestPlan_EvenRounds_15 pins odd-pair handling: one pair sits out each
// round (7 matches instead of 7.5), never scattering into an uneven batch.
func TestPlan_EvenRounds_15(t *testing.T) {
	n := 15
	pairs := make([]string, n)
	played, pending, position := map[string]int{}, map[string]int{}, map[string]int{}
	met := map[string]map[string]struct{}{}
	occupied := map[string]map[int]bool{}
	for i := range n {
		id := string(rune('A' + i))
		pairs[i] = id
		played[id], pending[id], position[id] = 0, 0, i
		met[id] = map[string]struct{}{}
		occupied[id] = map[int]bool{}
	}
	st := &leveledState{
		target: 10, open: 3, comfort: 5,
		pairs: pairs, played: played, pending: pending,
		met: met, position: position, occupied: occupied,
	}
	svc := &Service{shuffle: func(_ int, _ func(int, int)) {}}

	pairings := plan(svc, st)

	bySlot := map[int]int{}
	for _, p := range pairings {
		bySlot[p.Slot]++
	}
	for k := 1; k <= 3; k++ {
		assert.Equal(t, 7, bySlot[k], "Jornada %d should have 7 matches (one pair sits out)", k)
	}
}

// TestPlan_TopUpInCurrentRound verifies a top-up never lands before the
// competition's current calendar window, even though the pairs' own loads
// alone would place them in an earlier round.
func TestPlan_TopUpInCurrentRound(t *testing.T) {
	now := time.Now()
	pairs := []string{"A", "B", "C", "D"}
	st := &leveledState{
		target: 10, open: 2, comfort: 2,
		pairs:    pairs,
		met:      map[string]map[string]struct{}{"A": {}, "B": {}, "C": {}, "D": {}},
		played:   map[string]int{"A": 0, "B": 0, "C": 0, "D": 0},
		pending:  map[string]int{"A": 0, "B": 0, "C": 0, "D": 0},
		position: map[string]int{"A": 0, "B": 1, "C": 2, "D": 3},
		occupied: map[string]map[int]bool{"A": {}, "B": {}, "C": {}, "D": {}},
		start:    now.Add(-50 * 24 * time.Hour),
		end:      now.Add(50 * 24 * time.Hour), // u=10d, now=start+50d → cur=6
		now:      now,
	}
	svc := &Service{shuffle: func(_ int, _ func(int, int)) {}}

	pairings := plan(svc, st)

	require.NotEmpty(t, pairings)
	for _, p := range pairings {
		assert.GreaterOrEqual(t, p.Slot, 6, "top-up slot must never be before the current calendar window")
	}
}

// TestPlan_LastJornadaMultiple verifies that at k=target the occupied check
// is skipped: a pair that still wants() can hold multiple matches in the
// final Jornada, since there is no later round to push the overflow into.
func TestPlan_LastJornadaMultiple(t *testing.T) {
	// 3 pairs, target=2 (last Jornada). A has already played B in round 1,
	// leaving only A-C and B-C available. With one round left, A and B both
	// still want (pending=1 < open=2), so C must be scheduled twice.
	st := &leveledState{
		target: 2, open: 2, comfort: 2,
		pairs: []string{"A", "B", "C"},
		met: map[string]map[string]struct{}{
			"A": {"B": {}}, "B": {"A": {}}, "C": {},
		},
		played:   map[string]int{"A": 0, "B": 0, "C": 0},
		pending:  map[string]int{"A": 1, "B": 1, "C": 0},
		position: map[string]int{"A": 0, "B": 1, "C": 2},
		occupied: map[string]map[int]bool{"A": {1: true}, "B": {1: true}, "C": {}},
	}
	svc := &Service{shuffle: func(_ int, _ func(int, int)) {}}

	pairings := plan(svc, st)

	countC := 0
	for _, p := range pairings {
		if p.A == "C" || p.B == "C" {
			countC++
		}
	}
	assert.Equal(t, 2, countC, "C must hold both remaining matches in the last Jornada")
	for _, p := range pairings {
		assert.Equal(t, st.target, p.Slot, "last-Jornada pairings must land at k=target")
	}
}

// =============================================================================
// DB-touching tests (GenerateInitialAssignments + TopUpAssignments)
// =============================================================================

// pendingMatchesFor returns pending match records that involve pairID.
func pendingMatchesFor(t *testing.T, app core.App, compID, pairID string) []*core.Record {
	t.Helper()
	matches, err := app.FindRecordsByFilter("matches",
		"competition = {:c} && status = 'pending' && (pair1 = {:p} || pair2 = {:p})",
		"", 0, 0,
		map[string]any{"c": compID, "p": pairID})
	require.NoError(t, err)
	return matches
}

// allMatchesFor returns all match records that involve pairID.
func allMatchesFor(t *testing.T, app core.App, compID, pairID string) []*core.Record {
	t.Helper()
	matches, err := app.FindRecordsByFilter("matches",
		"competition = {:c} && (pair1 = {:p} || pair2 = {:p})",
		"", 0, 0,
		map[string]any{"c": compID, "p": pairID})
	require.NoError(t, err)
	return matches
}

// hasDuplicatePairing returns true if any two pairs appear more than once
// in the same match within the competition.
func hasDuplicatePairing(t *testing.T, app core.App, compID string) bool {
	t.Helper()
	matches, err := app.FindRecordsByFilter("matches",
		"competition = {:c}", "", 0, 0, map[string]any{"c": compID})
	require.NoError(t, err)
	type pair struct{ a, b string }
	seen := map[pair]bool{}
	for _, m := range matches {
		p1, p2 := m.GetString("pair1"), m.GetString("pair2")
		if p1 > p2 {
			p1, p2 = p2, p1
		}
		k := pair{p1, p2}
		if seen[k] {
			return true
		}
		seen[k] = true
	}
	return false
}

// -- TestGenerateInitialAssignments ----------------------------------------

func TestGenerateInitialAssignments(t *testing.T) {
	app := newTestApp(t)
	pairs := make([]*core.Record, 6)
	for i := range pairs {
		pairs[i] = makePair(t, app, "Gen")
	}
	comp := makeLeveledCompetition(t, app, pairs, 4, 2)

	svc := newDeterministicSvc(app)
	svc.SetShuffle(func(_ int, _ func(int, int)) {}) // deterministic for count assertions
	n, err := svc.GenerateInitialAssignments(app, comp, time.Now())
	require.NoError(t, err)
	assert.Greater(t, n, 0, "should create at least one match")

	// No duplicate pairings.
	assert.False(t, hasDuplicatePairing(t, app, comp.Id), "no duplicate pairings")

	// Every pair gets 2 or 3 pending matches.
	for _, p := range pairs {
		ms := pendingMatchesFor(t, app, comp.Id, p.Id)
		cnt := len(ms)
		assert.GreaterOrEqual(t, cnt, 2, "pair %s: expected >=2 pending, got %d", p.Id, cnt)
		assert.LessOrEqual(t, cnt, 3, "pair %s: expected <=3 pending, got %d", p.Id, cnt)
		// All matches have round_number=0 and status=pending.
		for _, m := range ms {
			assert.Equal(t, 0, m.GetInt("round_number"))
			assert.Equal(t, "pending", m.GetString("status"))
		}
	}
}

// -- TestGenerateInitialAssignments_Seeded ----------------------------------

func TestGenerateInitialAssignments_Seeded(t *testing.T) {
	app := newTestApp(t)
	pairs := make([]*core.Record, 6)
	for i := range pairs {
		pairs[i] = makePair(t, app, "Seed")
	}
	comp := makeLeveledCompetition(t, app, pairs, 4, 2)

	// pairs[0] = strongest (advanced_high, Elo 300) down to pairs[5] =
	// weakest (beginner, Elo -300) — 6 distinct levels 100 Elo apart, so
	// every adjacent pair sits exactly at the eloComfortZone boundary.
	seedIDs := make([]string, len(pairs))
	strongestFirst := []string{"advanced_high", "advanced", "intermediate_high", "intermediate", "intermediate_low", "beginner"}
	elo := make(map[string]float64, len(pairs))
	for i, p := range pairs {
		seedIDs[i] = p.Id
		p.Set("level", strongestFirst[i])
		require.NoError(t, app.Save(p))
		elo[p.Id] = LevelElo(strongestFirst[i])
	}

	// Identity shuffle: keep in order so results are deterministic.
	svc := newDeterministicSvc(app)
	svc.shuffle = func(_ int, _ func(i, j int)) {}

	_, err := svc.GenerateInitialAssignments(app, comp, time.Now())
	require.NoError(t, err)

	// With only 6 pairs and target=4, each pair meets 4 of the other 5 —
	// near a full round robin — so the completion check forces some
	// matches outside the 100-Elo comfort zone no matter the distance
	// metric (this held for the old rank-distance version too). The exact
	// pairing set below was captured from 5 independent runs of this exact
	// fixture (identity shuffle, same seed levels) that all produced the
	// identical result — completable()'s own internal randomness (leveled.
	// go's tryGreedy tries up to 100 random orderings) doesn't change the
	// outcome for a fixture this small, so the set is a genuine regression
	// pin, not an arbitrary copy: a change to the distance metric, the
	// comfort width, or the candidate ordering would very likely produce a
	// different set and fail this test loudly.
	wantOpponents := map[int][]int{
		0: {1, 2},    // advanced_high (300): both within comfort of p1 (gap 100)
		1: {0, 3},    // advanced (200): p0 in comfort (100); p3 forced outside (200)
		2: {3, 0, 5}, // intermediate_high (100): p3 in comfort (100); p0, p5 forced outside
		3: {2, 1, 4}, // intermediate (0): p2 in comfort (100); p1, p4 forced outside
		4: {5, 3},    // intermediate_low (-100): p5 in comfort (100); p3 forced outside
		5: {4, 2},    // beginner (-300): no comfort opponent exists (nearest is p4, gap 200)
	}
	for i, p := range pairs {
		ms := pendingMatchesFor(t, app, comp.Id, p.Id)
		gotOpponents := make([]int, 0, len(ms))
		for _, m := range ms {
			oppID := m.GetString("pair1")
			if oppID == p.Id {
				oppID = m.GetString("pair2")
			}
			for j, other := range pairs {
				if other.Id == oppID {
					gotOpponents = append(gotOpponents, j)
				}
			}
		}
		assert.ElementsMatch(t, wantOpponents[i], gotOpponents,
			"pair %d (elo %.0f): opponent set changed", i, elo[p.Id])
	}
}

// -- TestGenerateInitialAssignments_HomeAwayBalance --------------------------

// homeAwayCounts tallies how many times each pair appears as pair1 (home)
// and pair2 (away) across all matches in comp.
func homeAwayCounts(t *testing.T, app core.App, compID string, pairs []*core.Record) map[string][2]int {
	t.Helper()
	counts := map[string][2]int{}
	for _, p := range pairs {
		counts[p.Id] = [2]int{}
	}
	matches, err := app.FindRecordsByFilter("matches", "competition = {:c}", "", 0, 0,
		map[string]any{"c": compID})
	require.NoError(t, err)
	for _, m := range matches {
		p1, p2 := m.GetString("pair1"), m.GetString("pair2")
		c := counts[p1]
		c[0]++
		counts[p1] = c
		c = counts[p2]
		c[1]++
		counts[p2] = c
	}
	return counts
}

func TestGenerateInitialAssignments_HomeAwayBalance(t *testing.T) {
	app := newTestApp(t)
	pairs := make([]*core.Record, 8)
	for i := range pairs {
		pairs[i] = makePair(t, app, "HA")
	}
	comp := makeLeveledCompetition(t, app, pairs, 5, 2)

	svc := newDeterministicSvc(app)
	_, err := svc.GenerateInitialAssignments(app, comp, time.Now())
	require.NoError(t, err)

	for id, c := range homeAwayCounts(t, app, comp.Id, pairs) {
		home, away := c[0], c[1]
		diff := home - away
		if diff < 0 {
			diff = -diff
		}
		assert.LessOrEqual(t, diff, 1, "pair %s: home=%d away=%d, imbalance > 1", id, home, away)
	}
}

// TestTopUp_PicksAwayForHomeHeavyPair verifies a pair that has already played
// its persisted matches at home gets assigned away on its next top-up match.
func TestTopUp_PicksAwayForHomeHeavyPair(t *testing.T) {
	app := newTestApp(t)
	pa := makePair(t, app, "HA A")
	pb := makePair(t, app, "HA B")
	pc := makePair(t, app, "HA C")
	pd := makePair(t, app, "HA D")

	comp := makeLeveledCompetition(t, app, []*core.Record{pa, pb, pc, pd}, 2, 1)

	now := time.Now()
	// pa is home (pair1) in a final match against pb: home=1, away=0, load=1.
	makeLeveledMatch(t, app, comp.Id, pa.Id, pb.Id, "6-0 6-0", pa.Id, "final", now.Add(-time.Hour))

	svc := newDeterministicSvc(app)
	created, err := svc.TopUpAssignments(comp.Id, now)
	require.NoError(t, err)

	found := false
	for _, m := range created {
		if m.GetString("pair1") == pa.Id || m.GetString("pair2") == pa.Id {
			found = true
			assert.Equal(t, pa.Id, m.GetString("pair2"), "pa is home-heavy; its fresh opponent should be home, pa away")
		}
	}
	require.True(t, found, "pa should have received a top-up match")
}

// -- TestTopUp_OrdersByRating -----------------------------------------------

func TestTopUp_OrdersByRating(t *testing.T) {
	app := newTestApp(t)
	// 4 pairs with seed order so ratings are well-separated.
	pa := makePair(t, app, "TopA")
	pb := makePair(t, app, "TopB")
	pc := makePair(t, app, "TopC")
	pd := makePair(t, app, "TopD")
	pairs := []*core.Record{pa, pb, pc, pd}

	comp := makeLeveledCompetition(t, app, pairs, 3, 2)
	// pa strongest .. pd weakest, distinct levels so rating order is fixed.
	levels := []string{"advanced_high", "advanced", "intermediate", "beginner"}
	for i, p := range pairs {
		p.Set("level", levels[i])
		require.NoError(t, app.Save(p))
	}

	svc := newDeterministicSvc(app)
	// Level gives pa strong rating. Give pa a final match against pb already,
	// leaving pa needing opponents.
	now := time.Now()
	makeLeveledMatch(t, app, comp.Id, pa.Id, pb.Id, "6-0 6-0", pa.Id, "final", now.Add(-time.Hour))

	created, err := svc.TopUpAssignments(comp.Id, now)
	require.NoError(t, err)

	// pa (top seed) should get an opponent from inside its comfort zone (dist ≤ 2),
	// never pd (pos 3, dist=3 for pa at pos 0 in a 4-pair league — comfort=ceil(3/2)=2).
	for _, m := range created {
		if m.GetString("pair1") == pa.Id || m.GetString("pair2") == pa.Id {
			oppID := m.GetString("pair1")
			if oppID == pa.Id {
				oppID = m.GetString("pair2")
			}
			assert.NotEqual(t, pd.Id, oppID,
				"top-rated pair should not be matched with pair 3 positions away when comfort=2")
		}
	}
}

// -- TestTopUp_TargetIsHardCap ----------------------------------------------

func TestTopUp_TargetIsHardCap(t *testing.T) {
	app := newTestApp(t)
	pa := makePair(t, app, "Cap A")
	pb := makePair(t, app, "Cap B")
	pc := makePair(t, app, "Cap C")

	// target=2, open=2: 3 pairs so 3 < 3-1=2 is false. Use target=1, 3 pairs (1 < 2 → leveled).
	comp := makeLeveledCompetition(t, app, []*core.Record{pa, pb, pc}, 1, 1)

	now := time.Now()
	// pa already at target: 1 final match.
	makeLeveledMatch(t, app, comp.Id, pa.Id, pb.Id, "6-0 6-0", pa.Id, "final", now.Add(-time.Hour))

	svc := newDeterministicSvc(app)
	created, err := svc.TopUpAssignments(comp.Id, now)
	require.NoError(t, err)

	// pa must not appear in any new match.
	for _, m := range created {
		assert.NotEqual(t, pa.Id, m.GetString("pair1"), "pair at target must not get new assignment")
		assert.NotEqual(t, pa.Id, m.GetString("pair2"), "pair at target must not get new assignment")
	}
}

// -- TestTopUp_RequesterStopsAtOpen -----------------------------------------

// TestTopUp_RequesterStopsAtOpen verifies a pair at exactly `open` pending
// never becomes a requester (nextRequesterForRound must skip it via
// wants()). Levels pin the rating order so pc and pd are
// comfort-zone-adjacent (advanced/advanced_high) while pa/pb sit outside
// pc/pd's zone (beginner/beginner_high) — otherwise pa/pb's comfort-zone tie
// with a fresh opponent (both at the same rating distance) can make the
// requester pick pa/pb regardless of correctness, which would make this
// fixture unable to isolate the requester-role bug from ordinary opponent
// selection (see TestEligible "pending == open exactly is eligible" — an
// opponent at pending==open is a separate, allowed path, not what this test
// checks).
func TestTopUp_RequesterStopsAtOpen(t *testing.T) {
	app := newTestApp(t)
	pa := makePair(t, app, "Open A")
	pb := makePair(t, app, "Open B")
	pc := makePair(t, app, "Open C")
	pd := makePair(t, app, "Open D")
	pa.Set("level", "beginner")
	pb.Set("level", "beginner_high")
	pc.Set("level", "advanced")
	pd.Set("level", "advanced_high")
	require.NoError(t, app.Save(pa))
	require.NoError(t, app.Save(pb))
	require.NoError(t, app.Save(pc))
	require.NoError(t, app.Save(pd))

	// target=2, open=1, 4 pairs.
	comp := makeLeveledCompetition(t, app, []*core.Record{pa, pb, pc, pd}, 2, 1)

	now := time.Now()
	// pa already has open=1 pending match (with pb), so wants(pa)=false.
	// pc and pd have never met and are each other's comfort-zone match, so
	// the top-up should simply pair them together without ever touching pa.
	makeLeveledMatch(t, app, comp.Id, pa.Id, pb.Id, "", "", "pending", time.Time{})

	svc := newDeterministicSvc(app)
	created, err := svc.TopUpAssignments(comp.Id, now)
	require.NoError(t, err)
	require.NotEmpty(t, created, "pc/pd still want a match, so a top-up must create one")

	for _, m := range created {
		assert.NotEqual(t, pa.Id, m.GetString("pair1"), "pair at open pending should not be requester")
		assert.NotEqual(t, pa.Id, m.GetString("pair2"), "pair at open pending should not be requester")
	}
}

// -- TestTopUp_SkipsMetAndPending ------------------------------------------

func TestTopUp_SkipsMetAndPending(t *testing.T) {
	app := newTestApp(t)
	pa := makePair(t, app, "Met A")
	pb := makePair(t, app, "Met B")
	pc := makePair(t, app, "Met C")
	pd := makePair(t, app, "Met D")

	comp := makeLeveledCompetition(t, app, []*core.Record{pa, pb, pc, pd}, 2, 2)

	// pa already has a pending match against pb.
	makeLeveledMatch(t, app, comp.Id, pa.Id, pb.Id, "", "", "pending", time.Time{})

	svc := newDeterministicSvc(app)
	created, err := svc.TopUpAssignments(comp.Id, time.Now())
	require.NoError(t, err)

	// No new match should pair pa and pb again.
	for _, m := range created {
		p1, p2 := m.GetString("pair1"), m.GetString("pair2")
		isPB := (p1 == pa.Id && p2 == pb.Id) || (p1 == pb.Id && p2 == pa.Id)
		assert.False(t, isPB, "pa and pb are already pending — must not be re-paired")
	}
}

// -- TestTopUp_Avoid --------------------------------------------------------

func TestTopUp_Avoid(t *testing.T) {
	app := newTestApp(t)
	pa := makePair(t, app, "Av A")
	pb := makePair(t, app, "Av B")
	pc := makePair(t, app, "Av C")
	pd := makePair(t, app, "Av D")

	comp := makeLeveledCompetition(t, app, []*core.Record{pa, pb, pc, pd}, 2, 2)

	svc := newDeterministicSvc(app)
	// Avoid pa-pb.
	_, err := svc.TopUpAssignments(comp.Id, time.Now(), Pairing{A: pa.Id, B: pb.Id})
	require.NoError(t, err)

	// No new match should pair pa and pb.
	matches, err := app.FindRecordsByFilter("matches",
		"competition = {:c}", "", 0, 0, map[string]any{"c": comp.Id})
	require.NoError(t, err)
	for _, m := range matches {
		p1, p2 := m.GetString("pair1"), m.GetString("pair2")
		isPB := (p1 == pa.Id && p2 == pb.Id) || (p1 == pb.Id && p2 == pa.Id)
		assert.False(t, isPB, "avoided pairing must not be created")
	}
}

// -- TestTopUp_Idempotent ---------------------------------------------------

func TestTopUp_Idempotent(t *testing.T) {
	app := newTestApp(t)
	pairs := make([]*core.Record, 6)
	for i := range pairs {
		pairs[i] = makePair(t, app, "Idem")
	}
	comp := makeLeveledCompetition(t, app, pairs, 4, 2)

	svc := newDeterministicSvc(app)
	now := time.Now()

	first, err := svc.TopUpAssignments(comp.Id, now)
	require.NoError(t, err)

	second, err := svc.TopUpAssignments(comp.Id, now)
	require.NoError(t, err)

	assert.Greater(t, len(first), 0, "first call should create matches")
	assert.Empty(t, second, "second call with no change should create nothing (P6)")
}

// -- TestTopUp_NoOpWhenNotAssignable ----------------------------------------

func TestTopUp_NoOpWhenNotAssignable(t *testing.T) {
	app := newTestApp(t)
	pa := makePair(t, app, "NoOp A")
	pb := makePair(t, app, "NoOp B")
	pc := makePair(t, app, "NoOp C")
	pd := makePair(t, app, "NoOp D")
	svc := newDeterministicSvc(app)
	now := time.Now()

	t.Run("draft calendar", func(t *testing.T) {
		comp := makeLeveledCompetition(t, app, []*core.Record{pa, pb, pc, pd}, 2, 2)
		comp.Set("calendar_status", "draft")
		require.NoError(t, app.Save(comp))
		created, err := svc.TopUpAssignments(comp.Id, now)
		require.NoError(t, err)
		assert.Empty(t, created)
	})

	t.Run("finalized competition", func(t *testing.T) {
		comp := makeLeveledCompetition(t, app, []*core.Record{pa, pb, pc, pd}, 2, 2)
		comp.Set("finalized", true)
		require.NoError(t, app.Save(comp))
		created, err := svc.TopUpAssignments(comp.Id, now)
		require.NoError(t, err)
		assert.Empty(t, created)
	})

	t.Run("past end_date", func(t *testing.T) {
		comp := makeLeveledCompetition(t, app, []*core.Record{pa, pb, pc, pd}, 2, 2)
		past := now.Add(-48 * time.Hour)
		pastDT := past.Format("2006-01-02")
		comp.Set("end_date", pastDT)
		require.NoError(t, app.Save(comp))
		created, err := svc.TopUpAssignments(comp.Id, now)
		require.NoError(t, err)
		assert.Empty(t, created)
	})

	t.Run("round-robin (target=0)", func(t *testing.T) {
		comp := makeCompetition(t, app, []*core.Record{pa, pb, pc, pd})
		// target_matches defaults to 0, so IsLeveled = false
		created, err := svc.TopUpAssignments(comp.Id, now)
		require.NoError(t, err)
		assert.Empty(t, created)
	})

	t.Run("missing competition", func(t *testing.T) {
		created, err := svc.TopUpAssignments("nonexistent-id", now)
		assert.NoError(t, err)
		assert.Nil(t, created)
	})
}

// -- TestTopUp_ExcludesWithdrawn -------------------------------------------

func TestTopUp_ExcludesWithdrawn(t *testing.T) {
	app := newTestApp(t)
	pa := makePair(t, app, "Wd A")
	pb := makePair(t, app, "Wd B")
	pc := makePair(t, app, "Wd C")
	pd := makePair(t, app, "Wd D")

	comp := makeLeveledCompetition(t, app, []*core.Record{pa, pb, pc, pd}, 2, 2)
	// Withdraw pa.
	comp.Set("withdrawn_pairs", []string{pa.Id})
	require.NoError(t, app.Save(comp))

	svc := newDeterministicSvc(app)
	created, err := svc.TopUpAssignments(comp.Id, time.Now())
	require.NoError(t, err)

	for _, m := range created {
		assert.NotEqual(t, pa.Id, m.GetString("pair1"), "withdrawn pair must not be assigned")
		assert.NotEqual(t, pa.Id, m.GetString("pair2"), "withdrawn pair must not be assigned")
	}
}

// -- TestTopUp_NotifiesBothPairs --------------------------------------------

func TestTopUp_NotifiesBothPairs(t *testing.T) {
	app := newTestApp(t)
	pa := makePair(t, app, "Ntf A")
	pb := makePair(t, app, "Ntf B")
	pc := makePair(t, app, "Ntf C")
	pd := makePair(t, app, "Ntf D")
	pairs := []*core.Record{pa, pb, pc, pd}

	comp := makeLeveledCompetition(t, app, pairs, 2, 2)

	notifier := &fakeNotifier{}
	svc := New(app, notifier)
	created, err := svc.TopUpAssignments(comp.Id, time.Now())
	require.NoError(t, err)
	require.NotEmpty(t, created, "should create at least one match")

	pairNames := PairNames(app, comp.GetStringSlice("pairs"))

	// Two calls per created match (one per pair), each with 2 player IDs and
	// the OTHER pair's name in the body — never the recipient's own name.
	assert.Len(t, notifier.calls, 2*len(created),
		"two notification calls per created match, one per pair")
	for _, m := range created {
		p1ID, p2ID := m.GetString("pair1"), m.GetString("pair2")
		p1Players := PlayersForPair(app, p1ID)
		p2Players := PlayersForPair(app, p2ID)

		call1 := findNotifyCallFor(t, notifier.calls, m.Id, p1Players)
		assert.Equal(t, "match_assigned", call1.notifType)
		assert.Contains(t, call1.body, pairNames[p2ID], "pair1's players should be told pair2's name")
		assert.NotContains(t, call1.body, pairNames[p1ID], "pair1's players should not be told their own name")

		call2 := findNotifyCallFor(t, notifier.calls, m.Id, p2Players)
		assert.Equal(t, "match_assigned", call2.notifType)
		assert.Contains(t, call2.body, pairNames[p1ID], "pair2's players should be told pair1's name")
		assert.NotContains(t, call2.body, pairNames[p2ID], "pair2's players should not be told their own name")
	}
}

// findNotifyCallFor returns the notification call for matchID addressed to
// exactly playerIDs.
func findNotifyCallFor(t *testing.T, calls []notifyCall, matchID string, playerIDs []string) notifyCall {
	t.Helper()
	for _, call := range calls {
		if call.matchID != matchID || len(call.playerIDs) != len(playerIDs) {
			continue
		}
		match := true
		for _, id := range playerIDs {
			if !slices.Contains(call.playerIDs, id) {
				match = false
				break
			}
		}
		if match {
			return call
		}
	}
	require.Fail(t, "no notify call found", "matchID=%s playerIDs=%v", matchID, playerIDs)
	return notifyCall{}
}

// -- TestLeveledSeason_Invariants ------------------------------------------

func TestLeveledSeason_Invariants(t *testing.T) {
	app := newTestApp(t)
	pairs := make([]*core.Record, 8)
	for i := range pairs {
		pairs[i] = makePair(t, app, "Inv")
	}
	comp := makeLeveledCompetition(t, app, pairs, 5, 2)

	levelKeys := []string{"advanced_high", "advanced", "intermediate_high", "intermediate",
		"intermediate_low", "beginner_high", "beginner", "intermediate"}
	for i, p := range pairs {
		p.Set("level", levelKeys[i])
		require.NoError(t, app.Save(p))
	}

	svc := newDeterministicSvc(app)
	now := time.Now()

	// Generate initial assignments.
	_, err := svc.GenerateInitialAssignments(app, comp, now)
	require.NoError(t, err)

	checkInvariants := func(step string) {
		t.Helper()
		// P1: no duplicate pairings.
		assert.False(t, hasDuplicatePairing(t, app, comp.Id), "%s: P1 duplicate pairing", step)

		// P2 + P3: per pair.
		for _, p := range pairs {
			allMs := allMatchesFor(t, app, comp.Id, p.Id)
			pendMs := pendingMatchesFor(t, app, comp.Id, p.Id)
			played := 0
			for _, m := range allMs {
				if m.GetString("status") == "final" {
					played++
				}
			}
			load := played + len(pendMs)
			assert.LessOrEqual(t, load, 5, "%s: P2 pair %s load %d > target 5", step, p.Id, load)
			assert.LessOrEqual(t, len(pendMs), 3, "%s: P3 pair %s pending %d > open+1=3", step, p.Id, len(pendMs))
		}
	}

	checkInvariants("after generate")

	rng := rand.New(rand.NewPCG(42, 0))

	// Simulate season: finalize random pending matches, top up.
	for iter := range 40 {
		allPending, err := app.FindRecordsByFilter("matches",
			"competition = {:c} && status = 'pending'",
			"", 0, 0, map[string]any{"c": comp.Id})
		require.NoError(t, err)
		if len(allPending) == 0 {
			break
		}

		// Finalize a random pending match.
		m := allPending[rng.IntN(len(allPending))]
		winner := m.GetString("pair1")
		m.Set("status", "final")
		m.Set("scores", "6-3 6-4")
		m.Set("winner", winner)
		m.Set("finalized_at", now.Add(time.Duration(iter)*time.Hour).Format("2006-01-02"))
		require.NoError(t, app.Save(m))

		_, err = svc.TopUpAssignments(comp.Id, now)
		require.NoError(t, err)

		checkInvariants("iter " + time.Now().Format("05"))
	}

	// P5: every pair ends on exactly target.
	for _, p := range pairs {
		allMs := allMatchesFor(t, app, comp.Id, p.Id)
		played := 0
		for _, m := range allMs {
			if m.GetString("status") == "final" {
				played++
			}
		}
		pendMs := pendingMatchesFor(t, app, comp.Id, p.Id)
		load := played + len(pendMs)
		assert.Equal(t, 5, load,
			"P5: pair %s should end on exactly target=5, got load=%d", p.Id, load)
	}
}

// -- Slot assignment ---------------------------------------------------------

func TestPlan_SlotAssignment(t *testing.T) {
	app := newTestApp(t)
	pairs := make([]*core.Record, 16)
	for i := range pairs {
		pairs[i] = makePair(t, app, "Slot")
	}
	comp := makeLeveledCompetition(t, app, pairs, 6, 3)
	start := time.Now().Add(-7 * 24 * time.Hour)
	end := time.Now().Add(90 * 24 * time.Hour)
	comp.Set("start_date", start.Format(time.RFC3339))
	comp.Set("end_date", end.Format(time.RFC3339))
	require.NoError(t, app.Save(comp))

	svc := newDeterministicSvc(app)
	now := time.Now()
	_, err := svc.GenerateInitialAssignments(app, comp, now)
	require.NoError(t, err)

	matches, err := app.FindRecordsByFilter("matches", "competition = {:c}", "", 0, 0,
		map[string]any{"c": comp.Id})
	require.NoError(t, err)

	slotsByPair := map[string][]int{}
	for _, m := range matches {
		slotsByPair[m.GetString("pair1")] = append(slotsByPair[m.GetString("pair1")], m.GetInt("slot"))
		slotsByPair[m.GetString("pair2")] = append(slotsByPair[m.GetString("pair2")], m.GetInt("slot"))
	}

	// Round-fill assigns one match per pair per Jornada: every pair's
	// matches land on distinct slots within 1..open, never repeating a
	// round and never spilling past open.
	for _, p := range pairs {
		slots := slotsByPair[p.Id]
		sort.Ints(slots)
		require.NotEmpty(t, slots, "pair %s should have assigned matches", p.Id)
		seen := map[int]bool{}
		for _, s := range slots {
			assert.False(t, seen[s], "pair %s must never hold two matches in slot %d", p.Id, s)
			seen[s] = true
			assert.LessOrEqual(t, s, 3, "pair %s slot must not exceed open=3", p.Id)
		}
	}

	// 16 pairs, open=3 → exactly 3 Jornadas of 8 matches each.
	slotCounts := map[int]int{}
	for _, m := range matches {
		slotCounts[m.GetInt("slot")]++
	}
	assert.Equal(t, map[int]int{1: 8, 2: 8, 3: 8}, slotCounts,
		"16 pairs, open=3 must produce exactly 3 even Jornadas")
}

// TestPlan_TopUpSlot verifies a top-up assigned well into the season gets a
// slot at or above the current calendar position, even though load-based
// slots alone would put it back in the initial-batch range (finalizing a
// match leaves a pair's load unchanged: played+1, pending-1).
func TestPlan_TopUpSlot(t *testing.T) {
	app := newTestApp(t)
	pairs := make([]*core.Record, 16)
	for i := range pairs {
		pairs[i] = makePair(t, app, "TopUpSlot")
	}
	comp := makeLeveledCompetition(t, app, pairs, 10, 2)
	start := time.Now().Add(-50 * 24 * time.Hour)
	end := time.Now().Add(50 * 24 * time.Hour) // 100-day window, u=10d
	comp.Set("start_date", start.Format(time.RFC3339))
	comp.Set("end_date", end.Format(time.RFC3339))
	require.NoError(t, app.Save(comp))

	svc := newDeterministicSvc(app)
	now := time.Now()
	_, err := svc.GenerateInitialAssignments(app, comp, now)
	require.NoError(t, err)

	initial, err := app.FindRecordsByFilter("matches", "competition = {:c}", "", 0, 0,
		map[string]any{"c": comp.Id})
	require.NoError(t, err)

	// Finalize every pending match for one pair so it drops below `open`
	// and wants() a new assignment again. Top up at the same "now" — 50
	// days into a 100-day/target-10 window (u=10d) puts the current window
	// at floor(50/10)+1 = 6.
	targetPair := initial[0].GetString("pair1")
	var lastOpponent string
	for _, m := range initial {
		if m.GetString("pair1") != targetPair && m.GetString("pair2") != targetPair {
			continue
		}
		lastOpponent = m.GetString("pair2")
		if lastOpponent == targetPair {
			lastOpponent = m.GetString("pair1")
		}
		m.Set("status", "final")
		m.Set("scores", "6-3 6-4")
		m.Set("winner", targetPair)
		require.NoError(t, app.Save(m))
	}

	created, err := svc.TopUpAssignments(comp.Id, now, Pairing{A: targetPair, B: lastOpponent})
	require.NoError(t, err)
	require.NotEmpty(t, created, "top-up should create new matches")

	for _, nm := range created {
		assert.GreaterOrEqual(t, nm.GetInt("slot"), 6,
			"top-up slot must be floored to the current calendar window")
	}
}

func TestPlan_CalendarFloor(t *testing.T) {
	app := newTestApp(t)
	pairs := make([]*core.Record, 6)
	for i := range pairs {
		pairs[i] = makePair(t, app, "CalFloor")
	}
	comp := makeLeveledCompetition(t, app, pairs, 10, 2)
	start := time.Now().Add(-50 * 24 * time.Hour) // well in the past
	end := time.Now().Add(50 * 24 * time.Hour)    // 100-day window, now = start+50d
	comp.Set("start_date", start.Format(time.RFC3339))
	comp.Set("end_date", end.Format(time.RFC3339))
	require.NoError(t, app.Save(comp))

	svc := newDeterministicSvc(app)
	now := time.Now()
	_, err := svc.GenerateInitialAssignments(app, comp, now)
	require.NoError(t, err)

	// u = 100d/10 = 10d; now is ~50d after start → current window = floor(50/10)+1 = 6.
	matches, err := app.FindRecordsByFilter("matches", "competition = {:c}", "", 0, 0,
		map[string]any{"c": comp.Id})
	require.NoError(t, err)
	for _, m := range matches {
		assert.GreaterOrEqual(t, m.GetInt("slot"), 6,
			"slot must be floored to the current calendar window, not the pair's raw ordinal")
	}
}

// TestPlan_LateGeneration verifies that generating a calendar for a
// competition whose window already started skips the closed Jornadas
// entirely: with start=7 days ago, target=10 (u=7d), the current window is
// floor(7/7)+1=2, so the initial batch must land in Jornada 2 or later —
// never Jornada 1, and no match's deadline is in the past. This pins the
// scenario the owner hit: an admin who adds dates and generates a few days
// late must not see the app assign matches to an already-closed Jornada.
func TestPlan_LateGeneration(t *testing.T) {
	app := newTestApp(t)
	pairs := make([]*core.Record, 6)
	for i := range pairs {
		pairs[i] = makePair(t, app, "LateGen")
	}
	comp := makeLeveledCompetition(t, app, pairs, 10, 2)
	start := time.Now().Add(-7 * 24 * time.Hour)
	end := start.Add(70 * 24 * time.Hour) // u = 70d/10 = 7d
	comp.Set("start_date", start.Format(time.RFC3339))
	comp.Set("end_date", end.Format(time.RFC3339))
	require.NoError(t, app.Save(comp))

	svc := newDeterministicSvc(app)
	now := time.Now()
	_, err := svc.GenerateInitialAssignments(app, comp, now)
	require.NoError(t, err)

	matches, err := app.FindRecordsByFilter("matches", "competition = {:c}", "", 0, 0,
		map[string]any{"c": comp.Id})
	require.NoError(t, err)
	require.NotEmpty(t, matches)
	for _, m := range matches {
		slot := m.GetInt("slot")
		assert.GreaterOrEqual(t, slot, 2, "no match may land in the already-closed Jornada 1")
		assert.LessOrEqual(t, slot, 4, "open=2 initial batch must not spill past cur+open=4 (a round-skipped pair retries next Jornada)")
		deadline, ok := SlotDeadline(comp, slot)
		require.True(t, ok)
		assert.False(t, deadline.Before(now), "a generated match's arrange_by must never be in the past")
	}
}

// TestPlan_NegativeWindow covers the u<=0 guard: when end<start the pace
// unit is negative, so currentWindow must fall back to 1 instead of computing
// a nonsensical calendar position. Unreachable via the handler (fixtures.go
// rejects end<=start at creation), but the guard exists in currentWindow
// itself and must be verified in isolation.
func TestPlan_NegativeWindow(t *testing.T) {
	now := time.Now()
	st := &leveledState{
		target: 5, open: 2, comfort: 3,
		pairs:    []string{"p", "q"},
		met:      map[string]map[string]struct{}{"p": {}, "q": {}},
		played:   map[string]int{"p": 0, "q": 0},
		pending:  map[string]int{"p": 0, "q": 0},
		position: map[string]int{"p": 0, "q": 1},
		occupied: map[string]map[int]bool{"p": {}, "q": {}},
		start:    now.Add(-time.Hour),
		end:      now.Add(-2 * time.Hour), // end < start → u < 0
		now:      now,
	}
	svc := &Service{shuffle: func(_ int, _ func(int, int)) {}}

	pairings := plan(svc, st)

	require.NotEmpty(t, pairings)
	assert.Equal(t, 1, pairings[0].Slot, "u<0 must fall back to starting at slot 1")
}

func TestSlotCap(t *testing.T) {
	app := newTestApp(t)
	pairs := make([]*core.Record, 4)
	for i := range pairs {
		pairs[i] = makePair(t, app, "SlotCap")
	}
	comp := makeLeveledCompetition(t, app, pairs, 2, 2)
	start := time.Now().Add(-90 * 24 * time.Hour)
	end := time.Now().Add(-1 * 24 * time.Hour) // window already past → forces a high calendar floor
	comp.Set("start_date", start.Format(time.RFC3339))
	comp.Set("end_date", end.Format(time.RFC3339))
	require.NoError(t, app.Save(comp))

	svc := newDeterministicSvc(app)
	now := time.Now()
	_, err := svc.GenerateInitialAssignments(app, comp, now)
	require.NoError(t, err)

	matches, err := app.FindRecordsByFilter("matches", "competition = {:c}", "", 0, 0,
		map[string]any{"c": comp.Id})
	require.NoError(t, err)
	for _, m := range matches {
		assert.LessOrEqual(t, m.GetInt("slot"), 2, "stored slot must never exceed target_matches")
	}
}
