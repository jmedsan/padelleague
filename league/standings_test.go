package league

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeStandings_Basic(t *testing.T) {
	app := newTestApp(t)
	svc := New(app, nil)

	p1 := makePair(t, app, "Stand A")
	p2 := makePair(t, app, "Stand B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "final")
	m.Set("scores", "6-3 6-4")
	m.Set("winner", p1.Id)
	require.NoError(t, app.Save(m))

	rows, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	assert.Equal(t, p1.Id, rows[0].PairID)
	assert.Equal(t, 3, rows[0].Points)
	assert.Equal(t, 1, rows[0].Wins)
	assert.Equal(t, 0, rows[0].Losses)
	assert.Equal(t, 2, rows[0].SetsWon)
	assert.Equal(t, 0, rows[0].SetsLost)

	assert.Equal(t, p2.Id, rows[1].PairID)
	assert.Equal(t, 0, rows[1].Points)
	assert.Equal(t, 0, rows[1].Wins)
	assert.Equal(t, 1, rows[1].Losses)
}

func TestComputeStandings_Walkover(t *testing.T) {
	app := newTestApp(t)
	svc := New(app, nil)

	p1 := makePair(t, app, "WO A")
	p2 := makePair(t, app, "WO B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "final")
	m.Set("scores", "WO")
	m.Set("winner", p1.Id)
	require.NoError(t, app.Save(m))

	rows, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	assert.Equal(t, 1, rows[0].Wins)
	assert.Equal(t, 0, rows[0].SetsWon)
}

func TestComputeStandings_Penalties(t *testing.T) {
	app := newTestApp(t)
	svc := New(app, nil)

	p1 := makePair(t, app, "Pen A")
	p2 := makePair(t, app, "Pen B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "final")
	m.Set("scores", "6-3 6-4")
	m.Set("winner", p1.Id)
	require.NoError(t, app.Save(m))

	comp.Set("penalty_points", map[string]any{p1.Id: 2.0})
	require.NoError(t, app.Save(comp))

	rows, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	assert.Equal(t, 1, rows[0].Points)
	assert.Equal(t, 2, rows[0].Penalty)
}

func TestComputeStandings_Tiebreakers(t *testing.T) {
	app := newTestApp(t)
	svc := New(app, nil)

	p1 := makePair(t, app, "Tie A")
	p2 := makePair(t, app, "Tie B")
	p3 := makePair(t, app, "Tie C")
	comp := makeCompetition(t, app, []*core.Record{p1, p2, p3})

	// p1 beats p2
	m1 := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "final")
	m1.Set("scores", "6-3 6-4")
	m1.Set("winner", p1.Id)
	require.NoError(t, app.Save(m1))

	// p2 beats p3
	m2 := makeMatch(t, app, comp.Id, p2.Id, p3.Id, "final")
	m2.Set("scores", "6-3 6-4")
	m2.Set("winner", p2.Id)
	require.NoError(t, app.Save(m2))

	// p3 beats p1 (circular)
	m3 := makeMatch(t, app, comp.Id, p3.Id, p1.Id, "final")
	m3.Set("scores", "6-3 6-4")
	m3.Set("winner", p3.Id)
	require.NoError(t, app.Save(m3))

	rows, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	// All have 3 points, tiebreaker is set diff then game diff then H2H
	for _, r := range rows {
		assert.Equal(t, 3, r.Points)
	}
}

func TestComputeStandings_WalkoverPair2Wins(t *testing.T) {
	app := newTestApp(t)
	svc := New(app, nil)

	p1 := makePair(t, app, "WO2 A")
	p2 := makePair(t, app, "WO2 B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "final")
	m.Set("scores", "WO")
	m.Set("winner", p2.Id)
	require.NoError(t, app.Save(m))

	rows, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	// p2 wins the WO, should be first
	assert.Equal(t, p2.Id, rows[0].PairID)
	assert.Equal(t, 1, rows[0].Wins)
	assert.Equal(t, 0, rows[0].Losses)
	assert.Equal(t, p1.Id, rows[1].PairID)
	assert.Equal(t, 0, rows[1].Wins)
	assert.Equal(t, 1, rows[1].Losses)
}

func TestComputeStandings_PenaltyAsString(t *testing.T) {
	app := newTestApp(t)
	svc := New(app, nil)

	p1 := makePair(t, app, "PenS A")
	p2 := makePair(t, app, "PenS B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "final")
	m.Set("scores", "6-3 6-4")
	m.Set("winner", p1.Id)
	require.NoError(t, app.Save(m))

	// Set penalty_points as a raw JSON string via direct DB update
	_, err := app.DB().NewQuery(
		"UPDATE competitions SET penalty_points = {:pp} WHERE id = {:id}",
	).Bind(map[string]any{
		"pp": `{"` + p1.Id + `": 1}`,
		"id": comp.Id,
	}).Execute()
	require.NoError(t, err)

	rows, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	assert.Equal(t, 2, rows[0].Points) // 3 - 1 penalty
	assert.Equal(t, 1, rows[0].Penalty)
}

func TestComputeStandings_SetDiffTiebreaker(t *testing.T) {
	app := newTestApp(t)
	svc := New(app, nil)

	p1 := makePair(t, app, "SD A")
	p2 := makePair(t, app, "SD B")
	p3 := makePair(t, app, "SD C")
	comp := makeCompetition(t, app, []*core.Record{p1, p2, p3})

	// p1 beats p3 in 2 sets: set diff +2
	m1 := makeMatch(t, app, comp.Id, p1.Id, p3.Id, "final")
	m1.Set("scores", "6-3 6-4")
	m1.Set("winner", p1.Id)
	require.NoError(t, app.Save(m1))

	// p2 beats p3 in 3 sets: set diff +1
	m2 := makeMatch(t, app, comp.Id, p2.Id, p3.Id, "final")
	m2.Set("scores", "6-3 3-6 6-4")
	m2.Set("winner", p2.Id)
	require.NoError(t, app.Save(m2))

	rows, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	// Both p1 and p2 have 3 points, p1 has better set diff
	assert.Equal(t, p1.Id, rows[0].PairID)
	assert.Equal(t, p2.Id, rows[1].PairID)
}

func TestComputeStandings_GameDiffTiebreaker(t *testing.T) {
	app := newTestApp(t)
	svc := New(app, nil)

	p1 := makePair(t, app, "GD A")
	p2 := makePair(t, app, "GD B")
	p3 := makePair(t, app, "GD C")
	comp := makeCompetition(t, app, []*core.Record{p1, p2, p3})

	// p1 beats p3: 6-4 6-4 → set diff +2, game diff +4
	m1 := makeMatch(t, app, comp.Id, p1.Id, p3.Id, "final")
	m1.Set("scores", "6-4 6-4")
	m1.Set("winner", p1.Id)
	require.NoError(t, app.Save(m1))

	// p2 beats p3: 6-1 6-1 → set diff +2, game diff +10
	m2 := makeMatch(t, app, comp.Id, p2.Id, p3.Id, "final")
	m2.Set("scores", "6-1 6-1")
	m2.Set("winner", p2.Id)
	require.NoError(t, app.Save(m2))

	rows, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	// Same points (3), same set diff (+2), p2 has better game diff
	assert.Equal(t, p2.Id, rows[0].PairID)
	assert.Equal(t, p1.Id, rows[1].PairID)
}

func TestComputeStandings_InvalidCompetition(t *testing.T) {
	app := newTestApp(t)
	svc := New(app, nil)

	_, err := svc.ComputeStandings("nonexistent")
	assert.Error(t, err)
}

func TestComputeStandings_ThreeSets(t *testing.T) {
	app := newTestApp(t)
	svc := New(app, nil)

	p1 := makePair(t, app, "3S A")
	p2 := makePair(t, app, "3S B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "final")
	m.Set("scores", "6-3 3-6 7-5")
	m.Set("winner", p1.Id)
	require.NoError(t, app.Save(m))

	rows, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	assert.Equal(t, 2, rows[0].SetsWon)
	assert.Equal(t, 1, rows[0].SetsLost)
	assert.Equal(t, 16, rows[0].GamesWon)
	assert.Equal(t, 14, rows[0].GamesLost)
}
