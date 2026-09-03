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

// Player profile page: partner name, stats, win rate, streaks, comp stats

func TestGen2_PlayerProfile_FullStats(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player profile shows correct partner, stats, streak, comp stats",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Dale Fuerte a la Bola"},
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

// Player profile: date ordering of recent matches

func TestGen2_PlayerProfile_DateOrdering(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player profile recent matches sorted by date descending",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Dale Fuerte a la Bola"},
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
		febIdx := indexOf(body, "01/02/2026")
		janIdx := indexOf(body, "01/01/2026")
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

// R-237: dedup — a player on both pairs of a match should count it once

func TestGen2_PlayerProfile_DedupMultiPair(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player on both pairs counts match once",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Dale Fuerte a la Bola"},
	}

	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)

		shared := makeUserTB(tb, app, "Shared", "shared@test.local")
		other1 := makeUserTB(tb, app, "Other1", "other1@test.local")
		other2 := makeUserTB(tb, app, "Other2", "other2@test.local")

		pairCol, err := app.FindCollectionByNameOrId("pairs")
		require.NoError(tb, err)

		pairA := core.NewRecord(pairCol)
		pairA.Set("name", "PairA")
		pairA.Set("player1", shared.Id)
		pairA.Set("player2", other1.Id)
		require.NoError(tb, app.Save(pairA))

		pairB := core.NewRecord(pairCol)
		pairB.Set("name", "PairB")
		pairB.Set("player1", other2.Id)
		pairB.Set("player2", shared.Id)
		require.NoError(tb, app.Save(pairB))

		comp := makeCompetitionTB(tb, app, "league", []*core.Record{pairA, pairB})

		matchCol, err := app.FindCollectionByNameOrId("matches")
		require.NoError(tb, err)
		m := core.NewRecord(matchCol)
		m.Set("competition", comp.Id)
		m.Set("pair1", pairA.Id)
		m.Set("pair2", pairB.Id)
		m.Set("status", "final")
		m.Set("scores", "6-3 6-4")
		m.Set("winner", pairA.Id)
		m.Set("round_number", 1)
		m.Set("date", "2026-03-01")
		require.NoError(tb, app.Save(m))

		s.URL = "/player/" + shared.Id
		s.Headers = authHeaders(tb, shared)
	}

	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.Contains(tb, body, ">1</div>", "TotalPlayed should be 1 (not 2)")
		assert.Contains(tb, body, "100%", "WinRate should be 100%")
		assert.Contains(tb, body, "1V", "best streak should be 1V")
	}

	s.Test(t)
}

// Competition stats: per-comp played/wins/losses

func TestGen2_PlayerProfile_CompetitionStats(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player profile shows correct per-competition stats",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Dale Fuerte a la Bola"},
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
		assert.Contains(tb, body, ">3</div>", "TotalPlayed should be 3")
		assert.Contains(tb, body, "67%", "WinRate: 2/3 = 67%")
		// Collapse whitespace so we can match across template line breaks.
		compact := strings.Join(strings.Fields(body), " ")
		// Liga Alfa row: name, then Pos, PJ=2, PG=2, PP=0 (both pages now show
		// ShowPosition, so the row gains a Pos cell between name and PJ).
		assert.Contains(tb, compact,
			`Liga Alfa</a></td> <td class="text-center">1</td> <td class="text-center">2</td> <td class="text-center text-success">2</td> <td class="text-center text-error">0</td>`,
			"Liga Alfa row: position 1, 2 played, 2 wins, 0 losses")
		// Liga Beta row: name, then Pos, PJ=1, PG=0, PP=1.
		assert.Contains(tb, compact,
			`Liga Beta</a></td> <td class="text-center">2</td> <td class="text-center">1</td> <td class="text-center text-success">0</td> <td class="text-center text-error">1</td>`,
			"Liga Beta row: position 2, 1 played, 0 wins, 1 loss")
	}

	s.Test(t)
}

// Player profile: zero matches → winRate stays 0, no division by zero

func TestGen2_PlayerProfile_ZeroMatches(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player with no matches shows 0% win rate",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Dale Fuerte a la Bola"},
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

func TestPlayerProfileWithMatches(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /player/{id} with match history shows stats",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Stats A"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Stats A")
		p2 := makePairTB(tb, app, "Stats B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		// Create several final matches with winners for streak calculations
		for i := 0; i < 3; i++ {
			col, _ := app.FindCollectionByNameOrId("matches")
			m := core.NewRecord(col)
			m.Set("competition", comp.Id)
			m.Set("pair1", p1.Id)
			m.Set("pair2", p2.Id)
			m.Set("status", "final")
			m.Set("scores", "6-3 6-4")
			m.Set("winner", p1.Id)
			m.Set("round_number", i+1)
			require.NoError(tb, app.Save(m))
		}
		// One loss
		col, _ := app.FindCollectionByNameOrId("matches")
		m := core.NewRecord(col)
		m.Set("competition", comp.Id)
		m.Set("pair1", p1.Id)
		m.Set("pair2", p2.Id)
		m.Set("status", "final")
		m.Set("scores", "3-6 4-6")
		m.Set("winner", p2.Id)
		m.Set("round_number", 4)
		require.NoError(tb, app.Save(m))

		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.URL = "/player/" + user.Id
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestPlayerHistoryRowsHavePairLinks(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "player history rows render pairLink anchors",
		Method:         http.MethodGet,
		ExpectedStatus: 200,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Link A")
		p2 := makePairTB(tb, app, "Link B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		col, _ := app.FindCollectionByNameOrId("matches")
		m := core.NewRecord(col)
		m.Set("competition", comp.Id)
		m.Set("pair1", p1.Id)
		m.Set("pair2", p2.Id)
		m.Set("status", "final")
		m.Set("scores", "6-3 6-4")
		m.Set("winner", p1.Id)
		m.Set("round_number", 1)
		require.NoError(tb, app.Save(m))

		s.ExpectedContent = []string{
			`href="/pair/` + p1.Id + `"`,
			`href="/pair/` + p2.Id + `"`,
			"Link A",
			"Link B",
		}

		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.URL = "/player/" + user.Id
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}
