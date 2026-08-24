package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/league"
)

func TestComputeStandings_Points(t *testing.T) {
	app := newTestApp(t)

	p1 := makePair(t, app, "Pair A")
	p2 := makePair(t, app, "Pair B")
	comp := makeCompetition(t, app, nil)
	comp.Set("pairs", []string{p1.Id, p2.Id})
	require.NoError(t, app.Save(comp))

	makeFinalMatch(t, app, comp.Id, p1.Id, p2.Id, "6-3 6-4", p1.Id)

	svc := league.New(app, nil)
	rows, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	assert.Equal(t, p1.Id, rows[0].PairID)
	assert.Equal(t, 3, rows[0].Points)
	assert.Equal(t, 1, rows[0].Wins)
	assert.Equal(t, 0, rows[0].Losses)

	assert.Equal(t, p2.Id, rows[1].PairID)
	assert.Equal(t, 0, rows[1].Points)
	assert.Equal(t, 0, rows[1].Wins)
	assert.Equal(t, 1, rows[1].Losses)
}

func TestComputeStandings_SetDiffTiebreaker(t *testing.T) {
	app := newTestApp(t)

	p1 := makePair(t, app, "Pair A")
	p2 := makePair(t, app, "Pair B")
	p3 := makePair(t, app, "Pair C")
	comp := makeCompetition(t, app, nil)
	comp.Set("pairs", []string{p1.Id, p2.Id, p3.Id})
	require.NoError(t, app.Save(comp))

	makeFinalMatch(t, app, comp.Id, p1.Id, p3.Id, "6-1 6-1", p1.Id)
	makeFinalMatch(t, app, comp.Id, p2.Id, p3.Id, "7-5 7-5", p2.Id)

	svc := league.New(app, nil)
	rows, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	require.Len(t, rows, 3)

	assert.Equal(t, p1.Id, rows[0].PairID)
	assert.Equal(t, p2.Id, rows[1].PairID)
	assert.Equal(t, p3.Id, rows[2].PairID)
}

func TestComputeStandings_WO(t *testing.T) {
	app := newTestApp(t)

	p1 := makePair(t, app, "Pair A")
	p2 := makePair(t, app, "Pair B")
	comp := makeCompetition(t, app, nil)
	comp.Set("pairs", []string{p1.Id, p2.Id})
	require.NoError(t, app.Save(comp))

	makeFinalMatch(t, app, comp.Id, p1.Id, p2.Id, "WO", p1.Id)

	svc := league.New(app, nil)
	rows, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	assert.Equal(t, p1.Id, rows[0].PairID)
	assert.Equal(t, 3, rows[0].Points)
	assert.Equal(t, 1, rows[0].Wins)
	assert.Equal(t, 0, rows[0].SetsWon)
	assert.Equal(t, 0, rows[0].GamesWon)

	assert.Equal(t, p2.Id, rows[1].PairID)
	assert.Equal(t, 0, rows[1].Points)
	assert.Equal(t, 1, rows[1].Losses)
}

func TestComputeStandings_Penalty(t *testing.T) {
	app := newTestApp(t)

	p1 := makePair(t, app, "Pair A")
	p2 := makePair(t, app, "Pair B")
	comp := makeCompetition(t, app, nil)
	comp.Set("pairs", []string{p1.Id, p2.Id})
	require.NoError(t, app.Save(comp))

	makeFinalMatch(t, app, comp.Id, p1.Id, p2.Id, "6-3 6-4", p1.Id)

	comp.Set("penalty_points", map[string]any{p1.Id: float64(3)})
	require.NoError(t, app.Save(comp))

	svc := league.New(app, nil)
	rows, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	assert.Equal(t, 0, rows[0].Points)
	assert.Equal(t, 3, rows[0].Penalty)
}
