package handlers

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"

	"padelleague/league"
	"padelleague/middleware"
	"padelleague/notify"
	"padelleague/render"
)

func setupAdminRoutes(_ testing.TB, app *tests.TestApp, e *core.ServeEvent) {
	viewsFS := os.DirFS("..")
	r := render.New(viewsFS, "")
	notifier := notify.NewNotifier(app, "", "")
	svc := league.New(app, notifier)

	e.Router.BindFunc(middleware.CookieAuth)

	auth := NewAuthHandler(app, r.Page)
	e.Router.GET("/login", auth.Login)

	admin := NewAdminHandler(app, notifier, r.Page)
	comp := NewCompetitionHandler(app, svc, r.Page)

	g := e.Router.Group("/admin")
	g.BindFunc(requireAuthTest)
	g.BindFunc(requireAdminTest)
	g.GET("", comp.Dashboard)
	g.GET("/competitions/{id}", comp.Detail)
	g.POST("/competitions", comp.Create)
	g.GET("/players", admin.Players)
	g.GET("/invitations", admin.InvitationsList)
	g.POST("/invitations", admin.InvitationsCreate)
	g.GET("/venues", admin.Venues)
}

func makeAdminUser(t testing.TB, app core.App) *core.Record {
	t.Helper()
	n := userSeq.Add(1)
	col, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Set("email", fmt.Sprintf("admin%d@test.local", n))
	record.Set("display_name", "Test Admin")
	record.Set("role", "admin")
	record.SetPassword("testpass123456")
	record.SetVerified(true)
	require.NoError(t, app.Save(record))
	return record
}

func TestAdminNoAuth(t *testing.T) {
	scenario := tests.ApiScenario{
		Name:           "GET /admin without auth redirects to login",
		Method:         http.MethodGet,
		URL:            "/admin",
		ExpectedStatus: 302,
		BeforeTestFunc: setupAdminRoutes,
	}
	scenario.Test(t)
}

func TestAdminPlayerAuth(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "GET /admin with player auth redirects",
		Method:         http.MethodGet,
		URL:            "/admin",
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		player := makeUserTB(tb, app, "Player", "")
		s.Headers = authHeaders(tb, player)
	}
	s.Test(t)
}

func TestAdminWithAdminAuth(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "GET /admin with admin auth returns 200",
		Method:          http.MethodGet,
		URL:             "/admin",
		ExpectedStatus:  200,
		ExpectedContent: []string{"PadelLeague"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestAdminPlayersPage(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "GET /admin/players with admin auth returns 200",
		Method:          http.MethodGet,
		URL:             "/admin/players",
		ExpectedStatus:  200,
		ExpectedContent: []string{"PadelLeague"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestAdminInvitationsPage(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "GET /admin/invitations with admin auth returns 200",
		Method:          http.MethodGet,
		URL:             "/admin/invitations",
		ExpectedStatus:  200,
		ExpectedContent: []string{"PadelLeague"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestAdminVenuesPage(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "GET /admin/venues with admin auth returns 200",
		Method:          http.MethodGet,
		URL:             "/admin/venues",
		ExpectedStatus:  200,
		ExpectedContent: []string{"PadelLeague"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestAdminCreateCompetition(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /admin/competitions creates and redirects",
		Method:         http.MethodPost,
		URL:            "/admin/competitions",
		Body:           strings.NewReader("name=TestComp&type=league"),
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Headers = authHeaders(tb, admin)
		s.Headers["Content-Type"] = "application/x-www-form-urlencoded"
	}
	s.Test(t)
}

func TestAdminCreateInvitation(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /admin/invitations creates invite",
		Method:         http.MethodPost,
		URL:            "/admin/invitations",
		Body:           strings.NewReader("email=invite@test.com"),
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Headers = authHeaders(tb, admin)
		s.Headers["Content-Type"] = "application/x-www-form-urlencoded"
	}
	s.Test(t)
}
