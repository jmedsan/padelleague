package league

import (
	"testing"

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
