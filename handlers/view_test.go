package handlers

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"

	"padelleague/middleware"
)

func setupViewRoute(_ testing.TB, _ *tests.TestApp, e *core.ServeEvent) {
	h := NewViewHandler()
	e.Router.GET("/view/{mode}", h.Switch).BindFunc(middleware.RequireAuth)
}

func TestViewSwitch_ValidRefererRedirectsBack(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "view switch redirects to the Referer path",
		Method:         http.MethodGet,
		URL:            "/view/player",
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupViewRoute(tb, app, e)
		user := makeUserTB(tb, app, "View Switch User", "")
		s.Headers = authHeaders(tb, user)
		s.Headers["Referer"] = "/competition/abc123"
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/competition/abc123", res.Header.Get("Location"))
	}
	s.Test(t)
}

func TestViewSwitch_ExternalRefererFallsBackToHome(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "view switch rejects an off-site Referer (open redirect)",
		Method:         http.MethodGet,
		URL:            "/view/player",
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupViewRoute(tb, app, e)
		user := makeUserTB(tb, app, "View Switch User 2", "")
		s.Headers = authHeaders(tb, user)
		s.Headers["Referer"] = "https://evil.example.com/phish"
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/", res.Header.Get("Location"))
	}
	s.Test(t)
}

func TestViewSwitch_ProtocolRelativeRefererFallsBackToHome(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "view switch rejects a protocol-relative Referer (open redirect)",
		Method:         http.MethodGet,
		URL:            "/view/player",
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupViewRoute(tb, app, e)
		user := makeUserTB(tb, app, "View Switch User 3", "")
		s.Headers = authHeaders(tb, user)
		s.Headers["Referer"] = "//evil.example.com/phish"
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/", res.Header.Get("Location"))
	}
	s.Test(t)
}
