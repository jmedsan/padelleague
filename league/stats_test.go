package league

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tallyScore: isPair1 flag controls which side of the score counts

func TestTallyScore_IsPair1(t *testing.T) {
	t.Parallel()
	// Score "6-3 6-4": pair1 wins 2 sets, pair2 wins 0 sets.
	// Games: pair1 = 6+6=12, pair2 = 3+4=7.
	var got playerTotals
	tallyScore(&got, "6-3 6-4", true)
	assert.Equal(t, 2, got.setsWon, "isPair1: setsWon")
	assert.Equal(t, 0, got.setsLost, "isPair1: setsLost")
	assert.Equal(t, 12, got.gamesWon, "isPair1: gamesWon")
	assert.Equal(t, 7, got.gamesLost, "isPair1: gamesLost")
}

func TestTallyScore_IsPair2(t *testing.T) {
	t.Parallel()
	// Same score but isPair1=false: sides are swapped.
	var got playerTotals
	tallyScore(&got, "6-3 6-4", false)
	assert.Equal(t, 0, got.setsWon, "isPair2: setsWon")
	assert.Equal(t, 2, got.setsLost, "isPair2: setsLost")
	assert.Equal(t, 7, got.gamesWon, "isPair2: gamesWon")
	assert.Equal(t, 12, got.gamesLost, "isPair2: gamesLost")
}

func TestTallyScore_ThreeSets(t *testing.T) {
	t.Parallel()
	// "6-4 3-6 7-5": pair1 wins sets 1,3; pair2 wins set 2. Games: 16 vs 15.
	var got playerTotals
	tallyScore(&got, "6-4 3-6 7-5", true)
	assert.Equal(t, 2, got.setsWon)
	assert.Equal(t, 1, got.setsLost)
	assert.Equal(t, 16, got.gamesWon)
	assert.Equal(t, 15, got.gamesLost)
}

func TestTallyScore_WO_NoEffect(t *testing.T) {
	t.Parallel()
	var got playerTotals
	tallyScore(&got, "WO", true)
	assert.Equal(t, playerTotals{}, got)
}

func TestTallyScore_Accumulates(t *testing.T) {
	t.Parallel()
	var got playerTotals
	tallyScore(&got, "6-3 6-4", true)
	tallyScore(&got, "6-2 6-1", false) // as pair2: won 0 sets, lost 2
	assert.Equal(t, 2, got.setsWon, "accumulated setsWon")
	assert.Equal(t, 2, got.setsLost, "accumulated setsLost")
	assert.Equal(t, 12+3, got.gamesWon, "accumulated gamesWon")
	assert.Equal(t, 7+12, got.gamesLost, "accumulated gamesLost")
}

// computeCurrentStreak

func TestCurrentStreak(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		results []matchResult
		want    string
	}{
		{"empty", nil, ""},
		{"single win", []matchResult{{won: true}}, "1V"},
		{"single loss", []matchResult{{won: false}}, "1D"},
		{"three wins", []matchResult{{won: true}, {won: true}, {won: true}}, "3V"},
		{"two losses then win", []matchResult{{won: false}, {won: false}, {won: true}}, "2D"},
		{"win then loss", []matchResult{{won: true}, {won: false}}, "1V"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, computeCurrentStreak(tc.results))
		})
	}
}

// computeBestStreak

func TestBestStreak(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		results []matchResult
		want    string
	}{
		{"empty", nil, ""},
		{"all wins", []matchResult{{won: true}, {won: true}, {won: true}}, "3V"},
		{"all losses", []matchResult{{won: false}, {won: false}}, "0V"},
		{
			"win streak ignores losses",
			// W W W L L => bestWin=3
			[]matchResult{{won: true}, {won: true}, {won: true}, {won: false}, {won: false}},
			"3V",
		},
		{
			"only losses returns 0V",
			// W L L L => bestWin=1
			[]matchResult{{won: true}, {won: false}, {won: false}, {won: false}},
			"1V",
		},
		{
			"interleaved picks longest win run",
			// W W L W W W L => bestWin=3
			[]matchResult{{won: true}, {won: true}, {won: false}, {won: true}, {won: true}, {won: true}, {won: false}},
			"3V",
		},
		{
			"resets on loss",
			// L W W => bestWin=2
			[]matchResult{{won: false}, {won: true}, {won: true}},
			"2V",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, computeBestStreak(tc.results))
		})
	}
}

// buildRecentMatches: limit and field mapping

func TestBuildRecentMatches(t *testing.T) {
	t.Parallel()
	results := make([]matchResult, 25)
	for i := range results {
		results[i] = matchResult{
			matchID: "m" + string(rune('A'+i)),
			won:     i%2 == 0,
			p1:      "Pair A",
			p2:      "Pair B",
			score:   "6-3 6-4",
			date:    "2026-01-01",
		}
	}

	t.Run("limit caps at given value", func(t *testing.T) {
		got := buildRecentMatches(results, 10)
		assert.Len(t, got, 10)
	})

	t.Run("fewer than limit returns all", func(t *testing.T) {
		got := buildRecentMatches(results[:3], 10)
		assert.Len(t, got, 3)
	})

	t.Run("fields are mapped correctly", func(t *testing.T) {
		got := buildRecentMatches(results[:1], 5)
		require.Len(t, got, 1)
		assert.Equal(t, "mA", got[0].MatchID)
		assert.Equal(t, "Pair A", got[0].PairName1)
		assert.Equal(t, "Pair B", got[0].PairName2)
		assert.Equal(t, "6-3 6-4", got[0].Score)
		assert.True(t, got[0].Won)
		assert.Equal(t, "2026-01-01", got[0].Date)
	})
}

// Summarize: union of pair results, dedup, standings-derived competition stats

func TestSummarize_SinglePair(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	p1 := makePair(t, app, "SumPairA")
	p2 := makePair(t, app, "SumPairB")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "final")
	m.Set("scores", "6-3 6-4")
	m.Set("winner", p1.Id)
	require.NoError(t, app.Save(m))

	summary := svc.Summarize([]string{p1.Id})
	assert.Equal(t, 1, summary.TotalPlayed)
	assert.Equal(t, 1, summary.Wins)
	assert.Equal(t, 0, summary.Losses)
	assert.Equal(t, float64(100), summary.WinRate)
	assert.Equal(t, "1V", summary.Streak)
	require.Len(t, summary.CompetitionStats, 1)
	assert.Equal(t, 1, summary.CompetitionStats[0].Position, "single pair alone tops the standings")
}

func TestSummarize_UnionAcrossPairsDedupsSharedMatch(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	shared := makeUser(t, app, "Shared", "")
	other1 := makeUser(t, app, "Other1", "")
	other2 := makeUser(t, app, "Other2", "")

	pairCol, err := app.FindCollectionByNameOrId("pairs")
	require.NoError(t, err)

	pairA := core.NewRecord(pairCol)
	pairA.Set("name", "SumPairA")
	pairA.Set("player1", shared.Id)
	pairA.Set("player2", other1.Id)
	require.NoError(t, app.Save(pairA))

	pairB := core.NewRecord(pairCol)
	pairB.Set("name", "SumPairB")
	pairB.Set("player1", other2.Id)
	pairB.Set("player2", shared.Id)
	require.NoError(t, app.Save(pairB))

	comp := makeCompetition(t, app, []*core.Record{pairA, pairB})
	m := makeMatch(t, app, comp.Id, pairA.Id, pairB.Id, "final")
	m.Set("scores", "6-3 6-4")
	m.Set("winner", pairA.Id)
	require.NoError(t, app.Save(m))

	summary := svc.Summarize([]string{pairA.Id, pairB.Id})
	assert.Equal(t, 1, summary.TotalPlayed, "shared player on both pairs counts the match once")
	assert.Equal(t, 1, summary.Wins)
}

func TestSummarize_ZeroMatches(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	p1 := makePair(t, app, "SumEmptyPair")

	summary := svc.Summarize([]string{p1.Id})
	assert.Equal(t, 0, summary.TotalPlayed)
	assert.Equal(t, float64(0), summary.WinRate, "no division by zero")
	assert.Empty(t, summary.Streak)
}

func TestSummarize_CompetitionStatsFromStandings(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)

	p1 := makePair(t, app, "SumCompA")
	p2 := makePair(t, app, "SumCompB")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	for i := 0; i < 2; i++ {
		m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "final")
		m.Set("scores", "6-3 6-4")
		m.Set("winner", p1.Id)
		require.NoError(t, app.Save(m))
	}

	summary := svc.Summarize([]string{p1.Id})
	require.Len(t, summary.CompetitionStats, 1)
	cs := summary.CompetitionStats[0]
	assert.Equal(t, comp.Id, cs.CompID)
	assert.Equal(t, 2, cs.Played)
	assert.Equal(t, 2, cs.Wins)
	assert.Equal(t, 0, cs.Losses)
	assert.Equal(t, 1, cs.Position, "league position from ComputeStandings")
}

// currentStreakWinCount

func TestCurrentStreakWinCount(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		results []matchResult
		want    int
	}{
		{"empty", nil, 0},
		{"last was a loss", []matchResult{{won: false}, {won: true}}, 0},
		{"three wins", []matchResult{{won: true}, {won: true}, {won: true}}, 3},
		{"win streak stops at first loss", []matchResult{{won: true}, {won: true}, {won: false}, {won: true}}, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, currentStreakWinCount(tc.results))
		})
	}
}

// computeLevel

func TestComputeLevel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		winRatePct        float64
		played            int
		currentStreakWins int
		want              float64
	}{
		{"new player, no matches, floors at 1.0", 0, 0, 0, 1.0},
		{"new player, two matches, high win rate stays low", 100, 2, 2, 6.6},
		{"veteran, high win rate and hot streak scores near max", 80, 50, 8, 8.8},
		{"veteran, always loses scores near floor", 0, 50, 0, 2.0},
		{"average player", 50, 30, 0, 5.0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, computeLevel(tc.winRatePct, tc.played, tc.currentStreakWins))
		})
	}
}

// computeReliability / responsivenessScore / showUpScore

func TestComputeReliability_NoHistory_NeutralScore(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)
	p1 := makePair(t, app, "RelNoHistory")

	assert.Equal(t, float64(100), svc.computeReliability([]string{p1.Id}, 0))
}

func TestComputeReliability_FastResponsesAndNoWalkovers_HighScore(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)
	p1 := makePair(t, app, "RelFastA")
	p2 := makePair(t, app, "RelFastB")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "scheduled")
	m.Set("scores", "6-3 6-4")
	m.Set("winner", p1.Id)
	m.Set("status", "final")
	require.NoError(t, app.Save(m))

	proposer := PlayersForPair(app, p2.Id)[0]
	responder := PlayersForPair(app, p1.Id)[0]
	proposal := makeMessage(t, app, m.Id, proposer, "scheduling_proposal", "", time.Now().Add(-48*time.Hour))
	makeMessage(t, app, m.Id, responder, "scheduling_response", proposal.Id, time.Now().Add(-47*time.Hour))

	got := svc.computeReliability([]string{p1.Id}, 1)
	assert.InDelta(t, 100, got, 5, "same-hour response and no walkovers should score near max")
}

func TestComputeReliability_SlowResponse_LowerScore(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)
	p1 := makePair(t, app, "RelSlowA")
	p2 := makePair(t, app, "RelSlowB")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "final")

	proposer := PlayersForPair(app, p2.Id)[0]
	responder := PlayersForPair(app, p1.Id)[0]
	proposal := makeMessage(t, app, m.Id, proposer, "scheduling_proposal", "", time.Now().Add(-100*time.Hour))
	makeMessage(t, app, m.Id, responder, "scheduling_response", proposal.Id, time.Now().Add(-24*time.Hour))

	fast := svc.computeReliability([]string{p1.Id}, 1)

	app2 := newTestApp(t)
	svc2 := New(app2, nil)
	q1 := makePair(t, app2, "RelFastComp1")
	q2 := makePair(t, app2, "RelFastComp2")
	comp2 := makeCompetition(t, app2, []*core.Record{q1, q2})
	m2 := makeMatch(t, app2, comp2.Id, q1.Id, q2.Id, "final")
	proposer2 := PlayersForPair(app2, q2.Id)[0]
	responder2 := PlayersForPair(app2, q1.Id)[0]
	proposal2 := makeMessage(t, app2, m2.Id, proposer2, "scheduling_proposal", "", time.Now().Add(-2*time.Hour))
	makeMessage(t, app2, m2.Id, responder2, "scheduling_response", proposal2.Id, time.Now().Add(-1*time.Hour))
	slow := svc2.computeReliability([]string{q1.Id}, 1)

	assert.Less(t, fast, slow, "a 76h response should score lower than a 1h response")
}

func TestComputeReliability_WalkoverLoss_LowersScore(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := New(app, nil)
	p1 := makePair(t, app, "RelWOA")
	p2 := makePair(t, app, "RelWOB")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	m1 := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "final")
	m1.Set("scores", "6-3 6-4")
	m1.Set("winner", p1.Id)
	require.NoError(t, app.Save(m1))

	m2 := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "final")
	m2.Set("scores", "6-0 6-0")
	m2.Set("winner", p2.Id)
	m2.Set("review_type", "walkover")
	require.NoError(t, app.Save(m2))

	got := svc.computeReliability([]string{p1.Id}, 2)
	assert.Less(t, got, float64(100), "a walkover loss should reduce the reliability score")
}
