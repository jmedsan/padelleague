package middleware

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	_ "padelleague/migrations"
)

var userSeq atomic.Int64

func makeUser(t testing.TB, app core.App, role string) *core.Record {
	t.Helper()
	n := userSeq.Add(1)
	col, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Set("email", fmt.Sprintf("mwuser%d@test.local", n))
	record.Set("display_name", "MW "+role)
	record.Set("role", role)
	record.SetPassword("testpass123456")
	record.SetVerified(true)
	require.NoError(t, app.Save(record))
	return record
}

func authToken(t testing.TB, user *core.Record) string {
	t.Helper()
	token, err := user.NewAuthToken()
	require.NoError(t, err)
	return token
}


func TestRequireAppAdmin_NilAuth_Redirects(t *testing.T) {
	handlerReached := false
	s := tests.ApiScenario{
		Name:           "nil auth redirects to /",
		Method:         http.MethodGet,
		URL:            "/admin-test",
		ExpectedStatus: 302,
		BeforeTestFunc: func(_ testing.TB, _ *tests.TestApp, e *core.ServeEvent) {
			e.Router.GET("/admin-test", func(e *core.RequestEvent) error {
				handlerReached = true
				return e.String(200, "OK")
			}).BindFunc(RequireAppAdmin)
		},
		AfterTestFunc: func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
			assert.Equal(tb, "/", res.Header.Get("Location"))
			assert.False(tb, handlerReached, "handler must not be reached without auth")
		},
	}
	s.Test(t)
}

func TestRequireAppAdmin_PlayerRole_Redirects(t *testing.T) {
	handlerReached := false
	s := tests.ApiScenario{
		Name:           "player role redirects to /",
		Method:         http.MethodGet,
		URL:            "/admin-test",
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		e.Router.GET("/admin-test", func(e *core.RequestEvent) error {
			handlerReached = true
			return e.String(200, "OK")
		}).BindFunc(RequireAppAdmin)
		player := makeUser(tb, app, "player")
		s.Headers = map[string]string{"Authorization": authToken(tb, player)}
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/", res.Header.Get("Location"))
		assert.False(tb, handlerReached, "handler must not be reached for player role")
	}
	s.Test(t)
}

func TestRequireAppAdmin_AdminRole_Allowed(t *testing.T) {
	handlerReached := false
	s := tests.ApiScenario{
		Name:            "admin role passes through",
		Method:          http.MethodGet,
		URL:             "/admin-test",
		ExpectedStatus:  200,
		ExpectedContent: []string{"OK"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		e.Router.GET("/admin-test", func(e *core.RequestEvent) error {
			handlerReached = true
			return e.String(200, "OK")
		}).BindFunc(RequireAppAdmin)
		admin := makeUser(tb, app, "admin")
		s.Headers = map[string]string{"Authorization": authToken(tb, admin)}
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, _ *http.Response) {
		assert.True(tb, handlerReached, "handler must be reached for admin role")
	}
	s.Test(t)
}

func TestRequireAppAdmin_EmptyRole_Redirects(t *testing.T) {
	handlerReached := false
	s := tests.ApiScenario{
		Name:           "empty role redirects to /",
		Method:         http.MethodGet,
		URL:            "/admin-test",
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		e.Router.GET("/admin-test", func(e *core.RequestEvent) error {
			handlerReached = true
			return e.String(200, "OK")
		}).BindFunc(RequireAppAdmin)
		user := makeUser(tb, app, "player")
		token := authToken(tb, user)
		// Bypass PocketBase validation to set empty role via raw SQL.
		_, err := app.DB().NewQuery("UPDATE users SET role = '' WHERE id = {:id}").
			Bind(map[string]any{"id": user.Id}).Execute()
		require.NoError(tb, err)
		s.Headers = map[string]string{"Authorization": token}
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/", res.Header.Get("Location"))
		assert.False(tb, handlerReached, "handler must not be reached for empty role")
	}
	s.Test(t)
}

func TestRequireAppAdmin_UnexpectedRole_Redirects(t *testing.T) {
	handlerReached := false
	s := tests.ApiScenario{
		Name:           "unexpected role 'superadmin' redirects to /",
		Method:         http.MethodGet,
		URL:            "/admin-test",
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		e.Router.GET("/admin-test", func(e *core.RequestEvent) error {
			handlerReached = true
			return e.String(200, "OK")
		}).BindFunc(RequireAppAdmin)
		user := makeUser(tb, app, "player")
		token := authToken(tb, user)
		// Bypass PocketBase validation to set unexpected role via raw SQL.
		_, err := app.DB().NewQuery("UPDATE users SET role = 'superadmin' WHERE id = {:id}").
			Bind(map[string]any{"id": user.Id}).Execute()
		require.NoError(tb, err)
		s.Headers = map[string]string{"Authorization": token}
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/", res.Header.Get("Location"))
		assert.False(tb, handlerReached, "handler must not be reached for unexpected role")
	}
	s.Test(t)
}


func TestCookieAuth_CopiesCookieToHeader(t *testing.T) {
	var gotHeader, wantToken string
	s := tests.ApiScenario{
		Name:            "cookie value copied to Authorization header",
		Method:          http.MethodGet,
		URL:             "/cookie-test",
		ExpectedStatus:  200,
		ExpectedContent: []string{"OK"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		e.Router.BindFunc(CookieAuth)
		e.Router.GET("/cookie-test", func(e *core.RequestEvent) error {
			gotHeader = e.Request.Header.Get("Authorization")
			return e.String(200, "OK")
		})
		player := makeUser(tb, app, "player")
		wantToken = authToken(tb, player)
		s.Headers = map[string]string{"Cookie": "pb_auth=" + wantToken}
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, _ *http.Response) {
		assert.Equal(tb, wantToken, gotHeader, "Authorization header must equal the cookie token")
	}
	s.Test(t)
}

func TestCookieAuth_NoCookie_PassesThrough(t *testing.T) {
	handlerReached := false
	s := tests.ApiScenario{
		Name:            "no cookie still calls next",
		Method:          http.MethodGet,
		URL:             "/cookie-test",
		ExpectedStatus:  200,
		ExpectedContent: []string{"OK"},
		BeforeTestFunc: func(_ testing.TB, _ *tests.TestApp, e *core.ServeEvent) {
			e.Router.BindFunc(CookieAuth)
			e.Router.GET("/cookie-test", func(e *core.RequestEvent) error {
				handlerReached = true
				return e.String(200, "OK")
			})
		},
		AfterTestFunc: func(tb testing.TB, _ *tests.TestApp, _ *http.Response) {
			assert.True(tb, handlerReached, "handler must be reached even without cookie")
		},
	}
	s.Test(t)
}

func TestCookieAuth_SkipsAPIPath(t *testing.T) {
	var gotHeader string
	s := &tests.ApiScenario{
		Name:            "/api/ path skips cookie-to-header copy",
		Method:          http.MethodGet,
		URL:             "/api/test",
		ExpectedStatus:  200,
		ExpectedContent: []string{"OK"},
		Headers:         map[string]string{"Cookie": "pb_auth=sometoken"},
	}
	s.BeforeTestFunc = func(_ testing.TB, _ *tests.TestApp, e *core.ServeEvent) {
		e.Router.BindFunc(CookieAuth)
		e.Router.GET("/api/test", func(e *core.RequestEvent) error {
			gotHeader = e.Request.Header.Get("Authorization")
			return e.String(200, "OK")
		})
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, _ *http.Response) {
		assert.Empty(tb, gotHeader, "Authorization header must not be set for /api/ paths")
	}
	s.Test(t)
}

func TestCookieAuth_SkipsDashboardPath(t *testing.T) {
	var gotHeader string
	s := &tests.ApiScenario{
		Name:            "/_/ path skips cookie-to-header copy",
		Method:          http.MethodGet,
		URL:             "/_/test",
		ExpectedStatus:  200,
		ExpectedContent: []string{"OK"},
		Headers:         map[string]string{"Cookie": "pb_auth=sometoken"},
	}
	s.BeforeTestFunc = func(_ testing.TB, _ *tests.TestApp, e *core.ServeEvent) {
		e.Router.BindFunc(CookieAuth)
		e.Router.GET("/_/test", func(e *core.RequestEvent) error {
			gotHeader = e.Request.Header.Get("Authorization")
			return e.String(200, "OK")
		})
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, _ *http.Response) {
		assert.Empty(tb, gotHeader, "Authorization header must not be set for /_/ paths")
	}
	s.Test(t)
}

func TestCookieAuth_GarbageCookie_NoPanic(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "garbage cookie value does not panic",
		Method:          http.MethodGet,
		URL:             "/cookie-test",
		ExpectedStatus:  200,
		ExpectedContent: []string{"OK"},
		Headers:         map[string]string{"Cookie": "pb_auth=not-a-valid-token-!@#$%^&*()"},
	}
	s.BeforeTestFunc = func(_ testing.TB, _ *tests.TestApp, e *core.ServeEvent) {
		e.Router.BindFunc(CookieAuth)
		e.Router.GET("/cookie-test", func(e *core.RequestEvent) error {
			return e.String(200, "OK")
		})
	}
	s.Test(t)
}

func TestCookieAuth_EmptyCookieValue_PassesThrough(t *testing.T) {
	var gotHeader string
	s := &tests.ApiScenario{
		Name:            "empty cookie value does not set Authorization",
		Method:          http.MethodGet,
		URL:             "/cookie-test",
		ExpectedStatus:  200,
		ExpectedContent: []string{"OK"},
		Headers:         map[string]string{"Cookie": "pb_auth="},
	}
	s.BeforeTestFunc = func(_ testing.TB, _ *tests.TestApp, e *core.ServeEvent) {
		e.Router.BindFunc(CookieAuth)
		e.Router.GET("/cookie-test", func(e *core.RequestEvent) error {
			gotHeader = e.Request.Header.Get("Authorization")
			return e.String(200, "OK")
		})
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, _ *http.Response) {
		assert.Empty(tb, gotHeader, "empty cookie must not set Authorization header")
	}
	s.Test(t)
}

func TestClearAuthCookie(t *testing.T) {
	s := tests.ApiScenario{
		Name:            "ClearAuthCookie sets expired pb_auth cookie",
		Method:          http.MethodGet,
		URL:             "/clear-cookie-test",
		ExpectedStatus:  200,
		ExpectedContent: []string{"OK"},
	}
	s.BeforeTestFunc = func(_ testing.TB, _ *tests.TestApp, e *core.ServeEvent) {
		e.Router.GET("/clear-cookie-test", func(e *core.RequestEvent) error {
			ClearAuthCookie(e)
			return e.String(200, "OK")
		})
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		cookies := res.Cookies()
		require.Len(tb, cookies, 1)
		c := cookies[0]
		assert.Equal(tb, "pb_auth", c.Name)
		assert.Equal(tb, "", c.Value)
		assert.Equal(tb, "/", c.Path)
		assert.Equal(tb, -1, c.MaxAge)
		assert.True(tb, c.HttpOnly)
		assert.True(tb, c.Secure)
	}
	s.Test(t)
}
