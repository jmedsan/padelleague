package handlers

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/filesystem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/middleware"
	"padelleague/render"
)

func setupSettingsRoutes(_ testing.TB, app *tests.TestApp, e *core.ServeEvent) {
	viewsFS := os.DirFS("..")
	r := render.New(viewsFS, "", true)

	e.Router.BindFunc(middleware.CookieAuth)

	auth := NewAuthHandler(app, r.Page)
	e.Router.GET("/login", auth.Login)

	settings := NewAdminSettingsHandler(app, r.Page)

	g := e.Router.Group("/admin")
	g.BindFunc(requireAuthTest)
	g.BindFunc(requireAdminTest)
	g.GET("/settings", settings.Settings)
	g.POST("/settings/defaults", settings.SaveDefaults)
	g.POST("/settings/branding", settings.SaveBranding)
	g.POST("/settings/logo", settings.SettingsLogoUpload)
	g.POST("/settings/logo/delete", settings.SettingsLogoDelete)
}

func TestSettingsGET_ShowsDefaultsForm(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /admin/settings prefills the defaults form from app_settings",
		Method:          http.MethodGet,
		URL:             "/admin/settings",
		ExpectedStatus:  200,
		ExpectedContent: []string{`name="quorum_timeout_hours" value="48"`, `name="walkover_score" value="6-0 6-0"`},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSettingsRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestSaveDefaults_UpdatesAppSettings(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/settings/defaults updates the app_settings singleton",
		Method:          http.MethodPost,
		URL:             "/admin/settings/defaults",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Configuración guardada"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSettingsRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Body = strings.NewReader("gender_type=mixed&quorum_timeout_hours=72&arrange_grace_days=5&walkover_score=6-1+6-1&default_penalty=4&recovery_days=20&invite_max_uses=15&invite_expiration_days=10")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		records, err := app.FindRecordsByFilter("app_settings", "", "", 1, 0)
		require.NoError(tb, err)
		require.Len(tb, records, 1)
		rec := records[0]
		assert.Equal(tb, "mixed", rec.GetString("gender_type"))
		assert.Equal(tb, 72.0, rec.GetFloat("quorum_timeout_hours"))
		assert.Equal(tb, 5.0, rec.GetFloat("arrange_grace_days"))
		assert.Equal(tb, "6-1 6-1", rec.GetString("walkover_score"))
		assert.Equal(tb, 4.0, rec.GetFloat("default_penalty"))
		assert.Equal(tb, 20.0, rec.GetFloat("recovery_days"))
		assert.Equal(tb, 15.0, rec.GetFloat("invite_max_uses"))
		assert.Equal(tb, 10.0, rec.GetFloat("invite_expiration_days"))
	}
	s.Test(t)
}

func TestSaveDefaults_InvalidWalkoverScoreRejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/settings/defaults rejects an invalid walkover score",
		Method:          http.MethodPost,
		URL:             "/admin/settings/defaults",
		ExpectedStatus:  200,
		ExpectedContent: []string{"no válido"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSettingsRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Body = strings.NewReader("gender_type=free&quorum_timeout_hours=48&arrange_grace_days=3&walkover_score=bogus&default_penalty=3&recovery_days=14&invite_max_uses=10&invite_expiration_days=7")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		records, err := app.FindRecordsByFilter("app_settings", "", "", 1, 0)
		require.NoError(tb, err)
		require.Len(tb, records, 1)
		assert.Equal(tb, "6-0 6-0", records[0].GetString("walkover_score"), "invalid save must not persist")
	}
	s.Test(t)
}

func TestSettingsLogoDelete_ClearsLogoAndRedirects(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/settings/logo/delete clears the league logo and redirects",
		Method:         http.MethodPost,
		URL:            "/admin/settings/logo/delete",
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSettingsRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Headers = authHeaders(tb, admin)

		records, err := app.FindRecordsByFilter("app_settings", "", "", 1, 0)
		require.NoError(tb, err)
		require.Len(tb, records, 1)
		rec := records[0]
		f, err := filesystem.NewFileFromBytes([]byte("fake-league-logo-bytes"), "league_logo.png")
		require.NoError(tb, err)
		rec.Set("league_logo", f)
		require.NoError(tb, app.Save(rec))
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/admin/settings", res.Header.Get("HX-Redirect"))
		records, err := app.FindRecordsByFilter("app_settings", "", "", 1, 0)
		require.NoError(tb, err)
		require.Len(tb, records, 1)
		assert.Empty(tb, records[0].GetString("league_logo"))
	}
	s.Test(t)
}
