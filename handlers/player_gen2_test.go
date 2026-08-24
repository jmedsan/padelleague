package handlers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- tallyScore: isPair1 flag controls which side of the score counts ---

func TestGen2_TallyScore_IsPair1(t *testing.T) {
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

func TestGen2_TallyScore_IsPair2(t *testing.T) {
	t.Parallel()
	// Same score but isPair1=false: sides are swapped.
	var got playerTotals
	tallyScore(&got, "6-3 6-4", false)
	assert.Equal(t, 0, got.setsWon, "isPair2: setsWon")
	assert.Equal(t, 2, got.setsLost, "isPair2: setsLost")
	assert.Equal(t, 7, got.gamesWon, "isPair2: gamesWon")
	assert.Equal(t, 12, got.gamesLost, "isPair2: gamesLost")
}

func TestGen2_TallyScore_ThreeSets(t *testing.T) {
	t.Parallel()
	// "6-4 3-6 7-5": pair1 wins sets 1,3; pair2 wins set 2. Games: 16 vs 15.
	var got playerTotals
	tallyScore(&got, "6-4 3-6 7-5", true)
	assert.Equal(t, 2, got.setsWon)
	assert.Equal(t, 1, got.setsLost)
	assert.Equal(t, 16, got.gamesWon)
	assert.Equal(t, 15, got.gamesLost)
}

func TestGen2_TallyScore_WO_NoEffect(t *testing.T) {
	t.Parallel()
	var got playerTotals
	tallyScore(&got, "WO", true)
	assert.Equal(t, playerTotals{}, got)
}

func TestGen2_TallyScore_Accumulates(t *testing.T) {
	t.Parallel()
	var got playerTotals
	tallyScore(&got, "6-3 6-4", true)
	tallyScore(&got, "6-2 6-1", false) // as pair2: won 0 sets, lost 2
	assert.Equal(t, 2, got.setsWon, "accumulated setsWon")
	assert.Equal(t, 2, got.setsLost, "accumulated setsLost")
	assert.Equal(t, 12+3, got.gamesWon, "accumulated gamesWon")
	assert.Equal(t, 7+12, got.gamesLost, "accumulated gamesLost")
}

// --- computeCurrentStreak ---

func TestGen2_CurrentStreak(t *testing.T) {
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

// --- computeBestStreak ---

func TestGen2_BestStreak(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		results []matchResult
		want    string
	}{
		{"empty", nil, ""},
		{"all wins", []matchResult{{won: true}, {won: true}, {won: true}}, "3V"},
		{"all losses", []matchResult{{won: false}, {won: false}}, "2D"},
		{
			"win streak beats loss streak",
			// W W W L L => bestWin=3, bestLoss=2 => 3V (3 >= 2)
			[]matchResult{{won: true}, {won: true}, {won: true}, {won: false}, {won: false}},
			"3V",
		},
		{
			"loss streak beats win streak",
			// W L L L => bestWin=1, bestLoss=3 => 3D (1 < 3)
			[]matchResult{{won: true}, {won: false}, {won: false}, {won: false}},
			"3D",
		},
		{
			"tie favors win streak (>= boundary)",
			// W W L L => bestWin=2, bestLoss=2 => 2V (2 >= 2)
			[]matchResult{{won: true}, {won: true}, {won: false}, {won: false}},
			"2V",
		},
		{
			"interleaved picks longest",
			// W W L W W W L => bestWin=3
			[]matchResult{{won: true}, {won: true}, {won: false}, {won: true}, {won: true}, {won: true}, {won: false}},
			"3V",
		},
		{
			"resets on direction change",
			// L W W => curLoss resets when win starts, bestLoss=1, bestWin=2 => 2V
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

// --- buildRecentMatches: limit and field mapping ---

func TestGen2_BuildRecentMatches(t *testing.T) {
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

// --- Player profile page: partner name, stats, win rate, streaks, comp stats ---

func TestGen2_PlayerProfile_FullStats(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player profile shows correct partner, stats, streak, comp stats",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"PadelLeague"},
	}

	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)

		// Create two users manually to control who is player1/player2.
		u1 := makeUserTB(tb, app, "Alice", "alice@test.local")
		u2 := makeUserTB(tb, app, "Bob", "bob@test.local")

		// Create pair with u1 as player1, u2 as player2.
		col, err := app.FindCollectionByNameOrId("pairs")
		require.NoError(tb, err)
		pair := core.NewRecord(col)
		pair.Set("name", "A&B")
		pair.Set("player1", u1.Id)
		pair.Set("player2", u2.Id)
		require.NoError(tb, app.Save(pair))

		// Create competition with this pair.
		compCol, err := app.FindCollectionByNameOrId("competitions")
		require.NoError(tb, err)
		comp := core.NewRecord(compCol)
		comp.Set("name", "Gen2 League")
		comp.Set("type", "league")
		comp.Set("active", true)
		comp.Set("pairs", []string{pair.Id})
		require.NoError(tb, app.Save(comp))

		// Create a second pair as opponent.
		opp := makePairTB(tb, app, "Opp")

		// Match 1: pair is pair1, wins "6-3 6-4" (date earlier).
		matchCol, err := app.FindCollectionByNameOrId("matches")
		require.NoError(tb, err)
		m1 := core.NewRecord(matchCol)
		m1.Set("competition", comp.Id)
		m1.Set("pair1", pair.Id)
		m1.Set("pair2", opp.Id)
		m1.Set("status", "final")
		m1.Set("scores", "6-3 6-4")
		m1.Set("winner", pair.Id)
		m1.Set("round_number", 1)
		m1.Set("date", "2026-01-01")
		require.NoError(tb, app.Save(m1))

		// Match 2: pair is pair2, loses "6-2 6-1" (date later).
		m2 := core.NewRecord(matchCol)
		m2.Set("competition", comp.Id)
		m2.Set("pair1", opp.Id)
		m2.Set("pair2", pair.Id)
		m2.Set("status", "final")
		m2.Set("scores", "6-2 6-1")
		m2.Set("winner", opp.Id)
		m2.Set("round_number", 2)
		m2.Set("date", "2026-01-15")
		require.NoError(tb, app.Save(m2))

		s.URL = "/player/" + u1.Id
		s.Headers = authHeaders(tb, u1)
	}

	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)

		assert.Contains(tb, body, "Bob", "partner name should be Bob")
		assert.Contains(tb, body, ">2</div>", "TotalPlayed should be 2")
		assert.Contains(tb, body, "50%", "WinRate should be 50%")
		assert.Contains(tb, body, "2/2", "SetsWon/SetsLost should be 2/2")
		assert.Contains(tb, body, "15/19", "GamesWon/GamesLost should be 15/19")
		assert.Contains(tb, body, "1D", "current streak should be 1D")
		assert.Contains(tb, body, "1V", "best streak should be 1V")
		assert.Contains(tb, body, "Gen2 League", "competition name")
	}

	s.Test(t)
}

func readBody(tb testing.TB, res *http.Response) string {
	tb.Helper()
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 1024)
	for {
		n, err := res.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return string(buf)
}

// --- Player profile: date ordering of recent matches ---

func TestGen2_PlayerProfile_DateOrdering(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player profile recent matches sorted by date descending",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"PadelLeague"},
	}

	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)

		pair := makePairTB(tb, app, "DateP")
		opp := makePairTB(tb, app, "DateOpp")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{pair, opp})

		matchCol, err := app.FindCollectionByNameOrId("matches")
		require.NoError(tb, err)

		// Earlier match.
		m1 := core.NewRecord(matchCol)
		m1.Set("competition", comp.Id)
		m1.Set("pair1", pair.Id)
		m1.Set("pair2", opp.Id)
		m1.Set("status", "final")
		m1.Set("scores", "6-3 6-4")
		m1.Set("winner", pair.Id)
		m1.Set("round_number", 1)
		m1.Set("date", "2026-01-01")
		require.NoError(tb, app.Save(m1))

		// Later match.
		m2 := core.NewRecord(matchCol)
		m2.Set("competition", comp.Id)
		m2.Set("pair1", opp.Id)
		m2.Set("pair2", pair.Id)
		m2.Set("status", "final")
		m2.Set("scores", "6-1 6-2")
		m2.Set("winner", opp.Id)
		m2.Set("round_number", 2)
		m2.Set("date", "2026-02-01")
		require.NoError(tb, app.Save(m2))

		user, _ := app.FindRecordById("users", pair.GetString("player1"))
		s.URL = "/player/" + user.Id
		s.Headers = authHeaders(tb, user)
	}

	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		// Later match (Feb) should appear before earlier match (Jan) in the HTML.
		febIdx := indexOf(body, "2026-02-01")
		janIdx := indexOf(body, "2026-01-01")
		require.NotEqual(tb, -1, febIdx, "Feb match should be in output")
		require.NotEqual(tb, -1, janIdx, "Jan match should be in output")
		assert.Less(tb, febIdx, janIdx, "Feb match should appear before Jan match (desc order)")
	}

	s.Test(t)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// --- H2H: only p2 set redirects to player page (followed by test runner) ---

func TestGen2_H2H_OnlyP2_ShowsPlayerPage(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "GET /h2h with only p2 redirects to player page",
		Method:         http.MethodGet,
		ExpectedStatus: 302,
	}

	var pairID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "H2HP2Only")
		pairID = p1.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.URL = "/h2h?p2=" + p1.Id
		s.Headers = authHeaders(tb, user)
	}

	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		loc := res.Header.Get("Location")
		assert.Equal(tb, "/player/"+pairID, loc)
	}

	s.Test(t)
}

// --- H2H: only p1 set redirects to player page ---

func TestGen2_H2H_OnlyP1_ShowsPlayerPage(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "GET /h2h with only p1 redirects to player page",
		Method:         http.MethodGet,
		ExpectedStatus: 302,
	}

	var pairID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "H2HP1Only")
		pairID = p1.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.URL = "/h2h?p1=" + p1.Id
		s.Headers = authHeaders(tb, user)
	}

	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		loc := res.Header.Get("Location")
		assert.Equal(tb, "/player/"+pairID, loc)
	}

	s.Test(t)
}

// --- H2H: both empty redirects to home ---

func TestGen2_H2H_BothEmpty_ShowsHome(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "GET /h2h with no params redirects to home",
		Method:         http.MethodGet,
		URL:            "/h2h",
		ExpectedStatus: 302,
	}

	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		user := makeUserTB(tb, app, "H2HEmpty", "")
		s.Headers = authHeaders(tb, user)
	}

	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		loc := res.Header.Get("Location")
		assert.Equal(tb, "/", loc)
	}

	s.Test(t)
}

// --- tallyH2H: wins counting and recent limit ---

func TestGen2_TallyH2H(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "h2h tallies wins correctly and limits recent to 5",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"PadelLeague"},
	}

	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)

		p1 := makePairTB(tb, app, "H2HTalA")
		p2 := makePairTB(tb, app, "H2HTalB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		matchCol, err := app.FindCollectionByNameOrId("matches")
		require.NoError(tb, err)

		// Create 7 matches: p1 wins 4, p2 wins 3.
		scores := []struct {
			winner string
			score  string
		}{
			{p1.Id, "6-3 6-4"},
			{p2.Id, "6-2 6-1"},
			{p1.Id, "6-4 6-3"},
			{p2.Id, "7-5 6-4"},
			{p1.Id, "6-1 6-2"},
			{p2.Id, "6-3 6-4"},
			{p1.Id, "6-0 6-1"},
		}
		for i, sc := range scores {
			m := core.NewRecord(matchCol)
			m.Set("competition", comp.Id)
			m.Set("pair1", p1.Id)
			m.Set("pair2", p2.Id)
			m.Set("status", "final")
			m.Set("scores", sc.score)
			m.Set("winner", sc.winner)
			m.Set("round_number", i+1)
			require.NoError(tb, app.Save(m))
		}

		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.URL = "/h2h?p1=" + p1.Id + "&p2=" + p2.Id
		s.Headers = authHeaders(tb, user)
	}

	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		// Template: <div class="stat-value text-2xl">N</div> for Wins1, Total, Wins2.
		// 7 matches, p1 wins 4, p2 wins 3.
		assert.Contains(tb, body, ">4</div>", "p1 wins stat-value should be 4")
		assert.Contains(tb, body, ">7</div>", "total stat-value should be 7")
		assert.Contains(tb, body, ">3</div>", "p2 wins stat-value should be 3")

		// Recent is capped at 5: with 7 matches, only 5 rows should appear.
		// Each row has H2HTalA as PairName1, which only appears in <td> cells.
		rowCount := strings.Count(body, "<td>H2HTalA</td>")
		assert.Equal(tb, 5, rowCount, "recent should be capped at 5 entries")
	}

	s.Test(t)
}

// --- Competition stats: per-comp played/wins/losses ---

func TestGen2_PlayerProfile_CompetitionStats(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player profile shows correct per-competition stats",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"PadelLeague"},
	}

	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)

		pair := makePairTB(tb, app, "CSPair")
		opp := makePairTB(tb, app, "CSOpp")

		// Two competitions with different names.
		compCol, err := app.FindCollectionByNameOrId("competitions")
		require.NoError(tb, err)

		comp1 := core.NewRecord(compCol)
		comp1.Set("name", "Liga Alfa")
		comp1.Set("type", "league")
		comp1.Set("active", true)
		comp1.Set("pairs", []string{pair.Id, opp.Id})
		require.NoError(tb, app.Save(comp1))

		comp2 := core.NewRecord(compCol)
		comp2.Set("name", "Liga Beta")
		comp2.Set("type", "league")
		comp2.Set("active", true)
		comp2.Set("pairs", []string{pair.Id, opp.Id})
		require.NoError(tb, app.Save(comp2))

		matchCol, err := app.FindCollectionByNameOrId("matches")
		require.NoError(tb, err)

		// Comp1: 2 matches, pair wins both.
		for i := 0; i < 2; i++ {
			m := core.NewRecord(matchCol)
			m.Set("competition", comp1.Id)
			m.Set("pair1", pair.Id)
			m.Set("pair2", opp.Id)
			m.Set("status", "final")
			m.Set("scores", "6-3 6-4")
			m.Set("winner", pair.Id)
			m.Set("round_number", i+1)
			require.NoError(tb, app.Save(m))
		}

		// Comp2: 1 match, pair loses.
		m := core.NewRecord(matchCol)
		m.Set("competition", comp2.Id)
		m.Set("pair1", opp.Id)
		m.Set("pair2", pair.Id)
		m.Set("status", "final")
		m.Set("scores", "6-1 6-2")
		m.Set("winner", opp.Id)
		m.Set("round_number", 1)
		require.NoError(tb, app.Save(m))

		user, _ := app.FindRecordById("users", pair.GetString("player1"))
		s.URL = "/player/" + user.Id
		s.Headers = authHeaders(tb, user)
	}

	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.Contains(tb, body, "Liga Alfa", "comp1 name")
		assert.Contains(tb, body, "Liga Beta", "comp2 name")

		// Liga Alfa: 2 played, 2 wins, 0 losses.
		// Liga Beta: 1 played, 0 wins, 1 loss.
		// Template renders: <td class="text-center">N</td> for each.
		// Total played across both: 3.
		assert.Contains(tb, body, ">3</div>", "TotalPlayed should be 3")
		assert.Contains(tb, body, "67%", "WinRate: 2/3 = 67%")
	}

	s.Test(t)
}

func countOccurrences(s, sub string) int {
	count := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			count++
		}
	}
	return count
}

// --- Player profile: zero matches → winRate stays 0, no division by zero ---

func TestGen2_PlayerProfile_ZeroMatches(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "player with no matches shows 0% win rate",
		Method:         http.MethodGet,
		ExpectedStatus: 200,
	}

	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		user := makeUserTB(tb, app, "NoMatches", "")
		s.URL = "/player/" + user.Id
		s.Headers = authHeaders(tb, user)
	}

	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.Contains(tb, body, "0%", "zero matches should show 0% win rate")
		assert.Contains(tb, body, ">0</div>", "TotalPlayed should be 0")
	}

	s.Test(t)
}
