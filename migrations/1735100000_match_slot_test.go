package migrations

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var slotTestSeq atomic.Int64

func newSlotTestApp(t *testing.T) core.App {
	t.Helper()
	app, err := tests.NewTestApp()
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)
	return app
}

func makeSlotTestPair(t *testing.T, app core.App, name string) *core.Record {
	t.Helper()
	n := slotTestSeq.Add(1)
	usersCol, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	u1 := core.NewRecord(usersCol)
	u1.Set("email", fmt.Sprintf("slotp%dp1@test.local", n))
	u1.Set("username", fmt.Sprintf("slotu%dp1", n))
	u1.Set("display_name", name+" P1")
	u1.Set("roles", []string{"player"})
	u1.SetPassword("testpass123456")
	u1.SetVerified(true)
	require.NoError(t, app.Save(u1))

	u2 := core.NewRecord(usersCol)
	u2.Set("email", fmt.Sprintf("slotp%dp2@test.local", n))
	u2.Set("username", fmt.Sprintf("slotu%dp2", n))
	u2.Set("display_name", name+" P2")
	u2.Set("roles", []string{"player"})
	u2.SetPassword("testpass123456")
	u2.SetVerified(true)
	require.NoError(t, app.Save(u2))

	pairsCol, err := app.FindCollectionByNameOrId("pairs")
	require.NoError(t, err)
	pair := core.NewRecord(pairsCol)
	pair.Set("name", name)
	pair.Set("player1", u1.Id)
	pair.Set("player2", u2.Id)
	require.NoError(t, app.Save(pair))
	return pair
}

func makeSlotTestCompetition(t *testing.T, app core.App, pairs []*core.Record, target int) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("competitions")
	require.NoError(t, err)
	rec := core.NewRecord(col)
	rec.Set("name", "Slot Backfill Test")
	rec.Set("type", "league")
	rec.Set("target_matches", target)
	rec.Set("open_assignments", 2)
	ids := make([]string, len(pairs))
	for i, p := range pairs {
		ids[i] = p.Id
	}
	rec.Set("pairs", ids)
	require.NoError(t, app.Save(rec))
	return rec
}

func makeSlotTestMatch(t *testing.T, app core.App, compID, p1, p2 string, created time.Time) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("matches")
	require.NoError(t, err)
	m := core.NewRecord(col)
	m.Set("competition", compID)
	m.Set("pair1", p1)
	m.Set("pair2", p2)
	m.Set("status", "pending")
	m.Set("round_number", 0)
	require.NoError(t, app.Save(m))
	// created is an autodate field PocketBase stamps on Save; overwrite it
	// directly at the DB level via a second save with the field forced.
	m.SetRaw("created", created.UTC().Format("2006-01-02 15:04:05.000Z"))
	require.NoError(t, app.Save(m))
	return m
}

// TestBackfillMatchSlots verifies the migration's backfill assigns per-pair
// ordinal slots in created-ascending order, matching the live plan() formula
// (slot = max(counter_p1, counter_p2) + 1).
func TestBackfillMatchSlots(t *testing.T) {
	t.Parallel()
	app := newSlotTestApp(t)

	pa := makeSlotTestPair(t, app, "BackfillA")
	pb := makeSlotTestPair(t, app, "BackfillB")
	pc := makeSlotTestPair(t, app, "BackfillC")
	// target=1 < 3 pairs - 1 = 2 → IsLeveled=true
	comp := makeSlotTestCompetition(t, app, []*core.Record{pa, pb, pc}, 1)

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	m1 := makeSlotTestMatch(t, app, comp.Id, pa.Id, pb.Id, base)                  // slot 1 for both
	m2 := makeSlotTestMatch(t, app, comp.Id, pa.Id, pc.Id, base.Add(time.Hour))   // pa counter=1 → slot 2
	m3 := makeSlotTestMatch(t, app, comp.Id, pb.Id, pc.Id, base.Add(2*time.Hour)) // pb=1,pc=2 → slot 3

	require.NoError(t, backfillMatchSlots(app))

	got1, err := app.FindRecordById("matches", m1.Id)
	require.NoError(t, err)
	got2, err := app.FindRecordById("matches", m2.Id)
	require.NoError(t, err)
	got3, err := app.FindRecordById("matches", m3.Id)
	require.NoError(t, err)

	assert.Equal(t, 1, got1.GetInt("slot"), "first match (created-ascending) gets slot 1")
	assert.Equal(t, 2, got2.GetInt("slot"), "pa's 2nd match: max(counter[pa]=1, counter[pc]=0)+1=2")
	assert.Equal(t, 3, got3.GetInt("slot"), "pb's 2nd match: max(counter[pb]=1, counter[pc]=2)+1=3")
}

// TestBackfillMatchSlots_RecomputesArrangeByForNonFinal verifies the backfill
// sets arrange_by from the slot formula for pending matches, and leaves it
// untouched for final matches, when the competition has dates.
func TestBackfillMatchSlots_RecomputesArrangeByForNonFinal(t *testing.T) {
	t.Parallel()
	app := newSlotTestApp(t)

	pairs := make([]*core.Record, 12)
	for i := range pairs {
		pairs[i] = makeSlotTestPair(t, app, fmt.Sprintf("BackfillDate%d", i))
	}
	pa, pb, pc := pairs[0], pairs[1], pairs[2]
	comp := makeSlotTestCompetition(t, app, pairs, 10) // 10 < 12-1=11 → IsLeveled=true
	comp.Set("start_date", "2026-01-01T00:00:00Z")
	comp.Set("end_date", "2026-04-11T00:00:00Z") // 100-day window, u=10d
	require.NoError(t, app.Save(comp))

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	mPending := makeSlotTestMatch(t, app, comp.Id, pa.Id, pb.Id, base)
	mFinal := makeSlotTestMatch(t, app, comp.Id, pa.Id, pc.Id, base.Add(time.Hour))
	mFinal.Set("status", "final")
	require.NoError(t, app.Save(mFinal))

	require.NoError(t, backfillMatchSlots(app))

	gotPending, err := app.FindRecordById("matches", mPending.Id)
	require.NoError(t, err)
	assert.Equal(t, 1, gotPending.GetInt("slot"))
	assert.NotEmpty(t, gotPending.GetString("arrange_by"), "non-final match must get a computed arrange_by")

	gotFinal, err := app.FindRecordById("matches", mFinal.Id)
	require.NoError(t, err)
	assert.Equal(t, 2, gotFinal.GetInt("slot"), "final match still gets a slot for ordinal bookkeeping")
	assert.Empty(t, gotFinal.GetString("arrange_by"), "final match arrange_by must not be set by backfill")
}

// TestIsLeveledComp verifies the migration's leveled-competition detector
// matches league.IsLeveled's formula.
func TestIsLeveledComp(t *testing.T) {
	t.Parallel()
	app := newSlotTestApp(t)

	pairs := make([]*core.Record, 4)
	for i := range pairs {
		pairs[i] = makeSlotTestPair(t, app, fmt.Sprintf("IsLvl%d", i))
	}

	leveled := makeSlotTestCompetition(t, app, pairs, 2) // 2 < 4-1=3 → leveled
	assert.True(t, isLeveledComp(leveled))

	roundRobin := makeSlotTestCompetition(t, app, pairs, 0)
	assert.False(t, isLeveledComp(roundRobin))

	tooHigh := makeSlotTestCompetition(t, app, pairs, 3) // 3 < 4-1=3 is false
	assert.False(t, isLeveledComp(tooHigh))
}
