package league

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeLeveledMatch creates a match with round_number=0, matches_to_win=1.
func makeLeveledMatch(t *testing.T, app core.App, compID, p1, p2, scores, winner, status string, finalizedAt time.Time) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("matches")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Set("competition", compID)
	record.Set("pair1", p1)
	record.Set("pair2", p2)
	record.Set("round_number", 0)
	record.Set("matches_to_win", 1)
	record.Set("scores", scores)
	record.Set("winner", winner)
	record.Set("status", status)
	if !finalizedAt.IsZero() {
		dt, err := types.ParseDateTime(finalizedAt)
		require.NoError(t, err)
		record.SetRaw("finalized_at", dt)
	}
	require.NoError(t, app.Save(record))
	return record
}

func TestRatings_SeedOnly(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	pa := makePair(t, app, "Alpha")
	pb := makePair(t, app, "Beta")
	pc := makePair(t, app, "Gamma")
	pd := makePair(t, app, "Delta")
	pe := makePair(t, app, "Epsilon") // unseeded 5th pair

	// Seed order: pa(0), pb(1), pc(2), pd(3) — pe unseeded.
	comp := makeCompetition(t, app, []*core.Record{pa, pb, pc, pd, pe})
	comp.Set("seed_pairs", []string{pa.Id, pb.Id, pc.Id, pd.Id})
	require.NoError(t, app.Save(comp))

	ratings, err := Ratings(app, comp)
	require.NoError(t, err)

	// n=4 seeded pairs; index i gets ((4-1)/2 - i)*40
	// i=0 → 1.5*40 = +60; i=1 → 0.5*40 = +20; i=2 → -0.5*40 = -20; i=3 → -1.5*40 = -60
	assert.InDelta(t, 60.0, ratings[pa.Id], 0.001)
	assert.InDelta(t, 20.0, ratings[pb.Id], 0.001)
	assert.InDelta(t, -20.0, ratings[pc.Id], 0.001)
	assert.InDelta(t, -60.0, ratings[pd.Id], 0.001)
	assert.InDelta(t, 0.0, ratings[pe.Id], 0.001)
}

func TestRatings_SeedOnly_NoSeed(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	pa := makePair(t, app, "A")
	pb := makePair(t, app, "B")

	comp := makeCompetition(t, app, []*core.Record{pa, pb})
	// no seed_pairs set

	ratings, err := Ratings(app, comp)
	require.NoError(t, err)

	assert.InDelta(t, 0.0, ratings[pa.Id], 0.001)
	assert.InDelta(t, 0.0, ratings[pb.Id], 0.001)
}

func TestRatings_OneMatch(t *testing.T) {
	t.Parallel()

	matchTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	t.Run("2-0 win", func(t *testing.T) {
		app := newTestApp(t)
		pa := makePair(t, app, "One2")
		pb := makePair(t, app, "Two2")
		comp := makeCompetition(t, app, []*core.Record{pa, pb})

		makeLeveledMatch(t, app, comp.Id, pa.Id, pb.Id, "6-0 6-0", pa.Id, "final", matchTime)

		ratings, err := Ratings(app, comp)
		require.NoError(t, err)

		// K=200, s_A=1, E_A=0.5: delta = 200*(1-0.5) = +100
		assert.InDelta(t, 100.0, ratings[pa.Id], 0.5)
		assert.InDelta(t, -100.0, ratings[pb.Id], 0.5)
	})

	t.Run("2-1 win", func(t *testing.T) {
		app := newTestApp(t)
		pa := makePair(t, app, "One3")
		pb := makePair(t, app, "Two3")
		comp := makeCompetition(t, app, []*core.Record{pa, pb})

		makeLeveledMatch(t, app, comp.Id, pa.Id, pb.Id, "6-0 4-6 6-0", pa.Id, "final", matchTime)

		ratings, err := Ratings(app, comp)
		require.NoError(t, err)

		// K=200, s_A=2/3, E_A=0.5: delta = 200*(2/3-0.5) = 200/6 ≈ 33.33
		assert.InDelta(t, 33.3, ratings[pa.Id], 0.5)
		assert.InDelta(t, -33.3, ratings[pb.Id], 0.5)
	})
}

func TestRatings_KSchedule(t *testing.T) {
	t.Parallel()

	// K formula: max(40, 200 * 4 / (4 + n))
	cases := []struct {
		n     int
		wantK float64
	}{
		{0, 200.0},
		{1, 200.0 * 4 / 5},
		{4, 100.0},
		{16, 40.0},
		{100, 40.0},
	}
	for _, tc := range cases {
		got := kFactor(tc.n)
		assert.InDelta(t, tc.wantK, got, 0.001, "n=%d", tc.n)
	}
}

func TestRatings_ReplayOrder(t *testing.T) {
	t.Parallel()

	// Use 3 pairs: pa beats pb at t1, then pb beats pc at t2.
	// vs. reversed: pb beats pc at t1 (changing pb's rating first), then pa beats pb at t2.
	// With 3 pairs, the order of the two matches changes pa's rating because pb's
	// rating when pa plays it differs between the two orders.
	t1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)

	// Order 1: pa beats pb at t1, pb beats pc at t2.
	app := newTestApp(t)
	pa := makePair(t, app, "PA")
	pb := makePair(t, app, "PB")
	pc := makePair(t, app, "PC")
	comp := makeCompetition(t, app, []*core.Record{pa, pb, pc})
	makeLeveledMatch(t, app, comp.Id, pa.Id, pb.Id, "6-0 6-0", pa.Id, "final", t1)
	makeLeveledMatch(t, app, comp.Id, pb.Id, pc.Id, "6-0 6-0", pb.Id, "final", t2)
	r1, err := Ratings(app, comp)
	require.NoError(t, err)

	// Order 2: pb beats pc at t1, pa beats pb at t2.
	app2 := newTestApp(t)
	pa2 := makePair(t, app2, "PA2")
	pb2 := makePair(t, app2, "PB2")
	pc2 := makePair(t, app2, "PC2")
	comp2 := makeCompetition(t, app2, []*core.Record{pa2, pb2, pc2})
	makeLeveledMatch(t, app2, comp2.Id, pb2.Id, pc2.Id, "6-0 6-0", pb2.Id, "final", t1)
	makeLeveledMatch(t, app2, comp2.Id, pa2.Id, pb2.Id, "6-0 6-0", pa2.Id, "final", t2)
	r2, err := Ratings(app2, comp2)
	require.NoError(t, err)

	// The different ordering of pb's matches should produce different pa rating.
	// In order 1: pa plays pb when pb is at 0 → pa gains 100.
	// In order 2: pa plays pb when pb is already at +100 → E_A < 0.5 → pa gains less.
	assert.NotEqual(t, r1[pa.Id], r2[pa2.Id], "replay order must affect ratings")

	// Same order on two calls → same result (P8)
	r3, err := Ratings(app, comp)
	require.NoError(t, err)
	assert.Equal(t, r1[pa.Id], r3[pa.Id])
	assert.Equal(t, r1[pb.Id], r3[pb.Id])
}

func TestRatings_SkipsWalkovers(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	pa := makePair(t, app, "WalkA")
	pb := makePair(t, app, "WalkB")
	comp := makeCompetition(t, app, []*core.Record{pa, pb})

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	// WO score
	makeLeveledMatch(t, app, comp.Id, pa.Id, pb.Id, "WO", pa.Id, "final", now)

	ratings, err := Ratings(app, comp)
	require.NoError(t, err)

	// Walkover must not change ratings from zero
	assert.InDelta(t, 0.0, ratings[pa.Id], 0.001)
	assert.InDelta(t, 0.0, ratings[pb.Id], 0.001)
}
