package handlers_test

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/handlers"
)

func TestBackupNow_Succeeds(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/health/backup triggers an immediate backup",
		Method:         http.MethodPost,
		URL:            "/admin/health/backup",
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		settings := app.Settings()
		settings.Backups.Cron = "0 */6 * * *"
		settings.Backups.CronMaxKeep = 2
		require.NoError(tb, app.Save(settings))
		admin := handlers.MakeAdminUserTB(tb, app)
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	handlers.ExpectRedirect(s, redirectTo("/admin/health"))
	s.Test(t)
}

func TestBackupNow_RefusesWhenNotConfigured(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /admin/health/backup refuses when backups are not configured",
		Method:          http.MethodPost,
		URL:             "/admin/health/backup",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Backup no configurado"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := handlers.MakeAdminUserTB(tb, app)
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.Test(t)
}

func TestHealth_DisputeShowsLink(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "health dashboard shows disputed match link",
		Method:          http.MethodGet,
		URL:             "/admin/health",
		ExpectedStatus:  200,
		ExpectedContent: []string{"/match/"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := handlers.MakeAdminUserTB(tb, app)
		p1 := handlers.MakePairTB(tb, app, "HealthA")
		p2 := handlers.MakePairTB(tb, app, "HealthB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.Test(t)
}

func TestHealth_AllEmptyShowsSinIncidencias(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:     handlers.TestAppFactory,
		Name:               "health dashboard with nothing pending shows Sin incidencias, not empty category cards",
		Method:             http.MethodGet,
		URL:                "/admin/health",
		ExpectedStatus:     200,
		ExpectedContent:    []string{"Sin incidencias — todo en orden"},
		NotExpectedContent: []string{"card-title text-lg"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := handlers.MakeAdminUserTB(tb, app)
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.Test(t)
}

func TestHealth_MixedShowsOnlyNonEmptyCategories(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:     handlers.TestAppFactory,
		Name:               "health dashboard with one issue hides the other empty category cards",
		Method:             http.MethodGet,
		URL:                "/admin/health",
		ExpectedStatus:     200,
		ExpectedContent:    []string{"card-title text-lg"},
		NotExpectedContent: []string{"Sin incidencias — todo en orden", "Sin pagar"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := handlers.MakeAdminUserTB(tb, app)
		p1 := handlers.MakePairTB(tb, app, "HealthEmptyA")
		p2 := handlers.MakePairTB(tb, app, "HealthEmptyB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("payment_status", map[string]any{p1.Id: true, p2.Id: true})
		require.NoError(tb, app.Save(comp))
		handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.Test(t)
}

func TestHealth_NonAdminDenied(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "non-admin cannot access health dashboard",
		Method:         http.MethodGet,
		URL:            "/admin/health",
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		user := handlers.MakeUserTB(tb, app, "Regular Player", "")
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(_ testing.TB, _ *tests.TestApp, res *http.Response) {
		loc := res.Header.Get("Location")
		// Real middleware.RequireAppAdmin redirects non-admins to "/" (home),
		// not "/login" — the test stand-in it replaced redirected to /login,
		// which was never what production does for an authenticated non-admin.
		assert.Equal(t, "/", loc, "non-admin should be redirected home")
	}
	s.Test(t)
}
