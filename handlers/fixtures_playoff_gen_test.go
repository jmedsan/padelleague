package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Helper: create a playoff competition with optional seeding ---

func makePlayoffComp(tb testing.TB, app core.App, pairs []*core.Record, seeding map[string]int) *core.Record {
	tb.Helper()
	comp := makeCompetitionTB(tb, app, "playoff", pairs)
	if seeding != nil {
		raw, _ := json.Marshal(seeding)
		comp.Set("seeding", string(raw))
		require.NoError(tb, app.Save(comp))
	}
	return comp
}

// firstRoundMatches returns the round-1 matches as [pair1, pair2] tuples.
func firstRoundMatches(tb testing.TB, app core.App, compID string) [][2]string {
	tb.Helper()
	matches, err := app.FindRecordsByFilter("matches",
		"competition = {:id} && round_number = 1", "", 0, 0,
		map[string]any{"id": compID})
	require.NoError(tb, err)
	result := make([][2]string, len(matches))
	for i, m := range matches {
		result[i] = [2]string{m.GetString("pair1"), m.GetString("pair2")}
	}
	return result
}

// hasMatchup returns true if any match has pair1=a,pair2=b.
func hasMatchup(matches [][2]string, a, b string) bool {
	for _, m := range matches {
		if m[0] == a && m[1] == b {
			return true
		}
	}
	return false
}

// --- All unseeded: order matches input order ---

func TestPlayoffAllUnseeded_KeepsInputOrder(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "playoff all unseeded keeps input order",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var pairIDs []string
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "Uns A")
		p2 := makePairTB(tb, app, "Uns B")
		p3 := makePairTB(tb, app, "Uns C")
		p4 := makePairTB(tb, app, "Uns D")
		pairs := []*core.Record{p1, p2, p3, p4}
		pairIDs = []string{p1.Id, p2.Id, p3.Id, p4.Id}
		comp := makePlayoffComp(tb, app, pairs, nil)
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/generate"
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		// bracketSize=4: slots=[p1,p2,p3,p4] → matches: slot[0] vs slot[3], slot[1] vs slot[2]
		// With no seeding, input order preserved: p1 vs p4, p2 vs p3
		matches := firstRoundMatches(tb, app, compID)
		require.Len(tb, matches, 2)
		// Unseeded: slots=[p1,p2,p3,p4] → p1 vs p4, p2 vs p3
		assert.True(tb, hasMatchup(matches, pairIDs[0], pairIDs[3]),
			"expected p1 vs p4")
		assert.True(tb, hasMatchup(matches, pairIDs[1], pairIDs[2]),
			"expected p2 vs p3")
	}
	s.Test(t)
}

// --- All seeded: strict seed order ---

func TestPlayoffAllSeeded_SeedOrder(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "playoff all seeded sorts by seed",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var pairIDs []string
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "Sd A")
		p2 := makePairTB(tb, app, "Sd B")
		p3 := makePairTB(tb, app, "Sd C")
		p4 := makePairTB(tb, app, "Sd D")
		pairs := []*core.Record{p1, p2, p3, p4}
		pairIDs = []string{p1.Id, p2.Id, p3.Id, p4.Id}
		// Seeds out of sequence: p1=3, p2=1, p3=4, p4=2
		seeding := map[string]int{
			p1.Id: 3,
			p2.Id: 1,
			p3.Id: 4,
			p4.Id: 2,
		}
		comp := makePlayoffComp(tb, app, pairs, seeding)
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/generate"
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		// After sorting: seed1=p2, seed2=p4, seed3=p1, seed4=p3
		// bracketSize=4: slots=[p2, p4, p1, p3]
		// Matches: slot[0] vs slot[3] = p2 vs p3, slot[1] vs slot[2] = p4 vs p1
		matches := firstRoundMatches(tb, app, compID)
		require.Len(tb, matches, 2)
		// Seed 1 (p2) vs seed 4 (p3), and seed 2 (p4) vs seed 3 (p1)
		assert.True(tb, hasMatchup(matches, pairIDs[1], pairIDs[2]),
			"expected seed1(p2) vs seed4(p3)")
		assert.True(tb, hasMatchup(matches, pairIDs[3], pairIDs[0]),
			"expected seed2(p4) vs seed3(p1)")
	}
	s.Test(t)
}

// --- Mixed seeded/unseeded: seeded first, then unseeded ---

func TestPlayoffMixedSeeding_SeededFirst(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "playoff seeded pairs outrank unseeded",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var pairIDs []string
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "Mix A") // unseeded
		p2 := makePairTB(tb, app, "Mix B") // seed 2
		p3 := makePairTB(tb, app, "Mix C") // unseeded
		p4 := makePairTB(tb, app, "Mix D") // seed 1
		pairs := []*core.Record{p1, p2, p3, p4}
		pairIDs = []string{p1.Id, p2.Id, p3.Id, p4.Id}
		seeding := map[string]int{
			p2.Id: 2,
			p4.Id: 1,
		}
		comp := makePlayoffComp(tb, app, pairs, seeding)
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/generate"
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		// Sort: seed1=p4, seed2=p2, then unseeded in input order: p1, p3
		// slots=[p4, p2, p1, p3]
		// Matches: slot[0] vs slot[3] = p4 vs p3, slot[1] vs slot[2] = p2 vs p1
		matches := firstRoundMatches(tb, app, compID)
		require.Len(tb, matches, 2)
		// Seed 1 (p4) vs unseeded (p3), seed 2 (p2) vs unseeded (p1)
		assert.True(tb, hasMatchup(matches, pairIDs[3], pairIDs[2]),
			"expected seed1(p4) vs unseeded(p3)")
		assert.True(tb, hasMatchup(matches, pairIDs[1], pairIDs[0]),
			"expected seed2(p2) vs unseeded(p1)")
	}
	s.Test(t)
}

// --- Advancer pairing: 8 pairs → 4 first-round matches + 2 second-round + 1 final ---

func TestPlayoffAdvancerPairing_LaterRoundsExist(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "playoff 8 pairs creates correct bracket structure",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		pairs := make([]*core.Record, 8)
		for i := range pairs {
			pairs[i] = makePairTB(tb, app, fmt.Sprintf("Brk %c", 'A'+i))
		}
		comp := makePlayoffComp(tb, app, pairs, nil)
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/generate"
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		for round, expected := range map[int]int{1: 4, 2: 2, 3: 1} {
			matches, err := app.FindRecordsByFilter("matches",
				"competition = {:id} && round_number = {:r}", "", 0, 0,
				map[string]any{"id": compID, "r": round})
			require.NoError(tb, err)
			assert.Len(tb, matches, expected, "round %d should have %d matches", round, expected)
		}
	}
	s.Test(t)
}

// --- Fewer than 2 pairs guard ---

func TestPlayoffFewerThan2Pairs(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "playoff fewer than 2 pairs returns error",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Se necesitan al menos 2 parejas"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "Solo")
		comp := makePlayoffComp(tb, app, []*core.Record{p1}, nil)
		s.URL = "/admin/competitions/" + comp.Id + "/generate"
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}
