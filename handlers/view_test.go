package handlers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"

	"padelleague/middleware"
)

func setupViewRoute(_ *tests.TestApp, e *core.ServeEvent) {
	e.Router.BindFunc(middleware.CookieAuth)
	view := NewViewHandler()
	e.Router.GET("/view/{mode}", view.Switch).BindFunc(requireAuthTest)
}

func TestViewSwitchSetsCookie(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "GET /view/player sets the view_as cookie and redirects",
		Method:         http.MethodGet,
		URL:            "/view/player",
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupViewRoute(app, e)
		s.Headers = authHeaders(tb, makeAdminUser(tb, app))
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		assert.Contains(tb, res.Header.Get("Set-Cookie"), "view_as=player")
	}
	s.Test(t)
}

func TestViewSwitchRejectsUnknownMode(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "GET /view/bogus falls back to admin",
		Method:         http.MethodGet,
		URL:            "/view/bogus",
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupViewRoute(app, e)
		s.Headers = authHeaders(tb, makeAdminUser(tb, app))
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		cookie := res.Header.Get("Set-Cookie")
		assert.True(tb, strings.Contains(cookie, "view_as=admin"), "unknown mode defaults to admin, got %q", cookie)
	}
	s.Test(t)
}
