package league

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAwards_Basic(t *testing.T) {
	app := newTestApp(t)
	svc := New(app, nil)

	p1 := makePair(t, app, "Aw A")
	p2 := makePair(t, app, "Aw B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	for i := 0; i < 3; i++ {
		m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "final")
		m.Set("scores", "6-3 6-4")
		m.Set("winner", p1.Id)
		m.Set("round_number", i+1)
		require.NoError(t, app.Save(m))
	}

	awards := svc.Awards(comp.Id)
	require.NotEmpty(t, awards)

	var titles []string
	for _, a := range awards {
		titles = append(titles, a.Title)
	}
	assert.Contains(t, titles, "Mejor pareja")
	assert.Contains(t, titles, "Más partidos")
	assert.Contains(t, titles, "Mayor racha")
}

func TestAwards_NoMatches(t *testing.T) {
	app := newTestApp(t)
	svc := New(app, nil)

	p1 := makePair(t, app, "AwNone A")
	p2 := makePair(t, app, "AwNone B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	_ = comp

	awards := svc.Awards(comp.Id)
	assert.Empty(t, awards)
}

func TestAwards_InvalidCompetition(t *testing.T) {
	app := newTestApp(t)
	svc := New(app, nil)

	awards := svc.Awards("nonexistent")
	assert.Nil(t, awards)
}

func TestAwards_NoStreak(t *testing.T) {
	app := newTestApp(t)
	svc := New(app, nil)

	p1 := makePair(t, app, "AwNoStr A")
	p2 := makePair(t, app, "AwNoStr B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	// One win each — no streak > 1
	m1 := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "final")
	m1.Set("scores", "6-3 6-4")
	m1.Set("winner", p1.Id)
	require.NoError(t, app.Save(m1))

	m2 := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "final")
	m2.Set("scores", "3-6 4-6")
	m2.Set("winner", p2.Id)
	m2.Set("round_number", 2)
	require.NoError(t, app.Save(m2))

	awards := svc.Awards(comp.Id)
	for _, a := range awards {
		assert.NotEqual(t, "Mayor racha", a.Title)
	}
}

// awardByTitle returns the award with the given title, failing if absent.
func awardByTitle(t *testing.T, awards []Award, title string) Award {
	t.Helper()
	for _, a := range awards {
		if a.Title == title {
			return a
		}
	}
	t.Fatalf("award %q not found in %v", title, awards)
	return Award{}
}

// "Más partidos" must name the pair that actually played most, even when that
// pair sits last in the standings.
func TestAwards_MostPlayedIsNotTheLeader(t *testing.T) {
	app := newTestApp(t)
	svc := New(app, nil)

	winner := makePair(t, app, "MP Winner")
	middle := makePair(t, app, "MP Middle")
	loser := makePair(t, app, "MP Loser")
	comp := makeCompetition(t, app, []*core.Record{winner, middle, loser})

	// loser plays 4 and loses all; winner plays 3, middle plays 1.
	round := 1
	addFinal := func(a, b *core.Record, won *core.Record) {
		m := makeMatch(t, app, comp.Id, a.Id, b.Id, "final")
		m.Set("scores", "6-3 6-4")
		m.Set("winner", won.Id)
		m.Set("round_number", round)
		round++
		require.NoError(t, app.Save(m))
	}
	addFinal(winner, loser, winner)
	addFinal(winner, loser, winner)
	addFinal(winner, loser, winner)
	addFinal(middle, loser, middle)

	standings, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	require.Equal(t, "MP Winner", standings[0].PairName, "leader precondition")

	got := awardByTitle(t, svc.Awards(comp.Id), "Más partidos")
	assert.Equal(t, "MP Loser", got.PairName)
	assert.Equal(t, "4 partidos", got.Value)
}

// On a tie for most played the first pair in standings order keeps the award;
// a >= comparison would hand it to the last one scanned instead.
func TestAwards_MostPlayedTieKeepsStandingsLeader(t *testing.T) {
	app := newTestApp(t)
	svc := New(app, nil)

	a := makePair(t, app, "Tie A")
	b := makePair(t, app, "Tie B")
	c := makePair(t, app, "Tie C")
	comp := makeCompetition(t, app, []*core.Record{a, b, c})

	round := 1
	addFinal := func(x, y, won *core.Record) {
		m := makeMatch(t, app, comp.Id, x.Id, y.Id, "final")
		m.Set("scores", "6-3 6-4")
		m.Set("winner", won.Id)
		m.Set("round_number", round)
		round++
		require.NoError(t, app.Save(m))
	}
	// Every pair plays exactly 2: a beats b, a beats c, b beats c.
	addFinal(a, b, a)
	addFinal(a, c, a)
	addFinal(b, c, b)

	standings, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	for _, s := range standings {
		require.Equal(t, 2, s.Played, "tie precondition: every pair played 2")
	}

	got := awardByTitle(t, svc.Awards(comp.Id), "Más partidos")
	assert.Equal(t, standings[0].PairName, got.PairName)
}

// The streak must be credited to the winner, not the loser.
func TestAwards_StreakBelongsToWinner(t *testing.T) {
	app := newTestApp(t)
	svc := New(app, nil)

	champ := makePair(t, app, "Streak Champ")
	foil := makePair(t, app, "Streak Foil")
	comp := makeCompetition(t, app, []*core.Record{champ, foil})

	for i := 0; i < 3; i++ {
		m := makeMatch(t, app, comp.Id, champ.Id, foil.Id, "final")
		m.Set("scores", "6-3 6-4")
		m.Set("winner", champ.Id)
		m.Set("round_number", i+1)
		require.NoError(t, app.Save(m))
	}

	awards := svc.Awards(comp.Id)

	streak := awardByTitle(t, awards, "Mayor racha")
	assert.Equal(t, "Streak Champ", streak.PairName)
	assert.Equal(t, "3 victorias", streak.Value)

	best := awardByTitle(t, awards, "Mejor pareja")
	assert.Equal(t, "Streak Champ", best.PairName)
	assert.Equal(t, "9 pts", best.Value)
}

// Two pairs on an equal longest streak must award the better placed one, and
// must do so on every call — iterating the streaks map made this random.
func TestAwards_StreakTieIsDeterministic(t *testing.T) {
	app := newTestApp(t)
	svc := New(app, nil)

	a := makePair(t, app, "Rac A")
	b := makePair(t, app, "Rac B")
	c := makePair(t, app, "Rac C")
	d := makePair(t, app, "Rac D")
	comp := makeCompetition(t, app, []*core.Record{a, b, c, d})

	round := 1
	addFinal := func(x, y, won *core.Record, scores string) {
		m := makeMatch(t, app, comp.Id, x.Id, y.Id, "final")
		m.Set("scores", scores)
		m.Set("winner", won.Id)
		m.Set("round_number", round)
		round++
		require.NoError(t, app.Save(m))
	}
	// a and b each win two in a row: both peak at a streak of 2.
	// a wins by a wider margin so it places above b in the standings.
	addFinal(a, c, a, "6-0 6-0")
	addFinal(a, d, a, "6-0 6-0")
	addFinal(b, c, b, "7-5 7-5")
	addFinal(b, d, b, "7-5 7-5")

	standings, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	require.Equal(t, "Rac A", standings[0].PairName, "precondition: a places first")

	first := awardByTitle(t, svc.Awards(comp.Id), "Mayor racha")
	assert.Equal(t, "Rac A", first.PairName)
	assert.Equal(t, "2 victorias", first.Value)

	// Repeat: map iteration order varies per run, so a map-order winner would
	// eventually disagree with itself here.
	for i := 0; i < 20; i++ {
		again := awardByTitle(t, svc.Awards(comp.Id), "Mayor racha")
		require.Equal(t, first.PairName, again.PairName, "award changed on call %d", i+2)
	}
}
