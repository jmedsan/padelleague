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

func TestPairPage_ShowsPlayersAndCompetition(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "pair page shows players, competition with position, and matches",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Padel League"},
	}

	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)

		u1 := makeUserTB(tb, app, "PairP1", "pairp1@test.local")
		u2 := makeUserTB(tb, app, "PairP2", "pairp2@test.local")

		col, err := app.FindCollectionByNameOrId("pairs")
		require.NoError(tb, err)
		pair := core.NewRecord(col)
		pair.Set("name", "TestPair")
		pair.Set("player1", u1.Id)
		pair.Set("player2", u2.Id)
		require.NoError(tb, app.Save(pair))

		opp := makePairTB(tb, app, "OppPair")

		compCol, err := app.FindCollectionByNameOrId("competitions")
		require.NoError(tb, err)
		comp := core.NewRecord(compCol)
		comp.Set("name", "Pair Test Liga")
		comp.Set("type", "league")
		comp.Set("active", true)
		comp.Set("pairs", []string{pair.Id, opp.Id})
		require.NoError(tb, app.Save(comp))

		matchCol, err := app.FindCollectionByNameOrId("matches")
		require.NoError(tb, err)
		m := core.NewRecord(matchCol)
		m.Set("competition", comp.Id)
		m.Set("pair1", pair.Id)
		m.Set("pair2", opp.Id)
		m.Set("status", "final")
		m.Set("scores", "6-2 6-3")
		m.Set("winner", pair.Id)
		m.Set("round_number", 1)
		require.NoError(tb, app.Save(m))

		s.URL = "/pair/" + pair.Id
		user1, _ := app.FindRecordById("users", u1.Id)
		s.Headers = authHeaders(tb, user1)

		s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
			body := readBody(tb, res)
			compact := strings.Join(strings.Fields(body), " ")

			assert.Contains(tb, body, "TestPair", "pair name heading")
			assert.Contains(tb, body, "/player/"+u1.Id, "player 1 link")
			assert.Contains(tb, body, "/player/"+u2.Id, "player 2 link")
			assert.Contains(tb, body, "PairP1", "player 1 name")
			assert.Contains(tb, body, "PairP2", "player 2 name")
			assert.Contains(tb, compact, "/competition/"+comp.Id, "competition link")
			assert.Contains(tb, compact, "Pair Test Liga", "competition name")
			assert.Contains(tb, compact, "6-2 6-3", "match score in recent")
		}
	}

	s.Test(t)
}

func TestPairPage_NotFound(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "pair page returns 404 for unknown pair",
		Method:          http.MethodGet,
		URL:             "/pair/nonexistent",
		ExpectedStatus:  404,
		ExpectedContent: []string{"no encontrada"},
	}

	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		u := makeUserTB(tb, app, "Viewer", "viewer@test.local")
		user, _ := app.FindRecordById("users", u.Id)
		s.Headers = authHeaders(tb, user)
	}

	s.Test(t)
}
