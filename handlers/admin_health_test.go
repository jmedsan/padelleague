package handlers

import (
	"net/http"
	"os"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/render"
)

func setupHealthRoute(_ testing.TB, app *tests.TestApp, e *core.ServeEvent) {
	r := render.New(os.DirFS(".."), "")
	h := NewAdminHealthHandler(app, r.Page)
	e.Router.GET("/admin/health", h.Health).BindFunc(requireAuthTest).BindFunc(requireAdminTest)
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

func TestHealth_AllEmptyShowsSinIncidencias(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:     testAppFactory,
		Name:               "health dashboard with nothing pending shows Sin incidencias, not empty category cards",
		Method:             http.MethodGet,
		URL:                "/admin/health",
		ExpectedStatus:     200,
		ExpectedContent:    []string{"Sin incidencias — todo en orden"},
		NotExpectedContent: []string{"card-title text-base"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupHealthRoute(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestHealth_MixedShowsOnlyNonEmptyCategories(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:     testAppFactory,
		Name:               "health dashboard with one issue hides the other empty category cards",
		Method:             http.MethodGet,
		URL:                "/admin/health",
		ExpectedStatus:     200,
		ExpectedContent:    []string{"card-title text-base"},
		NotExpectedContent: []string{"Sin incidencias — todo en orden", "Sin pagar"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupHealthRoute(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "HealthEmptyA")
		p2 := makePairTB(tb, app, "HealthEmptyB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("payment_status", map[string]any{p1.Id: true, p2.Id: true})
		require.NoError(tb, app.Save(comp))
		makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
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
