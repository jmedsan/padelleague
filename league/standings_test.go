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
