package league

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeStandings_Basic(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestComputeStandings_PenaltyStoredAsJSONText(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	p1 := makePair(t, app, "PenS A")
	p2 := makePair(t, app, "PenS B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "final")
	m.Set("scores", "6-3 6-4")
	m.Set("winner", p1.Id)
	require.NoError(t, app.Save(m))

	// Written straight to SQLite as JSON text; PocketBase still reads a
	// JSONField back as types.JSONRaw, which is the branch under test.
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	_, err := svc.ComputeStandings("nonexistent")
	assert.Error(t, err)
}

func TestComputeStandings_ThreeSets(t *testing.T) {
	t.Parallel()
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

// finalMatch records a completed match with an explicit score and winner.
func finalMatch(t *testing.T, app core.App, compID string, p1, p2, winner *core.Record, scores string, round int) {
	t.Helper()
	m := makeMatch(t, app, compID, p1.Id, p2.Id, "final")
	m.Set("scores", scores)
	m.Set("winner", winner.Id)
	m.Set("round_number", round)
	require.NoError(t, app.Save(m))
}

func rowByName(t *testing.T, rows []StandingRowFull, name string) StandingRowFull {
	t.Helper()
	for _, r := range rows {
		if r.PairName == name {
			return r
		}
	}
	t.Fatalf("pair %q not in standings", name)
	return StandingRowFull{}
}

// Head-to-head is the last tiebreaker: two pairs level on points, set
// difference and game difference must be split by who beat whom.
func TestComputeStandings_HeadToHeadBreaksFullTie(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	a := makePair(t, app, "H2H A")
	b := makePair(t, app, "H2H B")
	c := makePair(t, app, "H2H C")
	d := makePair(t, app, "H2H D")
	comp := makeCompetition(t, app, []*core.Record{a, b, c, d})

	// a and b end level: one win and one loss each, identical set and game
	// difference. a's win is the head-to-head against b.
	finalMatch(t, app, comp.Id, a, b, a, "6-3 6-3", 1)
	finalMatch(t, app, comp.Id, b, c, b, "6-3 6-3", 2)
	finalMatch(t, app, comp.Id, d, a, d, "6-3 6-3", 3)

	rows, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	require.Len(t, rows, 4)

	rowA := rowByName(t, rows, "H2H A")
	rowB := rowByName(t, rows, "H2H B")
	require.Equal(t, rowB.Points, rowA.Points, "precondition: level on points")
	require.Equal(t, rowA.SetsWon-rowA.SetsLost, rowB.SetsWon-rowB.SetsLost,
		"precondition: level on set difference")
	require.Equal(t, rowA.GamesWon-rowA.GamesLost, rowB.GamesWon-rowB.GamesLost,
		"precondition: level on game difference")

	assert.Less(t, rowA.Position, rowB.Position,
		"a beat b head-to-head, so a must rank above b")

	// Positions are 1-based and dense.
	var order []string
	for i, r := range rows {
		assert.Equal(t, i+1, r.Position)
		order = append(order, r.PairName)
	}
	assert.Equal(t, []string{"H2H D", "H2H A", "H2H B", "H2H C"}, order)
}

// Set difference outranks game difference, and is computed as won minus lost.
func TestComputeStandings_SetDiffOutranksGameDiff(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	sweep := makePair(t, app, "SD Sweep")
	grind := makePair(t, app, "SD Grind")
	foil1 := makePair(t, app, "SD Foil1")
	foil2 := makePair(t, app, "SD Foil2")
	comp := makeCompetition(t, app, []*core.Record{sweep, grind, foil1, foil2})

	// Both win once (3 points each).
	// sweep wins in straight sets: set diff +2, game diff +4.
	finalMatch(t, app, comp.Id, sweep, foil1, sweep, "6-4 6-4", 1)
	// grind wins in three: set diff +1, but a larger game diff (+6).
	finalMatch(t, app, comp.Id, grind, foil2, grind, "6-0 0-6 6-0", 2)

	rows, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)

	rowSweep := rowByName(t, rows, "SD Sweep")
	rowGrind := rowByName(t, rows, "SD Grind")
	require.Equal(t, rowGrind.Points, rowSweep.Points, "precondition: level on points")
	require.Equal(t, 2, rowSweep.SetsWon-rowSweep.SetsLost)
	require.Equal(t, 1, rowGrind.SetsWon-rowGrind.SetsLost)
	require.Greater(t, rowGrind.GamesWon-rowGrind.GamesLost, rowSweep.GamesWon-rowSweep.GamesLost,
		"precondition: grind has the better game difference")

	assert.Less(t, rowSweep.Position, rowGrind.Position,
		"set difference is checked before game difference")
}

// Win and loss counters must be credited to the right side, in both the
// normal and the walkover path.
func TestComputeStandings_WinLossCounters(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	a := makePair(t, app, "WL A")
	b := makePair(t, app, "WL B")
	comp := makeCompetition(t, app, []*core.Record{a, b})

	// a is always pair1, so this covers all four credit paths: a normal win
	// for each side, and a walkover for each side.
	finalMatch(t, app, comp.Id, a, b, a, "6-3 6-4", 1)
	finalMatch(t, app, comp.Id, a, b, b, "6-3 6-4", 2)
	finalMatch(t, app, comp.Id, a, b, a, "WO", 3)
	finalMatch(t, app, comp.Id, a, b, b, "WO", 4)

	rows, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)

	rowA := rowByName(t, rows, "WL A")
	rowB := rowByName(t, rows, "WL B")

	assert.Equal(t, 2, rowA.Wins)
	assert.Equal(t, 2, rowA.Losses)
	assert.Equal(t, 4, rowA.Played)
	assert.Equal(t, 6, rowA.Points)
	assert.Equal(t, 2, rowB.Wins)
	assert.Equal(t, 2, rowB.Losses)
	assert.Equal(t, 4, rowB.Played)
	assert.Equal(t, 6, rowB.Points)
}

// A head-to-head that is level (one win each) resolves nothing, so the two
// pairs must keep their existing order rather than being reshuffled.
func TestComputeStandings_SplitHeadToHeadLeavesOrderAlone(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	first := makePair(t, app, "SP First")
	second := makePair(t, app, "SP Second")
	comp := makeCompetition(t, app, []*core.Record{first, second})

	// Mirrored results: level on points, sets and games, and 1-1 head-to-head.
	finalMatch(t, app, comp.Id, first, second, first, "6-3 6-4", 1)
	finalMatch(t, app, comp.Id, second, first, second, "6-3 6-4", 2)

	rows, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	rowFirst := rowByName(t, rows, "SP First")
	rowSecond := rowByName(t, rows, "SP Second")
	require.Equal(t, rowSecond.Points, rowFirst.Points, "precondition: level on points")
	require.Equal(t, rowFirst.SetsWon-rowFirst.SetsLost, rowSecond.SetsWon-rowSecond.SetsLost)
	require.Equal(t, rowFirst.GamesWon-rowFirst.GamesLost, rowSecond.GamesWon-rowSecond.GamesLost)

	assert.Equal(t, 1, rowFirst.Position,
		"nothing separates them, so registration order stands")
	assert.Equal(t, 2, rowSecond.Position)
}

// Same full tie as above, but the head-to-head winner is registered *after*
// the loser. A comparator that treats them as equal would leave the input
// order untouched and silently rank the wrong pair first.
func TestComputeStandings_HeadToHeadOverridesInputOrder(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	loser := makePair(t, app, "HO Loser")
	winner := makePair(t, app, "HO Winner")
	c := makePair(t, app, "HO C")
	d := makePair(t, app, "HO D")
	// loser is registered first, so it starts ahead of winner.
	comp := makeCompetition(t, app, []*core.Record{loser, winner, c, d})

	finalMatch(t, app, comp.Id, winner, loser, winner, "6-3 6-3", 1)
	finalMatch(t, app, comp.Id, loser, c, loser, "6-3 6-3", 2)
	finalMatch(t, app, comp.Id, d, winner, d, "6-3 6-3", 3)

	rows, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)

	rowWinner := rowByName(t, rows, "HO Winner")
	rowLoser := rowByName(t, rows, "HO Loser")
	require.Equal(t, rowLoser.Points, rowWinner.Points, "precondition: level on points")
	require.Equal(t, rowWinner.SetsWon-rowWinner.SetsLost, rowLoser.SetsWon-rowLoser.SetsLost,
		"precondition: level on set difference")
	require.Equal(t, rowWinner.GamesWon-rowWinner.GamesLost, rowLoser.GamesWon-rowLoser.GamesLost,
		"precondition: level on game difference")

	assert.Less(t, rowWinner.Position, rowLoser.Position,
		"head-to-head must promote the winner above its registration order")
}
