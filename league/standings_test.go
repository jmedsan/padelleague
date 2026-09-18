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

func TestComputeStandings_Form(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	p1 := makePair(t, app, "FormA")
	p2 := makePair(t, app, "FormB")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	// p1: loss, win, loss, win, win, win (oldest to newest) -> most recent 5
	// results, newest first: win, win, win, loss, win.
	dates := []string{"2026-01-01", "2026-01-02", "2026-01-03", "2026-01-04", "2026-01-05", "2026-01-06"}
	winners := []string{p2.Id, p1.Id, p2.Id, p1.Id, p1.Id, p1.Id}
	for i := range dates {
		m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "final")
		m.Set("scores", "6-3 6-4")
		m.Set("winner", winners[i])
		m.Set("date", dates[i])
		require.NoError(t, app.Save(m))
	}

	rows, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	var p1Row, p2Row StandingRowFull
	for _, r := range rows {
		if r.PairID == p1.Id {
			p1Row = r
		} else {
			p2Row = r
		}
	}

	require.Len(t, p1Row.Form, 5, "Form caps at the last 5 results")
	assert.Equal(t, []bool{true, true, true, false, true}, p1Row.Form, "most recent result first")

	require.Len(t, p2Row.Form, 5)
	assert.Equal(t, []bool{false, false, false, true, false}, p2Row.Form, "p2's form is p1's inverse for the same matches")
}

func TestComputeStandings_Form_NoMatches(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	p1 := makePair(t, app, "NoFormA")
	p2 := makePair(t, app, "NoFormB")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	rows, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Empty(t, rows[0].Form)
	assert.Empty(t, rows[1].Form)
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

	makePenalty(t, app, comp.Id, p1.Id, 2, false)

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
	// All have 3 points, tiebreaker is set diff then H2H then game diff
	for _, r := range rows {
		assert.Equal(t, 3, r.Points)
	}
}

// A pairwise head-to-head comparator is not transitive: A beats B, B beats
// C, C beats A is a valid 3-way cycle with no consistent pairwise order.
// FEP art. 3.3.10 resolves this with a mini-league among just the tied
// pairs rather than a pairwise sort. This case is fully symmetric (every
// pair has one 6-3 6-3 win and one 6-3 6-3 loss), so the mini-league itself
// leaves them level too — proving the sort terminates deterministically on
// a genuine cycle down to the final pair-name tiebreaker, not just that it
// doesn't crash.
func TestComputeStandings_ThreeWayCycle(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	a := makePair(t, app, "Cyc A")
	b := makePair(t, app, "Cyc B")
	c := makePair(t, app, "Cyc C")
	comp := makeCompetition(t, app, []*core.Record{a, b, c})

	finalMatch(t, app, comp.Id, a, b, a, "6-3 6-3", 1)
	finalMatch(t, app, comp.Id, b, c, b, "6-3 6-3", 2)
	finalMatch(t, app, comp.Id, c, a, c, "6-3 6-3", 3)

	rows, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	for _, r := range rows {
		assert.Equal(t, 3, r.Points, "precondition: all three level on points")
	}

	// Symmetric scores mean the mini-league and overall set/game diff are
	// level too, so the pair-name tiebreaker is what actually orders them —
	// assert that exact deterministic order.
	var order []string
	for _, r := range rows {
		order = append(order, r.PairName)
	}
	assert.Equal(t, []string{"Cyc A", "Cyc B", "Cyc C"}, order,
		"fully level 3-way cycle must fall back to pair-name order")
}

// Same cycle shape as above, but the mutual scores are no longer symmetric,
// so the mini-league's own set/game diff (computed only from matches among
// the tied trio) breaks the tie without needing to reach pair name.
func TestComputeStandings_ThreeWayCycleMiniLeagueSetDiffBreaksTie(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	a := makePair(t, app, "MLA")
	b := makePair(t, app, "MLB")
	c := makePair(t, app, "MLC")
	comp := makeCompetition(t, app, []*core.Record{a, b, c})

	// Each pair has exactly 1 win / 1 loss (3 mini-league points each, and
	// mini-league set diff level at 0 for all three, since every match is a
	// 2-0 sets result). Game diff is what separates them: a's win is a
	// blowout and its loss is a near-miss tiebreak set, so a comes out on
	// top; b's win is modest and its loss to a is a blowout, so b is last.
	finalMatch(t, app, comp.Id, a, b, a, "6-0 6-0", 1) // a: +2 sets, +12 games
	finalMatch(t, app, comp.Id, b, c, b, "6-4 6-4", 2) // b beats c: +2 sets, +4 games
	finalMatch(t, app, comp.Id, c, a, c, "7-6 7-6", 3) // a loses narrowly: -2 sets, -2 games

	rows, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	for _, r := range rows {
		assert.Equal(t, 3, r.Points, "precondition: all three level on points")
	}

	var order []string
	for _, r := range rows {
		order = append(order, r.PairName)
	}
	// Mini-league set diff (mutual matches only) is 0 for all three — every
	// pair wins one match 2 sets to 0 and loses one 0 sets to 2. It comes
	// down to mini-league game diff: a = +10 (12 won, 2 lost across its
	// blowout win and its narrow loss), c = -2, b = -8.
	assert.Equal(t, []string{"MLA", "MLC", "MLB"}, order,
		"mini-league game diff among the tied trio must break the cycle")
}

// Stability: the same 3-way cycle, fed in a different pair-registration
// order, must produce identical standings — the sort must not depend on
// input order once points, mini-league stats, and overall stats all agree.
func TestComputeStandings_ThreeWayCycleStableAcrossInputOrder(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	// Registered in a different order than the match sequence below.
	c := makePair(t, app, "STC")
	a := makePair(t, app, "STA")
	b := makePair(t, app, "STB")
	comp := makeCompetition(t, app, []*core.Record{c, a, b})

	finalMatch(t, app, comp.Id, a, b, a, "6-0 6-0", 1)
	finalMatch(t, app, comp.Id, b, c, b, "6-4 6-4", 2)
	finalMatch(t, app, comp.Id, c, a, c, "7-6 7-6", 3)

	rows, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	require.Len(t, rows, 3)

	var order []string
	for _, r := range rows {
		order = append(order, r.PairName)
	}
	assert.Equal(t, []string{"STA", "STC", "STB"}, order,
		"registration order must not change the resolved standings")
}

// A 2-way head-to-head split (1-1) must fall back to set/game difference
// computed from just the two head-to-head matches, before overall stats —
// per FEP art. 3.3.10 step 4.
func TestComputeStandings_TwoWaySplitUsesH2HSetDiff(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	a := makePair(t, app, "H2D A")
	b := makePair(t, app, "H2D B")
	comp := makeCompetition(t, app, []*core.Record{a, b})

	// a wins its match in straight, lopsided sets; b's win is a pair of
	// close tiebreaks. Both matches split 1-1 on wins, so overall set diff
	// is level (2-2) for both — but a's head-to-head set/game diff is
	// better because its win is the bigger blowout.
	finalMatch(t, app, comp.Id, a, b, a, "6-1 6-1", 1)
	finalMatch(t, app, comp.Id, b, a, b, "7-6 7-6", 2)

	rows, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	rowA := rowByName(t, rows, "H2D A")
	rowB := rowByName(t, rows, "H2D B")
	require.Equal(t, rowB.Points, rowA.Points, "precondition: level on points")
	require.Equal(t, rowA.SetsWon-rowA.SetsLost, rowB.SetsWon-rowB.SetsLost,
		"precondition: level on overall set difference")

	assert.Less(t, rowA.Position, rowB.Position,
		"a has the better head-to-head set/game diff even though overall stats are level")
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

	makePenalty(t, app, comp.Id, p1.Id, 1, false)

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

// Head-to-head breaks ties after points (FEP art. 3.3.10): two pairs
// level on points are split by who beat whom, before set/game diff.
func TestComputeStandings_HeadToHeadBreaksFullTie(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	a := makePair(t, app, "H2H A")
	b := makePair(t, app, "H2H B")
	c := makePair(t, app, "H2H C")
	d := makePair(t, app, "H2H D")
	comp := makeCompetition(t, app, []*core.Record{a, b, c, d})

	// Three pairs tied at 3 points (1W/1L each): a, b, d.
	// a beats b; b beats c; d beats a.
	// Overall matches played: a=2, b=2, d=1.
	// Tiebreaker criterion 1 (partidos jugados): a and b have played 2, d only 1.
	// d separates to last position among the three-way tie.
	// For the remaining sub-group {a, b}: head-to-head → a beat b → a ranks above b.
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
	// Order: a (most played + h2h over b), b, d (fewest played), c (0 pts).
	var order []string
	for i, r := range rows {
		assert.Equal(t, i+1, r.Position)
		order = append(order, r.PairName)
	}
	assert.Equal(t, []string{"H2H A", "H2H B", "H2H D", "H2H C"}, order)
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

// TestStandings_AdjustmentLeveled verifies that Adjustment and Score are
// computed correctly for a leveled league.
//
// Setup: 4 pairs, target_matches=3 (leveled: 3 < 4-1 is false, so use target=2
// with 4 pairs). Actually: 4 pairs, target=2 (< 4-1=3), which is leveled.
//
// Matches (all final):
//   A beats B  6-4 6-4  (A: 1W, B: 1L)
//   A beats C  6-4 6-4  (A: 2W, C: 1L)
//   D beats B  6-4 6-4  (D: 1W, B: 2L)
//   D beats C  6-4 6-4  (D: 2W, C: 2L)
//   B beats C  6-4 6-4  (B: 1W, C: 3L — wait, need to keep balanced)
//
// Simpler setup: A beats B, A beats C, D beats B, B beats C — all 2-0.
// Results:
//   A: 2W/0L, Points=6
//   D: 1W/1L, Points=3
//   B: 1W/2L, Points=3
//   C: 0W/2L, Points=0
//
// SOS(A) = mean win_rate of A's opponents (B and C):
//   win_rate(B) = 1/3, win_rate(C) = 0/2 = 0
//   SOS(A) = (1/3 + 0) / 2 = 1/6 ≈ 0.1667
//   Adjustment(A) = 3 × 2 × 1.5 × (1/6 - 0.5) = 9 × (-1/3) = -3.0
//
// SOS(D) = mean of D's played opponents: only B (D played A and B? — wait)
//
// Let me use a cleaner setup for predictable math.
//
// 4 pairs A,B,C,D with target_matches=3 (3 < 3 is false, target must be < pairs-1=3).
// Use target_matches=2 for 4 pairs (IsLeveled: 2 < 3 = true).
// Matches:
//   A beats B  (A: 1W, B: 1L)
//   A beats C  (A: 2W, C: 1L)
//   D beats B  (D: 1W, B: 2L)
//   D beats C  (D: 2W, C: 2L)
// Stats: A=2W/0L(Pts=6), D=2W/0L(Pts=6), B=0W/2L(Pts=0), C=0W/2L(Pts=0)
// SOS(A): A played B and C. win_rate(B)=0/2=0, win_rate(C)=0/2=0 → SOS=0
//   Adj(A) = 3×2×1.5×(0-0.5) = 9×(-0.5) = -4.5
// SOS(D): D played B and C. Same → Adj(D) = -4.5
// SOS(B): B played A and D. win_rate(A)=2/2=1, win_rate(D)=2/2=1 → SOS=1
//   Adj(B) = 3×2×1.5×(1-0.5) = 9×0.5 = 4.5
// SOS(C): C played A and D. → SOS=1, Adj(C) = 4.5
// Score: A=6-4.5=1.5, D=6-4.5=1.5, B=0+4.5=4.5, C=0+4.5=4.5
// Order by Score desc: B/C (4.5) > A/D (1.5).
// Tiebreak B vs C (same Score=4.5, same Points=0): by existing chain (set diff/game/name).
// Tiebreak A vs D (same Score=1.5, same Points=6): by existing chain.
//
// This proves the adjustment works AND that order follows Score not Points.
func TestStandings_AdjustmentLeveled(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	a := makePair(t, app, "Adj A")
	b := makePair(t, app, "Adj B")
	c := makePair(t, app, "Adj C")
	d := makePair(t, app, "Adj D")

	// target_matches=2, 4 pairs: 2 < 4-1=3 → IsLeveled=true
	comp := makeCompetition(t, app, []*core.Record{a, b, c, d})
	comp.Set("target_matches", 2)
	require.NoError(t, app.Save(comp))

	// A beats B and C; D beats B and C.
	finalMatch(t, app, comp.Id, a, b, a, "6-4 6-4", 0)
	finalMatch(t, app, comp.Id, a, c, a, "6-4 6-4", 0)
	finalMatch(t, app, comp.Id, d, b, d, "6-4 6-4", 0)
	finalMatch(t, app, comp.Id, d, c, d, "6-4 6-4", 0)

	rows, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	require.Len(t, rows, 4)

	// Locate each pair's row.
	rowA := rowByName(t, rows, "Adj A")
	rowB := rowByName(t, rows, "Adj B")
	rowC := rowByName(t, rows, "Adj C")
	rowD := rowByName(t, rows, "Adj D")

	// Base points.
	assert.Equal(t, 6, rowA.Points)
	assert.Equal(t, 6, rowD.Points)
	assert.Equal(t, 0, rowB.Points)
	assert.Equal(t, 0, rowC.Points)

	// Adjustments: A and D played two opponents each with 0% win rate.
	// SOS = 0, Adj = 3×2×1.5×(0−0.5) = −4.5
	assert.InDelta(t, -4.5, rowA.Adjustment, 0.05)
	assert.InDelta(t, -4.5, rowD.Adjustment, 0.05)

	// B and C played two opponents each with 100% win rate.
	// SOS = 1, Adj = 3×2×1.5×(1−0.5) = 4.5
	assert.InDelta(t, 4.5, rowB.Adjustment, 0.05)
	assert.InDelta(t, 4.5, rowC.Adjustment, 0.05)

	// Score = Points + Adjustment.
	assert.InDelta(t, 1.5, rowA.Score, 0.05)
	assert.InDelta(t, 1.5, rowD.Score, 0.05)
	assert.InDelta(t, 4.5, rowB.Score, 0.05)
	assert.InDelta(t, 4.5, rowC.Score, 0.05)

	// Order follows Score (descending): B/C (4.5) > A/D (1.5).
	// B and C both have Score 4.5; A and D both have 1.5.
	// Positions 1 and 2 must be B and C (in some order).
	top2 := map[string]bool{rows[0].PairName: true, rows[1].PairName: true}
	assert.True(t, top2["Adj B"], "B should be in top 2 by Score")
	assert.True(t, top2["Adj C"], "C should be in top 2 by Score")
	// Positions 3 and 4 must be A and D.
	bottom2 := map[string]bool{rows[2].PairName: true, rows[3].PairName: true}
	assert.True(t, bottom2["Adj A"], "A should be in bottom 2 by Score")
	assert.True(t, bottom2["Adj D"], "D should be in bottom 2 by Score")
}

// TestStandings_RoundRobinUnchanged verifies that Adjustment is 0 and the
// sort order is byte-identical to the pre-leveled behavior (P7).
func TestStandings_RoundRobinUnchanged(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	a := makePair(t, app, "RRU A")
	b := makePair(t, app, "RRU B")
	c := makePair(t, app, "RRU C")

	// target_matches=0 → IsLeveled=false.
	comp := makeCompetition(t, app, []*core.Record{a, b, c})
	// default target_matches is 0

	finalMatch(t, app, comp.Id, a, b, a, "6-3 6-4", 1)
	finalMatch(t, app, comp.Id, b, c, b, "6-3 6-4", 2)
	finalMatch(t, app, comp.Id, a, c, a, "6-3 6-4", 3)

	rows, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	require.Len(t, rows, 3)

	rowA := rowByName(t, rows, "RRU A")
	rowB := rowByName(t, rows, "RRU B")
	rowC := rowByName(t, rows, "RRU C")

	// Adjustment must be 0 for round-robin.
	assert.Equal(t, 0.0, rowA.Adjustment)
	assert.Equal(t, 0.0, rowB.Adjustment)
	assert.Equal(t, 0.0, rowC.Adjustment)

	// Score == float64(Points).
	assert.Equal(t, float64(rowA.Points), rowA.Score)
	assert.Equal(t, float64(rowB.Points), rowB.Score)
	assert.Equal(t, float64(rowC.Points), rowC.Score)

	// Order by points: A(6) > B(3) > C(0).
	assert.Equal(t, "RRU A", rows[0].PairName)
	assert.Equal(t, "RRU B", rows[1].PairName)
	assert.Equal(t, "RRU C", rows[2].PairName)
}

func TestPenaltyTotals_SumsActiveRows(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	p1 := makePair(t, app, "PT A")
	p2 := makePair(t, app, "PT B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	makePenalty(t, app, comp.Id, p1.Id, 3, false)
	makePenalty(t, app, comp.Id, p1.Id, 2, false)
	makePenalty(t, app, comp.Id, p1.Id, 5, true) // voided — must not count
	makePenalty(t, app, comp.Id, p2.Id, 1, false)

	totals, err := PenaltyTotals(app, comp.Id)
	require.NoError(t, err)
	assert.Equal(t, 5.0, totals[p1.Id], "active penalties for p1 must sum to 5")
	assert.Equal(t, 1.0, totals[p2.Id], "active penalties for p2 must sum to 1")
}

func TestPenaltyTotals_VoidedRowExcluded(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	p1 := makePair(t, app, "PV A")
	comp := makeCompetition(t, app, []*core.Record{p1})

	makePenalty(t, app, comp.Id, p1.Id, 3, false)

	totals, err := PenaltyTotals(app, comp.Id)
	require.NoError(t, err)
	assert.Equal(t, 3.0, totals[p1.Id])

	// Void the penalty
	rows, err := app.FindRecordsByFilter("penalties",
		"competition = {:c} && pair = {:p}", "", 0, 0,
		map[string]any{"c": comp.Id, "p": p1.Id})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	_, err = VoidPenalty(app, VoidPenaltyInput{PenaltyID: rows[0].Id})
	require.NoError(t, err)

	totals2, err := PenaltyTotals(app, comp.Id)
	require.NoError(t, err)
	assert.Zero(t, totals2[p1.Id], "voided penalty must drop from totals")
}

func TestStandings_PenaltyDeductsPoints(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	p1 := makePair(t, app, "PD A")
	p2 := makePair(t, app, "PD B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "final")
	m.Set("scores", "6-3 6-4")
	m.Set("winner", p1.Id)
	require.NoError(t, app.Save(m))

	makePenalty(t, app, comp.Id, p1.Id, 3, false)

	rows, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	var p1Row StandingRowFull
	for _, r := range rows {
		if r.PairID == p1.Id {
			p1Row = r
		}
	}
	assert.Equal(t, 0, p1Row.Points, "3 points from win minus 3 penalty = 0")
	assert.Equal(t, 3, p1Row.Penalty)
}

// TestComputeStandings_SubGroupFreshMiniStats verifies that mini-league stats
// are recomputed fresh for each sub-group (FEP §3.3.10 P2). The bug: the old
// code captured 4-way mini stats in closures and reused them when recursing
// into the {B,C,D} sub-group, producing a wrong ordering.
//
// Setup: 4 pairs A, B, C, D all at 3 points. A gets 3 mini-wins (separates
// to position 1). In the {B,C,D} sub-group:
//   - Fresh game diff: D(+4) > B(+2) > C(-6)  →  correct order D, B, C
//   - 4-way game diff: B(0) > D(-8) > C(-18)  →  bug order B, D, C
//
// Expected full order: [A, D, B, C].
func TestComputeStandings_SubGroupFreshMiniStats(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	a := makePair(t, app, "SG A")
	b := makePair(t, app, "SG B")
	c := makePair(t, app, "SG C")
	d := makePair(t, app, "SG D")
	comp := makeCompetition(t, app, []*core.Record{a, b, c, d})

	// A beats B barely (7-6 7-6): A gains +14 games, B gains +12 games from this match.
	// Using a penalty of 6 on A to bring A's points to 3 (same as B/C/D with 1W each).
	finalMatch(t, app, comp.Id, a, b, a, "7-6 7-6", 1)
	// A crushes C and D: both gain 0 games from these losses.
	finalMatch(t, app, comp.Id, a, c, a, "6-0 6-0", 2)
	finalMatch(t, app, comp.Id, a, d, a, "6-0 6-0", 3)

	// {B,C,D} ring: B→C→D→B with carefully chosen scores.
	// Fresh {B,C,D} game diffs: B=+2, C=-6, D=+4  →  D > B > C.
	// 4-way game diffs for {B,C,D} (adding A-match contribution):
	//   B: +2 + (12-14) = 0;  C: -6 + (0-12) = -18;  D: +4 + (0-12) = -8  →  B > D > C.
	finalMatch(t, app, comp.Id, b, c, b, "6-0 6-0", 4) // B beats C 6-0 6-0
	finalMatch(t, app, comp.Id, c, d, c, "6-3 6-3", 5) // C beats D 6-3 6-3
	finalMatch(t, app, comp.Id, d, b, d, "6-1 6-1", 6) // D beats B 6-1 6-1

	// Penalty of 6 brings A from 9pts (3W) down to 3pts, equal to B/C/D.
	makePenalty(t, app, comp.Id, a.Id, 6, false)

	rows, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	require.Len(t, rows, 4)

	var order []string
	for i, r := range rows {
		assert.Equal(t, i+1, r.Position)
		order = append(order, r.PairName)
	}
	// Correct (fresh sub-group stats): A separates first (3 mini-wins),
	// then D > B > C by fresh {B,C,D} game diff.
	// Bug (4-way stats for sub-group): would produce [A, B, D, C].
	assert.Equal(t, []string{"SG A", "SG D", "SG B", "SG C"}, order)
}
