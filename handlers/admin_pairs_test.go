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

func TestAdminPairsPage(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /admin/pairs returns pair list",
		Method:          http.MethodGet,
		URL:             "/admin/pairs",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Parejas"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupFullAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestAdminPairsSortedByName(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /admin/pairs renders pairs in alphabetical order",
		Method:          http.MethodGet,
		URL:             "/admin/pairs",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Parejas"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupFullAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		makePairTB(tb, app, "Zorros")
		makePairTB(tb, app, "Aguilas")
		makePairTB(tb, app, "Lobos")
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		idxA := strings.Index(body, "Aguilas")
		idxL := strings.Index(body, "Lobos")
		idxZ := strings.Index(body, "Zorros")
		require.NotEqual(tb, -1, idxA, "Aguilas should appear in response")
		require.NotEqual(tb, -1, idxL, "Lobos should appear in response")
		require.NotEqual(tb, -1, idxZ, "Zorros should appear in response")
		assert.Less(tb, idxA, idxL, "Aguilas should appear before Lobos")
		assert.Less(tb, idxL, idxZ, "Lobos should appear before Zorros")
	}
	s.Test(t)
}

func TestAdminPairsCreate(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/pairs creates pair",
		Method:         http.MethodPost,
		URL:            "/admin/pairs",
		ExpectedStatus: 204,
	}
	var u1ID, u2ID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupFullAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		u1 := makeUserTB(tb, app, "PairP1", "")
		u2 := makeUserTB(tb, app, "PairP2", "")
		u1ID, u2ID = u1.Id, u2.Id
		s.Body = strings.NewReader("name=NuevaPair&player1=" + u1.Id + "&player2=" + u2.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		pairs, err := app.FindRecordsByFilter("pairs", "name = 'NuevaPair'", "", 0, 0, nil)
		require.NoError(tb, err)
		require.Equal(tb, 1, len(pairs))
		assert.Equal(tb, u1ID, pairs[0].GetString("player1"))
		assert.Equal(tb, u2ID, pairs[0].GetString("player2"))
	}
	s.Test(t)
}

func TestAdminPairsUpdate(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/pairs/{id} updates pair name",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var pairID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupFullAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		pair := makePairTB(tb, app, "UpdPair")
		pairID = pair.Id
		s.URL = "/admin/pairs/" + pair.Id
		s.Body = strings.NewReader("name=Renamed")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		p, err := app.FindRecordById("pairs", pairID)
		require.NoError(tb, err)
		assert.Equal(tb, "Renamed", p.GetString("name"))
	}
	s.Test(t)
}
