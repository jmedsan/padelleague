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
