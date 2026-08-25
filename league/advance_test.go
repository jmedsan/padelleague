package league

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makePlayoffCompetition(t *testing.T, app core.App, pairs []*core.Record) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("competitions")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Set("name", "Playoff")
	record.Set("type", "playoff")
	record.Set("active", true)
	pairIDs := make([]string, len(pairs))
	for i, p := range pairs {
		pairIDs[i] = p.Id
	}
	record.Set("pairs", pairIDs)
	require.NoError(t, app.Save(record))
	return record
}

func makeMatchRound(t *testing.T, app core.App, compID, p1ID, p2ID string, round int) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("matches")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Set("competition", compID)
	record.Set("pair1", p1ID)
	record.Set("pair2", p2ID)
	record.Set("status", "pending")
	record.Set("round_number", round)
	require.NoError(t, app.Save(record))
	return record
}

func TestAdvancePlayoff_AdvancesWinners(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	p1 := makePair(t, app, "PO A")
	p2 := makePair(t, app, "PO B")
	p3 := makePair(t, app, "PO C")
	p4 := makePair(t, app, "PO D")
	comp := makePlayoffCompetition(t, app, []*core.Record{p1, p2, p3, p4})

	// Round 1: two matches
	m1 := makeMatchRound(t, app, comp.Id, p1.Id, p2.Id, 1)
	m1.Set("status", "final")
	m1.Set("scores", "6-3 6-4")
	m1.Set("winner", p1.Id)
	require.NoError(t, app.Save(m1))

	m2 := makeMatchRound(t, app, comp.Id, p3.Id, p4.Id, 1)
	m2.Set("status", "final")
	m2.Set("scores", "6-3 6-4")
	m2.Set("winner", p4.Id)
	require.NoError(t, app.Save(m2))

	// Round 2: final (empty pair slots)
	col, err := app.FindCollectionByNameOrId("matches")
	require.NoError(t, err)
	final := core.NewRecord(col)
	final.Set("competition", comp.Id)
	final.Set("status", "pending")
	final.Set("round_number", 2)
	require.NoError(t, app.Save(final))

	advErr := svc.AdvancePlayoff(m2)
	require.NoError(t, advErr)

	updated, updErr := app.FindRecordById("matches", final.Id)
	require.NoError(t, updErr)
	assert.Equal(t, p1.Id, updated.GetString("pair1"))
	assert.Equal(t, p4.Id, updated.GetString("pair2"))
}

func TestAdvancePlayoff_NotAllFinished(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	p1 := makePair(t, app, "POInc A")
	p2 := makePair(t, app, "POInc B")
	p3 := makePair(t, app, "POInc C")
	p4 := makePair(t, app, "POInc D")
	comp := makePlayoffCompetition(t, app, []*core.Record{p1, p2, p3, p4})

	m1 := makeMatchRound(t, app, comp.Id, p1.Id, p2.Id, 1)
	m1.Set("status", "final")
	m1.Set("scores", "6-3 6-4")
	m1.Set("winner", p1.Id)
	require.NoError(t, app.Save(m1))

	// m2 still pending
	makeMatchRound(t, app, comp.Id, p3.Id, p4.Id, 1)

	col, _ := app.FindCollectionByNameOrId("matches")
	r2 := core.NewRecord(col)
	r2.Set("competition", comp.Id)
	r2.Set("status", "pending")
	r2.Set("round_number", 2)
	require.NoError(t, app.Save(r2))

	err := svc.AdvancePlayoff(m1)
	require.NoError(t, err)
	// No advancement should happen
}

func TestAdvancePlayoff_LeagueIgnored(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	p1 := makePair(t, app, "Lg A")
	p2 := makePair(t, app, "Lg B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "final")
	m.Set("scores", "6-3 6-4")
	m.Set("winner", p1.Id)
	require.NoError(t, app.Save(m))

	err := svc.AdvancePlayoff(m)
	require.NoError(t, err)
}

func TestAdvancePlayoff_NoNextRound(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	p1 := makePair(t, app, "POEnd A")
	p2 := makePair(t, app, "POEnd B")
	comp := makePlayoffCompetition(t, app, []*core.Record{p1, p2})

	m := makeMatchRound(t, app, comp.Id, p1.Id, p2.Id, 1)
	m.Set("status", "final")
	m.Set("scores", "6-3 6-4")
	m.Set("winner", p1.Id)
	require.NoError(t, app.Save(m))

	// No round 2 matches exist
	err := svc.AdvancePlayoff(m)
	require.NoError(t, err)
}

// A quarter-final round feeding two semi-finals: winner 0 and 1 fill the
// first semi, winners 2 and 3 the second. Indexing the winners by anything
// other than i*2 / i*2+1 puts the wrong pair through.
func TestAdvancePlayoff_PairsWinnersTwoPerNextMatch(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	pairs := make([]*core.Record, 8)
	for i := range pairs {
		pairs[i] = makePair(t, app, "QF "+string(rune('A'+i)))
	}
	comp := makePlayoffCompetition(t, app, pairs)

	// Round 1: four matches. The first pair of each wins.
	winners := []*core.Record{pairs[0], pairs[2], pairs[4], pairs[6]}
	for i := 0; i < 4; i++ {
		m := makeMatchRound(t, app, comp.Id, pairs[i*2].Id, pairs[i*2+1].Id, 1)
		m.Set("status", "final")
		m.Set("scores", "6-3 6-4")
		m.Set("winner", winners[i].Id)
		require.NoError(t, app.Save(m))
	}
	// Round 2: two empty semi-finals waiting to be filled.
	semi1 := makeMatchRound(t, app, comp.Id, "", "", 2)
	semi2 := makeMatchRound(t, app, comp.Id, "", "", 2)

	require.NoError(t, svc.AdvancePlayoff(mustMatch(t, app, comp.Id, 1)))

	got1, err := app.FindRecordById("matches", semi1.Id)
	require.NoError(t, err)
	got2, err := app.FindRecordById("matches", semi2.Id)
	require.NoError(t, err)

	assert.Equal(t, winners[0].Id, got1.GetString("pair1"))
	assert.Equal(t, winners[1].Id, got1.GetString("pair2"))
	assert.Equal(t, winners[2].Id, got2.GetString("pair1"))
	assert.Equal(t, winners[3].Id, got2.GetString("pair2"))
}

// Three winners feeding two next matches: the second slot of the second match
// has no winner to take, so the bounds check must stop it rather than read
// past the end of the winners slice.
func TestAdvancePlayoff_StopsAtEndOfWinners(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	pairs := make([]*core.Record, 6)
	for i := range pairs {
		pairs[i] = makePair(t, app, "OD "+string(rune('A'+i)))
	}
	comp := makePlayoffCompetition(t, app, pairs)

	winners := []*core.Record{pairs[0], pairs[2], pairs[4]}
	for i := 0; i < 3; i++ {
		m := makeMatchRound(t, app, comp.Id, pairs[i*2].Id, pairs[i*2+1].Id, 1)
		m.Set("status", "final")
		m.Set("scores", "6-3 6-4")
		m.Set("winner", winners[i].Id)
		require.NoError(t, app.Save(m))
	}
	next1 := makeMatchRound(t, app, comp.Id, "", "", 2)
	next2 := makeMatchRound(t, app, comp.Id, "", "", 2)

	require.NoError(t, svc.AdvancePlayoff(mustMatch(t, app, comp.Id, 1)))

	got1, err := app.FindRecordById("matches", next1.Id)
	require.NoError(t, err)
	got2, err := app.FindRecordById("matches", next2.Id)
	require.NoError(t, err)

	assert.Equal(t, winners[0].Id, got1.GetString("pair1"))
	assert.Equal(t, winners[1].Id, got1.GetString("pair2"))
	assert.Equal(t, winners[2].Id, got2.GetString("pair1"))
	assert.Empty(t, got2.GetString("pair2"), "no fourth winner exists to fill this slot")
}

// mustMatch returns any match of the competition in the given round.
func mustMatch(t *testing.T, app core.App, compID string, round int) *core.Record {
	t.Helper()
	recs, err := app.FindRecordsByFilter("matches",
		"competition = {:cid} && round_number = {:rn}", "", 1, 0,
		map[string]any{"cid": compID, "rn": round})
	require.NoError(t, err)
	require.NotEmpty(t, recs)
	return recs[0]
}

// Two winners but two next matches: the second one has no winners left at
// all. p1Idx lands exactly on len(roundWinners), so the bounds check is the
// only thing stopping a read past the end.
func TestAdvancePlayoff_SecondNextMatchHasNoWinnersLeft(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	pairs := make([]*core.Record, 4)
	for i := range pairs {
		pairs[i] = makePair(t, app, "EX "+string(rune('A'+i)))
	}
	comp := makePlayoffCompetition(t, app, pairs)

	winners := []*core.Record{pairs[0], pairs[2]}
	for i := 0; i < 2; i++ {
		m := makeMatchRound(t, app, comp.Id, pairs[i*2].Id, pairs[i*2+1].Id, 1)
		m.Set("status", "final")
		m.Set("scores", "6-3 6-4")
		m.Set("winner", winners[i].Id)
		require.NoError(t, app.Save(m))
	}
	next1 := makeMatchRound(t, app, comp.Id, "", "", 2)
	next2 := makeMatchRound(t, app, comp.Id, "", "", 2)

	require.NoError(t, svc.AdvancePlayoff(mustMatch(t, app, comp.Id, 1)))

	got1, err := app.FindRecordById("matches", next1.Id)
	require.NoError(t, err)
	got2, err := app.FindRecordById("matches", next2.Id)
	require.NoError(t, err)

	assert.Equal(t, winners[0].Id, got1.GetString("pair1"))
	assert.Equal(t, winners[1].Id, got1.GetString("pair2"))
	assert.Empty(t, got2.GetString("pair1"), "no winners remain for the second match")
	assert.Empty(t, got2.GetString("pair2"))
}

// Matches finalized in reverse creation order must still map winners to the
// correct next-round slots. Before the "created" sort fix, the query could
// return matches ordered by last-update time, swapping winners across slots.
func TestAdvancePlayoff_OutOfOrderFinalization(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	pairs := make([]*core.Record, 8)
	for i := range pairs {
		pairs[i] = makePair(t, app, "OOF "+string(rune('A'+i)))
	}
	comp := makePlayoffCompetition(t, app, pairs)

	// Create round-1 matches in slot order (0-3).
	r1 := make([]*core.Record, 4)
	for i := 0; i < 4; i++ {
		r1[i] = makeMatchRound(t, app, comp.Id, pairs[i*2].Id, pairs[i*2+1].Id, 1)
	}

	// Create round-2 matches (semis) in slot order.
	semi1 := makeMatchRound(t, app, comp.Id, "", "", 2)
	semi2 := makeMatchRound(t, app, comp.Id, "", "", 2)

	// Finalize in REVERSE creation order: match 3 first, then 2, 1, 0.
	// Each gets a different submitted_at so any "submitted_at" sort would
	// return them in the wrong order.
	winners := []*core.Record{pairs[0], pairs[2], pairs[4], pairs[6]}
	for _, idx := range []int{3, 2, 1, 0} {
		m, err := app.FindRecordById("matches", r1[idx].Id)
		require.NoError(t, err)
		m.Set("status", "final")
		m.Set("scores", "6-3 6-4")
		m.Set("winner", winners[idx].Id)
		m.Set("submitted_at", time.Now().UTC().Format(time.RFC3339))
		require.NoError(t, app.Save(m))
		time.Sleep(10 * time.Millisecond)
	}

	require.NoError(t, svc.AdvancePlayoff(r1[0]))

	got1, err := app.FindRecordById("matches", semi1.Id)
	require.NoError(t, err)
	got2, err := app.FindRecordById("matches", semi2.Id)
	require.NoError(t, err)

	assert.Equal(t, winners[0].Id, got1.GetString("pair1"), "semi1 pair1 = winner of match 0")
	assert.Equal(t, winners[1].Id, got1.GetString("pair2"), "semi1 pair2 = winner of match 1")
	assert.Equal(t, winners[2].Id, got2.GetString("pair1"), "semi2 pair1 = winner of match 2")
	assert.Equal(t, winners[3].Id, got2.GetString("pair2"), "semi2 pair2 = winner of match 3")
}

// Re-calling AdvancePlayoff after the next round has already left pending
// must NOT overwrite its pair slots. This guards against admin overrides on
// already-final matches re-seeding a match that is in progress or played.
func TestAdvancePlayoff_SkipsNonPendingNextRound(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	p1 := makePair(t, app, "Guard A")
	p2 := makePair(t, app, "Guard B")
	p3 := makePair(t, app, "Guard C")
	p4 := makePair(t, app, "Guard D")
	comp := makePlayoffCompetition(t, app, []*core.Record{p1, p2, p3, p4})

	// Round 1: two matches, both final.
	m1 := makeMatchRound(t, app, comp.Id, p1.Id, p2.Id, 1)
	m1.Set("status", "final")
	m1.Set("scores", "6-3 6-4")
	m1.Set("winner", p1.Id)
	require.NoError(t, app.Save(m1))

	m2 := makeMatchRound(t, app, comp.Id, p3.Id, p4.Id, 1)
	m2.Set("status", "final")
	m2.Set("scores", "6-3 6-4")
	m2.Set("winner", p3.Id)
	require.NoError(t, app.Save(m2))

	// Round 2: final match, initially pending.
	finalMatch := makeMatchRound(t, app, comp.Id, "", "", 2)

	// First advance seeds the final correctly.
	require.NoError(t, svc.AdvancePlayoff(m2))
	seeded, err := app.FindRecordById("matches", finalMatch.Id)
	require.NoError(t, err)
	assert.Equal(t, p1.Id, seeded.GetString("pair1"))
	assert.Equal(t, p3.Id, seeded.GetString("pair2"))

	// Simulate: the final match has advanced past pending (score submitted).
	seeded.Set("status", StatusConfirmed)
	seeded.Set("scores", "6-2 6-1")
	require.NoError(t, app.Save(seeded))

	// Admin overrides round 1 match, swapping the winner to p2.
	m1fresh, err := app.FindRecordById("matches", m1.Id)
	require.NoError(t, err)
	m1fresh.Set("winner", p2.Id)
	require.NoError(t, app.Save(m1fresh))

	// AdvancePlayoff re-fires — must NOT overwrite the confirmed final.
	require.NoError(t, svc.AdvancePlayoff(m1fresh))

	after, err := app.FindRecordById("matches", finalMatch.Id)
	require.NoError(t, err)
	assert.Equal(t, p1.Id, after.GetString("pair1"), "pair1 must not be overwritten")
	assert.Equal(t, p3.Id, after.GetString("pair2"), "pair2 must not be overwritten")
	assert.Equal(t, StatusConfirmed, after.GetString("status"), "status must remain confirmed")
}
