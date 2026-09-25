package league

import (
	"math/rand/v2"
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
	// 4 pairs at positions 0,1,2,3. Requester is position 0. target=4, open=2.
	// comfort = ceil(4/2) = 2. Zone for pos 0: d <= 2 → positions 1,2.
	// Position 3 is outside the zone.
	st := &leveledState{
		target:   4,
		open:     2,
		comfort:  2,
		pairs:    []string{"A", "B", "C", "D"},
		met:      map[string]map[string]struct{}{"A": {}, "B": {}, "C": {}, "D": {}},
		played:   map[string]int{"A": 0, "B": 0, "C": 0, "D": 0},
		pending:  map[string]int{"A": 0, "B": 0, "C": 0, "D": 0},
		position: map[string]int{"A": 0, "B": 1, "C": 2, "D": 3},
	}

	// identity shuffle: inside-zone candidates keep order [B(1), C(2)]
	// first candidate inside zone should be chosen.
	identityShuffle := func(_ int, _ func(int, int)) {}
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
		played:   map[string]int{"A": 0, "B": 0, "C": 0, "D": 0},
		pending:  map[string]int{"A": 0, "B": 0, "C": 0, "D": 0},
		position: map[string]int{"A": 0, "B": 1, "C": 2, "D": 3},
	}

	identityShuffle := func(_ int, _ func(int, int)) {}
	svc := &Service{shuffle: identityShuffle}
	got := svc.chooseOpponent(st, "A")
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
		met:     map[string]map[string]struct{}{},
		played:  map[string]int{"A": 0, "B": 4, "C": 1, "D": 0, "E": 2, "F": 2},
		pending: map[string]int{"A": 0, "B": 0, "C": 0, "D": 0, "E": 0, "F": 0},
		position: map[string]int{
			"A": 0, "B": 2, "C": -2, "D": 3, "E": -10, "F": 10,
		},
	}
	identityShuffle := func(_ int, _ func(int, int)) {}
	svc := &Service{shuffle: identityShuffle}

	_, outside := svc.collectCandidates(st, "A")
	require.Len(t, outside, 5)
	got := []string{outside[0].id, outside[1].id, outside[2].id, outside[3].id, outside[4].id}
	assert.Equal(t, []string{"C", "B", "D", "E", "F"}, got,
		"dist ties (B,C) break on load (C<B); D sorts last on dist; "+
			"E,F tie on dist+load, break on position (E<F)")
}

// Note: `a.dist < b.dist` and `a.load < b.load` in collectCandidates' sort.Slice
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

	// Seed order: pairs[0] = strongest, pairs[5] = weakest.
	seedIDs := make([]string, len(pairs))
	for i, p := range pairs {
		seedIDs[i] = p.Id
	}
	comp.Set("seed_pairs", seedIDs)
	require.NoError(t, app.Save(comp))

	// Identity shuffle: keep in order so we can predict opponent proximity.
	svc := newDeterministicSvc(app)
	svc.shuffle = func(_ int, _ func(i, j int)) {}

	_, err := svc.GenerateInitialAssignments(app, comp, time.Now())
	require.NoError(t, err)

	// Each pair's opponent should be within comfort=ceil(4/2)=2 places.
	for _, p := range pairs {
		ms := pendingMatchesFor(t, app, comp.Id, p.Id)
		myPos := -1
		for i, s := range seedIDs {
			if s == p.Id {
				myPos = i
				break
			}
		}
		for _, m := range ms {
			oppID := m.GetString("pair1")
			if oppID == p.Id {
				oppID = m.GetString("pair2")
			}
			oppPos := -1
			for i, s := range seedIDs {
				if s == oppID {
					oppPos = i
					break
				}
			}
			dist := myPos - oppPos
			if dist < 0 {
				dist = -dist
			}
			assert.LessOrEqual(t, dist, 2,
				"pair at pos %d got opponent at pos %d (dist %d > comfort 2)", myPos, oppPos, dist)
		}
	}
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
	comp.Set("seed_pairs", []string{pa.Id, pb.Id, pc.Id, pd.Id})
	require.NoError(t, app.Save(comp))

	svc := newDeterministicSvc(app)
	// Seed gives pa strong rating. Give pa a final match against pb already,
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

func TestTopUp_RequesterStopsAtOpen(t *testing.T) {
	app := newTestApp(t)
	pa := makePair(t, app, "Open A")
	pb := makePair(t, app, "Open B")
	pc := makePair(t, app, "Open C")
	pd := makePair(t, app, "Open D")

	// target=3, open=1, 4 pairs (3 < 3 is false — use target=2, open=1, 4 pairs: 2 < 3 → leveled).
	comp := makeLeveledCompetition(t, app, []*core.Record{pa, pb, pc, pd}, 2, 1)

	now := time.Now()
	// pa already has open=1 pending match.
	makeLeveledMatch(t, app, comp.Id, pa.Id, pb.Id, "", "", "pending", time.Time{})

	svc := newDeterministicSvc(app)
	created, err := svc.TopUpAssignments(comp.Id, now)
	require.NoError(t, err)

	// pa should not be a requester (already at open pending).
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

	comp := makeLeveledCompetition(t, app, []*core.Record{pa, pb, pc, pd}, 2, 2)

	notifier := &fakeNotifier{}
	svc := New(app, notifier)
	created, err := svc.TopUpAssignments(comp.Id, time.Now())
	require.NoError(t, err)
	require.NotEmpty(t, created, "should create at least one match")

	// One call per created match, 4 player IDs (2 pairs × 2 players).
	assert.Len(t, notifier.calls, len(created),
		"one notification call per created match")
	for _, call := range notifier.calls {
		assert.Equal(t, "match_assigned", call.notifType)
		assert.Len(t, call.playerIDs, 4,
			"notification should include 4 player IDs (both pairs)")
	}
}

// -- TestLeveledSeason_Invariants ------------------------------------------

func TestLeveledSeason_Invariants(t *testing.T) {
	app := newTestApp(t)
	pairs := make([]*core.Record, 8)
	for i := range pairs {
		pairs[i] = makePair(t, app, "Inv")
	}
	comp := makeLeveledCompetition(t, app, pairs, 5, 2)

	seedIDs := make([]string, len(pairs))
	for i, p := range pairs {
		seedIDs[i] = p.Id
	}
	comp.Set("seed_pairs", seedIDs)
	require.NoError(t, app.Save(comp))

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

	// slot = smallest slot free for BOTH pairs (hole-filling): every pair's
	// own matches land on DISTINCT slots — never two matches in the same
	// Jornada for one pair, so a collision-free schedule is always
	// achievable. Needing a slot free for BOTH sides (not each pair's own
	// smallest independently) can occasionally leave a hole for one side —
	// its own next free slot was already taken by the other pair's
	// unrelated match — and can push a pairing past open+1=4; neither ever
	// goes past open+2=5 in this fixture (16 pairs, open 3).
	for _, p := range pairs {
		slots := slotsByPair[p.Id]
		sort.Ints(slots)
		require.NotEmpty(t, slots, "pair %s should have assigned matches", p.Id)
		seen := map[int]bool{}
		for _, s := range slots {
			assert.False(t, seen[s], "pair %s must never hold two matches in slot %d", p.Id, s)
			seen[s] = true
			assert.LessOrEqual(t, s, 5, "pair %s slot must not exceed open+2=5", p.Id)
		}
	}

	// Exactly `open` (3) Jornada groups worth of slots (1..3) should exist;
	// slot 4 is the documented "one above open" overflow, present for at
	// most a few pairs, never forming its own full Jornada.
	slotCounts := map[int]int{}
	for _, m := range matches {
		slotCounts[m.GetInt("slot")]++
	}
	for s := 1; s <= 3; s++ {
		assert.Positive(t, slotCounts[s], "slot %d should have matches (Jornada %d)", s, s)
	}
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

// TestSlotFor_NegativeWindow covers the u<=0 guard: when end<start the pace
// unit is negative, so currentWindow must fall back to 1 instead of computing
// a nonsensical calendar position. Unreachable via the handler (fixtures.go
// rejects end<=start at creation), but the guard exists in currentWindow
// itself and must be verified in isolation.
func TestSlotFor_NegativeWindow(t *testing.T) {
	now := time.Now()
	st := &leveledState{
		target:   5,
		start:    now.Add(-time.Hour),
		end:      now.Add(-2 * time.Hour), // end < start → u < 0
		now:      now,
		occupied: map[string]map[int]bool{},
	}
	assert.Equal(t, 1, st.slotFor("p", "q"), "u<0 must fall back to slot 1")
}

// TestSlotFor_FillsHoles verifies a pair whose earlier Jornada is still open
// (e.g. it was the opponent side of another pair's slot-2 match, so it has no
// match at slot 1 yet) gets its hole filled first, rather than being pushed
// to slot 3 alongside a fully-booked opponent.
func TestSlotFor_FillsHoles(t *testing.T) {
	st := &leveledState{
		target: 5,
		occupied: map[string]map[int]bool{
			"p": {2: true}, // p already has a match at slot 2, slot 1 is a hole
			"q": {},        // q has no matches yet
		},
	}
	assert.Equal(t, 1, st.slotFor("p", "q"), "p's hole at slot 1 must be filled, not slot 3")
}

// TestSlotFor_NoCrossPairCollision verifies slotFor picks a slot free for
// BOTH pairs, not each pair's own smallest slot independently: p's smallest
// free slot (1) must not be used when q already occupies it, even though q's
// own smallest free slot (2) happens to be exactly the slot p occupies.
func TestSlotFor_NoCrossPairCollision(t *testing.T) {
	st := &leveledState{
		target: 5,
		occupied: map[string]map[int]bool{
			"p": {2: true}, // p's own smallest free slot is 1
			"q": {1: true}, // q's own smallest free slot is 2 — but slot 1 collides with q
		},
	}
	assert.Equal(t, 3, st.slotFor("p", "q"), "slot must be free for both pairs, not just the max of their own frees")
}

// TestSlotFor_CurrentWindowFloor verifies slotFor never assigns a slot before
// the competition's current calendar window, even when both pairs have no
// occupied slots yet (a mid-season top-up must not land in the past).
func TestSlotFor_CurrentWindowFloor(t *testing.T) {
	now := time.Now()
	st := &leveledState{
		target:   10,
		start:    now.Add(-50 * 24 * time.Hour),
		end:      now.Add(50 * 24 * time.Hour), // 100-day window, u=10d, now=start+50d → cur=6
		now:      now,
		occupied: map[string]map[int]bool{},
	}
	assert.Equal(t, 6, st.slotFor("p", "q"), "slot must floor to the current calendar window")
}

// TestSlotFor_CapAtTarget verifies slotFor never returns a slot past target,
// even when both pairs' free slots would otherwise land beyond it.
func TestSlotFor_CapAtTarget(t *testing.T) {
	st := &leveledState{
		target: 3,
		occupied: map[string]map[int]bool{
			"p": {1: true, 2: true, 3: true},
			"q": {1: true, 2: true, 3: true},
		},
	}
	assert.Equal(t, 3, st.slotFor("p", "q"), "slot must be capped at target even past a full house")
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
