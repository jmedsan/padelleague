package league

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReleasedPairings_RoundTrip(t *testing.T) {
	app := newTestApp(t)
	pa := makePair(t, app, "Rel A")
	pb := makePair(t, app, "Rel B")
	comp := makeLeveledCompetition(t, app, []*core.Record{pa, pb}, 2, 2)

	assert.Empty(t, ReleasedPairings(comp), "no releases yet")

	require.NoError(t, AppendReleasedPairing(app, comp, pa.Id, pb.Id))

	got := ReleasedPairings(comp)
	require.Len(t, got, 1)
	assert.ElementsMatch(t, []string{pa.Id, pb.Id}, []string{got[0].A, got[0].B})

	// Appending the same unordered pair again must not duplicate the entry.
	require.NoError(t, AppendReleasedPairing(app, comp, pb.Id, pa.Id))
	assert.Len(t, ReleasedPairings(comp), 1, "reverse-order append is a no-op")
}

// TestTopUp_NeverRecreatesReleasedPairing pins the oracle's finding: a
// released pairing must stay avoided across separate TopUpAssignments runs,
// not just the one immediately following the release. AdminRelease's avoid
// argument (hooks/hooks.go's delete hook) only reaches the run it's passed
// to; a later run — e.g. the daily cron, which calls TopUpAssignments with
// no avoid at all — has nothing but the persisted released_pairings record
// to stop it from re-creating the exact match the admin just released.
//
// 4 pairs, target=1 (each plays exactly one opponent): pc-pd's single match
// is already final, so they're fully done and cannot receive pa or pb.
// After pa-pb is released, they are each other's only remaining opponent —
// the tightest case where a missing block reliably recreates the pairing.
func TestTopUp_NeverRecreatesReleasedPairing(t *testing.T) {
	app := newTestApp(t)
	pa := makePair(t, app, "NoRe A")
	pb := makePair(t, app, "NoRe B")
	pc := makePair(t, app, "NoRe C")
	pd := makePair(t, app, "NoRe D")
	pairs := []*core.Record{pa, pb, pc, pd}

	comp := makeLeveledCompetition(t, app, pairs, 1, 1)
	svc := newDeterministicSvc(app)
	now := time.Now()

	makeLeveledMatch(t, app, comp.Id, pc.Id, pd.Id, "6-0 6-0", pc.Id, "final", now)

	// Simulate AdminRelease: a pa-pb match existed, gets deleted, and the
	// release is persisted.
	papb := makeLeveledMatch(t, app, comp.Id, pa.Id, pb.Id, "", "", "pending", time.Time{})
	require.NoError(t, app.Delete(papb))
	require.NoError(t, AppendReleasedPairing(app, comp, pa.Id, pb.Id))

	assertNeverPaired := func(t *testing.T, created []*core.Record) {
		t.Helper()
		for _, m := range created {
			p1, p2 := m.GetString("pair1"), m.GetString("pair2")
			isPaPb := (p1 == pa.Id && p2 == pb.Id) || (p1 == pb.Id && p2 == pa.Id)
			assert.False(t, isPaPb, "released pairing must never be recreated")
		}
	}

	// Immediate run, as the delete hook fires it: with an explicit avoid,
	// and no other opponent available, this correctly creates nothing.
	immediate, err := svc.TopUpAssignments(comp.Id, now, Pairing{A: pa.Id, B: pb.Id})
	require.NoError(t, err)
	assertNeverPaired(t, immediate)
	assert.Empty(t, immediate, "pc/pd are already done — nothing should be assignable yet")

	// Cron-style run: no avoid argument at all. Only the persisted
	// released_pairings record (read into met by buildLeveledState) can
	// stop this one from re-pairing pa and pb — without it, they are each
	// other's only remaining opponent and get recreated.
	cronRun, err := svc.TopUpAssignments(comp.Id, now)
	require.NoError(t, err)
	assertNeverPaired(t, cronRun)
}
