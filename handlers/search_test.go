package handlers

import (
	"net/http"
	"os"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/league"
	"padelleague/notify"
	"padelleague/render"
	"padelleague/search"
)

func setupSearchRoutes(_ testing.TB, app *tests.TestApp, e *core.ServeEvent, ix *search.Index) {
	r := render.New(os.DirFS(".."), "", true)
	svc := league.New(app, notify.NewNotifier(app, "", ""))
	h := NewSearchHandler(app, svc, ix, r.Partial)
	e.Router.GET("/search", h.Search).BindFunc(requireAuthTest)
}

func seedSearchIndex(tb testing.TB, app core.App, ix *search.Index) {
	tb.Helper()
	entries := search.Build(app)
	entries = append(entries,
		search.NewEntry(search.Entry{Label: "Admin Secret Panel", Type: "página", URL: "/admin/secret", Scope: search.Scope{Admin: true}}),
		search.NewEntry(search.Entry{Label: "Public Player Page", Type: "página", URL: "/player/1", Scope: search.Scope{Public: true}}),
	)
	ix.Replace(entries)
}

func TestSearch_PlayerExcludesAdminEntry(t *testing.T) {
	t.Parallel()
	ix := &search.Index{}
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player search excludes admin-only entries",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Public Player Page"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSearchRoutes(tb, app, e, ix)
		user := makeUserTB(tb, app, "Search Player", "")
		seedSearchIndex(tb, app, ix)
		s.URL = "/search?q=panel"
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.NotContains(t, body, "Admin Secret Panel",
			"player must never see admin-scoped entries")
	}
	s.Test(t)
}

func TestSearch_AdminSeesAdminEntry(t *testing.T) {
	t.Parallel()
	ix := &search.Index{}
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "admin search includes admin-only entries",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Admin Secret Panel"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSearchRoutes(tb, app, e, ix)
		user := makeUserTB(tb, app, "Search Admin", "")
		user.Set("roles", []string{"admin"})
		require.NoError(tb, app.Save(user))
		seedSearchIndex(tb, app, ix)
		s.URL = "/search?q=secret"
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestSearch_AdminAsPlayerExcludesAdminEntry(t *testing.T) {
	t.Parallel()
	ix := &search.Index{}
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "admin viewing as player must not see admin-only entries",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Public Player Page"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSearchRoutes(tb, app, e, ix)
		user := makeUserTB(tb, app, "Admin As Player", "")
		user.Set("roles", []string{"admin"})
		require.NoError(tb, app.Save(user))
		seedSearchIndex(tb, app, ix)
		s.URL = "/search?q=panel"
		hdrs := authHeaders(tb, user)
		hdrs["Cookie"] = "view_as=player"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.NotContains(t, body, "Admin Secret Panel",
			"admin in player-view must not see admin-scoped entries")
	}
	s.Test(t)
}

func TestSearch_QueryRecordedInHistory(t *testing.T) {
	t.Parallel()
	ix := &search.Index{}
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "search query is recorded in search_history",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Clasificación"},
	}
	var userID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSearchRoutes(tb, app, e, ix)
		user := makeUserTB(tb, app, "History User", "")
		userID = user.Id
		seedSearchIndex(tb, app, ix)
		s.URL = "/search?q=clasificacion"
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		records, err := app.FindRecordsByFilter("search_history",
			"user = {:uid} && query = 'clasificacion'", "", 0, 0,
			map[string]any{"uid": userID})
		require.NoError(tb, err)
		assert.Len(t, records, 1, "query must be recorded in search_history")
	}
	s.Test(t)
}

func TestSearch_RecentSearchesPerUser(t *testing.T) {
	t.Parallel()
	ix := &search.Index{}
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "empty search shows user's recent searches only",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"mi busqueda"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSearchRoutes(tb, app, e, ix)
		user1 := makeUserTB(tb, app, "Recent User 1", "")
		user2 := makeUserTB(tb, app, "Recent User 2", "")
		seedSearchIndex(tb, app, ix)

		col, err := app.FindCollectionByNameOrId("search_history")
		require.NoError(tb, err)

		rec1 := core.NewRecord(col)
		rec1.Set("user", user1.Id)
		rec1.Set("query", "mi busqueda")
		require.NoError(tb, app.Save(rec1))

		rec2 := core.NewRecord(col)
		rec2.Set("user", user2.Id)
		rec2.Set("query", "otra busqueda secreta")
		require.NoError(tb, app.Save(rec2))

		s.URL = "/search?q="
		s.Headers = authHeaders(tb, user1)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.NotContains(t, body, "otra busqueda secreta",
			"user1 must not see user2's recent searches")
	}
	s.Test(t)
}

func TestSearch_EmptyQueryReturnsSuggestions(t *testing.T) {
	t.Parallel()
	ix := &search.Index{}
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "empty query returns suggestions section",
		Method:          http.MethodGet,
		URL:             "/search?q=",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Búsquedas recientes"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSearchRoutes(tb, app, e, ix)
		p1 := makePairTB(tb, app, "SugA")
		p2 := makePairTB(tb, app, "SugB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		seedSearchIndex(tb, app, ix)

		col, err := app.FindCollectionByNameOrId("search_history")
		require.NoError(tb, err)
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		rec := core.NewRecord(col)
		rec.Set("user", user.Id)
		rec.Set("query", "test query")
		require.NoError(tb, app.Save(rec))

		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestSearch_PlayerExcludesForeignCompEntry(t *testing.T) {
	t.Parallel()
	ix := &search.Index{}
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player search excludes foreign competition entries",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"My Thread Message"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSearchRoutes(tb, app, e, ix)
		p1 := makePairTB(tb, app, "MyComp Player")
		p2 := makePairTB(tb, app, "OtherComp Player")
		comp1 := makeCompetitionTB(tb, app, "league", []*core.Record{p1})
		comp2 := makeCompetitionTB(tb, app, "league", []*core.Record{p2})

		entries := []search.Entry{
			search.NewEntry(search.Entry{Label: "My Thread Message", Type: "mensaje", URL: "/match/1", Scope: search.Scope{CompID: comp1.Id}}),
			search.NewEntry(search.Entry{Label: "Foreign Thread Message", Type: "mensaje", URL: "/match/2", Scope: search.Scope{CompID: comp2.Id}}),
		}
		ix.Replace(entries)

		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.URL = "/search?q=thread"
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.Contains(t, body, "My Thread Message")
		assert.NotContains(t, body, "Foreign Thread Message",
			"player must not see entries from competitions they're not in")
	}
	s.Test(t)
}

func TestSearch_NoResults(t *testing.T) {
	t.Parallel()
	ix := &search.Index{}
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "search with no matches shows empty state",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"No se encontraron resultados"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSearchRoutes(tb, app, e, ix)
		user := makeUserTB(tb, app, "NoResult User", "")
		ix.Replace([]search.Entry{})
		s.URL = "/search?q=xyznonexistent"
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestSearch_ZeroQueryNeverBlank(t *testing.T) {
	t.Parallel()
	ix := &search.Index{}
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "empty query for fresh user shows quick-nav (never blank)",
		Method:          http.MethodGet,
		URL:             "/search?q=",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Ir a", "Inicio", "Mi perfil"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSearchRoutes(tb, app, e, ix)
		user := makeUserTB(tb, app, "Fresh User", "")
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestSearch_ZeroQueryPlayerTask(t *testing.T) {
	t.Parallel()
	ix := &search.Index{}
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "empty query for player with disputed match shows obligation link",
		Method:          http.MethodGet,
		URL:             "/search?q=",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Tu próxima acción", "/match/", "Disputa abierta"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSearchRoutes(tb, app, e, ix)
		p1 := makePairTB(tb, app, "TaskA")
		p2 := makePairTB(tb, app, "TaskB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestSearch_ZeroQueryAdminAlertAndSetup(t *testing.T) {
	t.Parallel()
	ix := &search.Index{}
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "empty query for admin shows alert + setup links",
		Method:          http.MethodGet,
		URL:             "/search?q=",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Tu próxima acción", "Disputas"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSearchRoutes(tb, app, e, ix)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "AlertA")
		p2 := makePairTB(tb, app, "AlertB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestSearch_ZeroQueryObligationsAreLinks(t *testing.T) {
	t.Parallel()
	ix := &search.Index{}
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "obligation rows are <a> links, not plain <div>",
		Method:          http.MethodGet,
		URL:             "/search?q=",
		ExpectedStatus:  200,
		ExpectedContent: []string{`<a href="/match/`},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSearchRoutes(tb, app, e, ix)
		p1 := makePairTB(tb, app, "LinkA")
		p2 := makePairTB(tb, app, "LinkB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}
