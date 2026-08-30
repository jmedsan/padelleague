package handlers

import (
	"net/http"
	"os"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"

	"padelleague/render"
)

func setupHealthRoute(_ testing.TB, app *tests.TestApp, e *core.ServeEvent) {
	r := render.New(os.DirFS(".."), "")
	h := NewAdminHealthHandler(app, r.Page)
	e.Router.GET("/admin/health", h.Health).BindFunc(requireAuthTest).BindFunc(requireAdminTest)
}

func TestHealth_AdminSeesGroups(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "admin health dashboard shows all five groups",
		Method:         http.MethodGet,
		URL:            "/admin/health",
		ExpectedStatus: 200,
		ExpectedContent: []string{
			"Disputas abiertas",
			"Walkovers pendientes",
			"Partidos vencidos",
			"Parejas sin pagar",
			"Partidos sin fecha",
		},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupHealthRoute(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestHealth_DisputeShowsLink(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "health dashboard shows disputed match link",
		Method:          http.MethodGet,
		URL:             "/admin/health",
		ExpectedStatus:  200,
		ExpectedContent: []string{"/match/"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupHealthRoute(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "HealthA")
		p2 := makePairTB(tb, app, "HealthB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestHealth_EmptyGroupShowsSinPendientes(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "health dashboard empty group shows Sin pendientes",
		Method:          http.MethodGet,
		URL:             "/admin/health",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Sin pendientes"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupHealthRoute(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestHealth_NonAdminDenied(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "non-admin cannot access health dashboard",
		Method:         http.MethodGet,
		URL:            "/admin/health",
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupHealthRoute(tb, app, e)
		user := makeUserTB(tb, app, "Regular Player", "")
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(_ testing.TB, _ *tests.TestApp, res *http.Response) {
		loc := res.Header.Get("Location")
		assert.Contains(t, loc, "/login", "non-admin should be redirected to login")
	}
	s.Test(t)
}
