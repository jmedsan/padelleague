package routes

import (
	"io/fs"
	"net/http"
	"testing"
	"testing/fstest"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/league"
	"padelleague/notify"
	"padelleague/render"

	_ "padelleague/migrations"
)

func minimalFS() fs.FS {
	return fstest.MapFS{
		"views/layout.html":            {Data: []byte(`{{block "content" .}}{{end}}`)},
		"views/error.html":             {Data: []byte(`{{define "content"}}{{.ErrorMessage}}{{end}}`)},
		"static/manifest.json":         {Data: []byte(`{}`)},
		"static/sw.js":                 {Data: []byte(``)},
		"views/login.html":             {Data: []byte(`{{define "content"}}login{{end}}`)},
		"views/register.html":          {Data: []byte(`{{define "content"}}register{{end}}`)},
		"views/forgot-password.html":   {Data: []byte(`{{define "content"}}forgot{{end}}`)},
		"views/reset-password.html":    {Data: []byte(`{{define "content"}}reset{{end}}`)},
		"views/home.html":              {Data: []byte(`{{define "content"}}home{{end}}`)},
		"views/admin/dashboard.html":   {Data: []byte(`{{define "content"}}admin{{end}}`)},
		"views/admin/pairs.html":       {Data: []byte(`{{define "content"}}pairs{{end}}`)},
		"views/admin/players.html":     {Data: []byte(`{{define "content"}}players{{end}}`)},
		"views/admin/invitations.html": {Data: []byte(`{{define "content"}}inv{{end}}`)},
		"views/admin/disputes.html":    {Data: []byte(`{{define "content"}}disputes{{end}}`)},
		"views/admin/venues.html":      {Data: []byte(`{{define "content"}}venues{{end}}`)},
	}
}

// adminGETPaths returns all GET paths under /admin that should require admin auth.
func adminGETPaths() []string {
	return []string{
		"/admin",
		"/admin/competitions",
		"/admin/pairs",
		"/admin/players",
		"/admin/invitations",
		"/admin/disputes",
		"/admin/venues",
	}
}

func TestAdminRoutes_RejectUnauthenticated(t *testing.T) {
	for _, path := range adminGETPaths() {
		t.Run(path, func(t *testing.T) {
			s := &tests.ApiScenario{
				Name:           "GET " + path + " without auth redirects",
				Method:         http.MethodGet,
				URL:            path,
				ExpectedStatus: 302,
			}
			s.BeforeTestFunc = func(_ testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				viewsFS := minimalFS()
				renderer := render.New(viewsFS, "")
				notifier := notify.NewNotifier(app, "", "")
				svc := league.New(app, notifier)
				Register(e, Deps{
					App:       app,
					Renderer:  renderer,
					Notifier:  notifier,
					LeagueSvc: svc,
					StaticFS:  viewsFS,
				})
			}
			s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
				loc := res.Header.Get("Location")
				assert.Equal(tb, "/login", loc, "unauthenticated request to %s should redirect to /login", path)
			}
			s.Test(t)
		})
	}
}

func TestAdminRoutes_RejectPlayer(t *testing.T) {
	for _, path := range adminGETPaths() {
		t.Run(path, func(t *testing.T) {
			s := &tests.ApiScenario{
				Name:           "GET " + path + " as player redirects",
				Method:         http.MethodGet,
				URL:            path,
				ExpectedStatus: 302,
			}
			s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				viewsFS := minimalFS()
				renderer := render.New(viewsFS, "")
				notifier := notify.NewNotifier(app, "", "")
				svc := league.New(app, notifier)
				Register(e, Deps{
					App:       app,
					Renderer:  renderer,
					Notifier:  notifier,
					LeagueSvc: svc,
					StaticFS:  viewsFS,
				})
				player := makePlayer(tb, app)
				s.Headers = authHeaders(tb, player)
			}
			s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
				loc := res.Header.Get("Location")
				assert.Equal(tb, "/", loc, "player request to %s should redirect to /", path)
			}
			s.Test(t)
		})
	}
}

// Admin-allow is NOT tested here because the handlers return mixed status codes
// (200 or 400) depending on DB state, and ApiScenario requires a fixed ExpectedStatus.
// The middleware itself is tested in middleware/auth_test.go (worker2).
// The two reject tests above prove the middleware IS attached to every admin route.

func TestPublicRoutes_RejectUnauthenticated(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "GET / without auth redirects to login",
		Method:         http.MethodGet,
		URL:            "/",
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(_ testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		viewsFS := minimalFS()
		renderer := render.New(viewsFS, "")
		notifier := notify.NewNotifier(app, "", "")
		svc := league.New(app, notifier)
		Register(e, Deps{
			App:       app,
			Renderer:  renderer,
			Notifier:  notifier,
			LeagueSvc: svc,
			StaticFS:  viewsFS,
		})
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/login", res.Header.Get("Location"))
	}
	s.Test(t)
}

func TestAuthRoutes_NoAuthRequired(t *testing.T) {
	paths := []string{"/login", "/register", "/forgot-password"}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			s := &tests.ApiScenario{
				Name:            "GET " + path + " without auth succeeds",
				Method:          http.MethodGet,
				URL:             path,
				ExpectedStatus:  200,
				ExpectedContent: []string{""}, // non-nil to skip empty-body check
			}
			s.BeforeTestFunc = func(_ testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				viewsFS := minimalFS()
				renderer := render.New(viewsFS, "")
				notifier := notify.NewNotifier(app, "", "")
				svc := league.New(app, notifier)
				Register(e, Deps{
					App:       app,
					Renderer:  renderer,
					Notifier:  notifier,
					LeagueSvc: svc,
					StaticFS:  viewsFS,
				})
			}
			s.Test(t)
		})
	}
}

func TestRequireAuth_HXRequest_RedirectsViaHeader(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "GET / as HX-Request without auth returns HX-Redirect",
		Method:         http.MethodGet,
		URL:            "/",
		ExpectedStatus: 204,
		Headers:        map[string]string{"HX-Request": "true"},
	}
	s.BeforeTestFunc = func(_ testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		viewsFS := minimalFS()
		renderer := render.New(viewsFS, "")
		notifier := notify.NewNotifier(app, "", "")
		svc := league.New(app, notifier)
		Register(e, Deps{
			App:       app,
			Renderer:  renderer,
			Notifier:  notifier,
			LeagueSvc: svc,
			StaticFS:  viewsFS,
		})
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/login", res.Header.Get("HX-Redirect"))
	}
	s.Test(t)
}

func TestStaticRoutes_ManifestJSON(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "GET /manifest.json returns JSON",
		Method:          http.MethodGet,
		URL:             "/manifest.json",
		ExpectedStatus:  200,
		ExpectedContent: []string{"{}"},
	}
	s.BeforeTestFunc = func(_ testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		viewsFS := minimalFS()
		renderer := render.New(viewsFS, "")
		notifier := notify.NewNotifier(app, "", "")
		svc := league.New(app, notifier)
		Register(e, Deps{
			App:       app,
			Renderer:  renderer,
			Notifier:  notifier,
			LeagueSvc: svc,
			StaticFS:  viewsFS,
		})
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "application/manifest+json", res.Header.Get("Content-Type"))
	}
	s.Test(t)
}

func TestStaticRoutes_ServiceWorker(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "GET /sw.js returns JS with Service-Worker-Allowed header",
		Method:         http.MethodGet,
		URL:            "/sw.js",
		ExpectedStatus: 200,
	}
	s.BeforeTestFunc = func(_ testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		viewsFS := minimalFS()
		renderer := render.New(viewsFS, "")
		notifier := notify.NewNotifier(app, "", "")
		svc := league.New(app, notifier)
		Register(e, Deps{
			App:       app,
			Renderer:  renderer,
			Notifier:  notifier,
			LeagueSvc: svc,
			StaticFS:  viewsFS,
		})
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/", res.Header.Get("Service-Worker-Allowed"))
	}
	s.Test(t)
}

func makePlayer(tb testing.TB, app core.App) *core.Record {
	tb.Helper()
	col, err := app.FindCollectionByNameOrId("users")
	require.NoError(tb, err)
	r := core.NewRecord(col)
	r.Set("email", "routeplayer@test.local")
	r.Set("username", "routeplayer")
	r.Set("display_name", "Route Player")
	r.Set("role", "player")
	r.SetPassword("testpass123456")
	r.SetVerified(true)
	require.NoError(tb, app.Save(r))
	return r
}

func authHeaders(tb testing.TB, user *core.Record) map[string]string {
	tb.Helper()
	token, err := user.NewAuthToken()
	require.NoError(tb, err)
	return map[string]string{"Authorization": token}
}
