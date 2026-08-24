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

	"padelleague/league"
	"padelleague/middleware"
	"padelleague/notify"
	"padelleague/render"
)

func setupCompRoutes(_ testing.TB, app *tests.TestApp, e *core.ServeEvent) {
	viewsFS := os.DirFS("..")
	r := render.New(viewsFS, "")
	notifier := notify.NewNotifier(app, "", "")
	svc := league.New(app, notifier)

	e.Router.BindFunc(middleware.CookieAuth)

	auth := NewAuthHandler(app, r.Page)
	e.Router.GET("/login", auth.Login)

	admin := NewAdminHandler(app, notifier, r.Page)
	comp := NewCompetitionHandler(app, svc, r.Page)
	fixture := NewFixtureHandler(app, svc, r.Page)

	g := e.Router.Group("/admin")
	g.BindFunc(requireAuthTest)
	g.BindFunc(requireAdminTest)
	g.GET("", comp.Dashboard)
	g.GET("/competitions/{id}", comp.Detail)
	g.POST("/competitions", comp.Create)
	g.POST("/competitions/{id}", comp.Update)
	g.POST("/competitions/{id}/toggle", comp.Toggle)
	g.POST("/competitions/{id}/pairs", comp.AddPair)
	g.POST("/competitions/{id}/remove-pair", comp.RemovePair)
	g.POST("/competitions/{id}/payment", comp.TogglePayment)
	g.POST("/competitions/{id}/penalty", comp.ApplyPenalty)
	g.POST("/competitions/{id}/generate", fixture.GenerateFixtures)
	g.POST("/disputes/{id}/resolve", admin.DisputesResolve)
}

func TestCompUpdate(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /admin/competitions/{id} updates competition",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		comp := makeCompetitionTB(tb, app, "league", nil)
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id
		s.Body = strings.NewReader("name=Updated&type=league")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.Equal(tb, "Updated", c.GetString("name"))
	}
	s.Test(t)
}

func TestCompToggle(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /admin/competitions/{id}/toggle toggles active",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		comp := makeCompetitionTB(tb, app, "league", nil)
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/toggle"
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.Equal(tb, false, c.GetBool("active"))
	}
	s.Test(t)
}

func TestCompAddPair(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /admin/competitions/{id}/pairs adds pair",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, pairID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		comp := makeCompetitionTB(tb, app, "league", nil)
		compID = comp.Id
		pair := makePairTB(tb, app, "NewPair")
		pairID = pair.Id
		s.URL = "/admin/competitions/" + comp.Id + "/pairs"
		s.Body = strings.NewReader("pair=" + pair.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		pairs := c.GetStringSlice("pairs")
		assert.Contains(tb, pairs, pairID)
	}
	s.Test(t)
}

func TestCompRemovePair(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /admin/competitions/{id}/remove-pair removes pair",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, pairID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		pair := makePairTB(tb, app, "RemPair")
		pairID = pair.Id
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{pair})
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/remove-pair"
		s.Body = strings.NewReader("pair_id=" + pair.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		pairs := c.GetStringSlice("pairs")
		assert.NotContains(tb, pairs, pairID)
	}
	s.Test(t)
}

func TestCompTogglePayment(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /admin/competitions/{id}/payment toggles payment",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		pair := makePairTB(tb, app, "PayPair")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{pair})
		s.URL = "/admin/competitions/" + comp.Id + "/payment"
		s.Body = strings.NewReader("pair_id=" + pair.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestCompApplyPenalty(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /admin/competitions/{id}/penalty applies penalty",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		pair := makePairTB(tb, app, "PenPair")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{pair})
		s.URL = "/admin/competitions/" + comp.Id + "/penalty"
		s.Body = strings.NewReader("pair_id=" + pair.Id + "&action=apply")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestCompGenerateFixtures(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /admin/competitions/{id}/generate creates fixtures",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "GenA")
		p2 := makePairTB(tb, app, "GenB")
		p3 := makePairTB(tb, app, "GenC")
		p4 := makePairTB(tb, app, "GenD")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2, p3, p4})
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/generate"
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		matches, err := app.FindRecordsByFilter("matches",
			"competition = {:cid}", "", 0, 0,
			map[string]any{"cid": compID})
		require.NoError(tb, err)
		assert.Greater(tb, len(matches), 0)
	}
	s.Test(t)
}

func TestDisputeResolve(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /admin/disputes/{id}/resolve resolves dispute",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, p1ID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "DispA")
		p2 := makePairTB(tb, app, "DispB")
		p1ID = p1.Id
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		matchID = match.Id
		s.URL = "/admin/disputes/" + match.Id + "/resolve"
		s.Body = strings.NewReader("score=6-3+6-4&winner=" + p1.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "final", m.GetString("status"))
		assert.Equal(tb, "6-3 6-4", m.GetString("scores"))
		assert.Equal(tb, p1ID, m.GetString("winner"))
	}
	s.Test(t)
}
