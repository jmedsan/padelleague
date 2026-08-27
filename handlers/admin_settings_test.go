package handlers

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/middleware"
	"padelleague/render"
)

func setupSettingsRoutes(_ testing.TB, app *tests.TestApp, e *core.ServeEvent, devTools bool) {
	viewsFS := os.DirFS("..")
	r := render.New(viewsFS, "")

	e.Router.BindFunc(middleware.CookieAuth)

	auth := NewAuthHandler(app, r.Page)
	e.Router.GET("/login", auth.Login)

	settings := NewAdminSettingsHandler(app, devTools, r.Page)

	g := e.Router.Group("/admin")
	g.BindFunc(requireAuthTest)
	g.BindFunc(requireAdminTest)
	g.GET("/settings", settings.Settings)
	g.POST("/settings/reset", settings.Reset)
}

func TestSettingsGET(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /admin/settings as admin returns 200",
		Method:          http.MethodGet,
		URL:             "/admin/settings",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Reiniciar base de datos", "Datos de ejemplo a cargar", "reset-overlay", "Reiniciando la base de datos"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSettingsRoutes(tb, app, e, true)
		admin := makeAdminUser(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestSettingsGET_DevToolsFalse(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:     testAppFactory,
		Name:               "GET /admin/settings with devTools=false hides reset form",
		Method:             http.MethodGet,
		URL:                "/admin/settings",
		ExpectedStatus:     200,
		ExpectedContent:    []string{"No hay opciones"},
		NotExpectedContent: []string{"Reiniciar base de datos"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSettingsRoutes(tb, app, e, false)
		admin := makeAdminUser(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestResetWrongConfirm(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST reset with wrong confirm leaves data unchanged",
		Method:          http.MethodPost,
		URL:             "/admin/settings/reset",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Escribe DELETE"},
	}
	var playerID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSettingsRoutes(tb, app, e, true)
		admin := makeAdminUser(tb, app)
		player := makeUserTB(tb, app, "Player", "")
		playerID = player.Id
		s.Body = strings.NewReader("confirm=WRONG&players=on")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		_, err := app.FindRecordById("users", playerID)
		assert.NoError(tb, err, "player should still exist")
		assert.Contains(tb, res.Header.Get("Content-Type"), "text/html")
	}
	s.Test(t)
}

func TestResetFromScratch(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST reset with no example checkboxes wipes to a clean DB, admins survive",
		Method:          http.MethodPost,
		URL:             "/admin/settings/reset",
		ExpectedStatus:  200,
		ExpectedContent: []string{"reiniciada", "vacía"},
	}
	var admin1ID, admin2ID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSettingsRoutes(tb, app, e, true)
		admin1 := makeAdminUser(tb, app)
		admin2 := makeAdminUser(tb, app)
		admin1ID = admin1.Id
		admin2ID = admin2.Id
		makeUserTB(tb, app, "Player1", "")
		makePairTB(tb, app, "TestPair")
		s.Body = strings.NewReader("confirm=DELETE")
		hdrs := authHeaders(tb, admin1)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		_, err := app.FindRecordById("users", admin1ID)
		assert.NoError(tb, err, "admin1 should survive")
		_, err = app.FindRecordById("users", admin2ID)
		assert.NoError(tb, err, "admin2 should survive")

		players, err := app.FindRecordsByFilter("users", "roles ~ 'player'", "", 0, 0)
		require.NoError(tb, err)
		assert.Empty(tb, players, "players should be wiped and none loaded")

		pairs, err := app.FindRecordsByFilter("pairs", "id != ''", "", 0, 0)
		require.NoError(tb, err)
		assert.Empty(tb, pairs, "pairs should be wiped and none loaded")

		comps, err := app.FindRecordsByFilter("competitions", "id != ''", "", 0, 0)
		require.NoError(tb, err)
		assert.Empty(tb, comps, "competitions should be wiped and none loaded")
	}
	s.Test(t)
}

func TestResetLoadPlayersOnly(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST reset players=on wipes all then loads only sample players",
		Method:          http.MethodPost,
		URL:             "/admin/settings/reset",
		ExpectedStatus:  200,
		ExpectedContent: []string{"reiniciada", "ejemplo"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSettingsRoutes(tb, app, e, true)
		admin := makeAdminUser(tb, app)
		makeUserTB(tb, app, "OldPlayer", "")
		s.Body = strings.NewReader("confirm=DELETE&players=on")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		players, err := app.FindRecordsByFilter("users", "roles ~ 'player'", "", 0, 0)
		require.NoError(tb, err)
		assert.Len(tb, players, 8, "the 8 sample players should be loaded (old ones wiped)")

		pairs, err := app.FindRecordsByFilter("pairs", "id != ''", "", 0, 0)
		require.NoError(tb, err)
		assert.Empty(tb, pairs, "no pairs loaded when only players selected")

		comps, err := app.FindRecordsByFilter("competitions", "id != ''", "", 0, 0)
		require.NoError(tb, err)
		assert.Empty(tb, comps, "no competition loaded when only players selected")
	}
	s.Test(t)
}

func TestResetLoadFullSample(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST reset with all example categories loads the full sample league",
		Method:          http.MethodPost,
		URL:             "/admin/settings/reset",
		ExpectedStatus:  200,
		ExpectedContent: []string{"reiniciada", "ejemplo"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSettingsRoutes(tb, app, e, true)
		admin := makeAdminUser(tb, app)
		s.Body = strings.NewReader("confirm=DELETE&players=on&pairs=on&competitions=on&matches=on")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		comps, err := app.FindRecordsByFilter("competitions", "name = 'Liga de ejemplo'", "", 0, 0)
		require.NoError(tb, err)
		assert.Len(tb, comps, 1, "sample competition should exist")

		players, err := app.FindRecordsByFilter("users", "roles ~ 'player'", "", 0, 0)
		require.NoError(tb, err)
		assert.Len(tb, players, 8, "should have 8 sample players")

		finals, err := app.FindRecordsByFilter("matches", "status = 'final'", "", 0, 0)
		require.NoError(tb, err)
		assert.NotEmpty(tb, finals, "rounds 1-4 should be played (matches selected)")
	}
	s.Test(t)
}

func TestResetLoadCompetitionNotPlayed(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST reset competitions=on matches=off loads the competition with no played matches",
		Method:          http.MethodPost,
		URL:             "/admin/settings/reset",
		ExpectedStatus:  200,
		ExpectedContent: []string{"reiniciada", "ejemplo"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSettingsRoutes(tb, app, e, true)
		admin := makeAdminUser(tb, app)
		s.Body = strings.NewReader("confirm=DELETE&players=on&pairs=on&competitions=on")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		comps, err := app.FindRecordsByFilter("competitions", "id != ''", "", 0, 0)
		require.NoError(tb, err)
		assert.Len(tb, comps, 1, "the sample competition should be loaded")

		matches, err := app.FindRecordsByFilter("matches", "id != ''", "", 0, 0)
		require.NoError(tb, err)
		assert.NotEmpty(tb, matches, "rounds/fixtures should exist")

		finals, err := app.FindRecordsByFilter("matches", "status = 'final'", "", 0, 0)
		require.NoError(tb, err)
		assert.Empty(tb, finals, "no matches played when 'matches' not selected")
	}
	s.Test(t)
}

func TestResetDevToolsFalseRejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST reset with devTools=false is rejected",
		Method:          http.MethodPost,
		URL:             "/admin/settings/reset",
		ExpectedStatus:  200,
		ExpectedContent: []string{"No disponible"},
	}
	var playerID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSettingsRoutes(tb, app, e, false)
		admin := makeAdminUser(tb, app)
		player := makeUserTB(tb, app, "Player", "")
		playerID = player.Id
		s.Body = strings.NewReader("confirm=DELETE&players=on&pairs=on&competitions=on&matches=on")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		_, err := app.FindRecordById("users", playerID)
		assert.NoError(tb, err, "player should still exist when devTools=false")
	}
	s.Test(t)
}
