package handlers

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/league"
	"padelleague/middleware"
	"padelleague/notify"
	"padelleague/render"
)

func setupFullAdminRoutes(_ testing.TB, app *tests.TestApp, e *core.ServeEvent) {
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
	g.POST("/competitions/{id}/copy-pairs", comp.CopyPairs)
	g.POST("/competitions/{id}/remove-pair", comp.RemovePair)
	g.POST("/competitions/{id}/payment", comp.TogglePayment)
	g.POST("/competitions/{id}/payment-all", comp.TogglePaymentAll)
	g.POST("/competitions/{id}/penalty", comp.ApplyPenalty)
	g.POST("/competitions/{id}/generate", fixture.GenerateFixtures)
	g.GET("/players", admin.Players)
	g.POST("/players/pre-create", admin.PlayerPreCreate)
	g.POST("/players/{id}", admin.PlayerUpdate)
	g.GET("/pairs", admin.Pairs)
	g.POST("/pairs", admin.PairsCreate)
	g.POST("/pairs/{id}", admin.PairsUpdate)
	g.GET("/disputes", admin.Disputes)
	g.POST("/disputes/{id}/resolve", admin.DisputesResolve)
	g.GET("/invitations", admin.InvitationsList)
	g.POST("/invitations", admin.InvitationsCreate)
	g.POST("/invitations/{id}/revoke", admin.InvitationsRevoke)
	g.GET("/venues", admin.Venues)
	g.POST("/venues", admin.VenuesCreate)
	g.POST("/venues/{id}", admin.VenuesUpdate)
	g.POST("/venues/{id}/delete", admin.VenuesDelete)
}

func TestAdminDisputesPage(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "GET /admin/disputes with disputed match",
		Method:          http.MethodGet,
		URL:             "/admin/disputes",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Disputas"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupFullAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "DispPageA")
		p2 := makePairTB(tb, app, "DispPageB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestAdminPairsPage(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "GET /admin/pairs returns pair list",
		Method:          http.MethodGet,
		URL:             "/admin/pairs",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Parejas"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupFullAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestAdminPairsCreate(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /admin/pairs creates pair",
		Method:         http.MethodPost,
		URL:            "/admin/pairs",
		ExpectedStatus: 204,
	}
	var u1ID, u2ID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupFullAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		u1 := makeUserTB(tb, app, "PairP1", "")
		u2 := makeUserTB(tb, app, "PairP2", "")
		u1ID, u2ID = u1.Id, u2.Id
		s.Body = strings.NewReader("name=NuevaPair&player1=" + u1.Id + "&player2=" + u2.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		pairs, err := app.FindRecordsByFilter("pairs", "name = 'NuevaPair'", "", 0, 0, nil)
		require.NoError(tb, err)
		require.Equal(tb, 1, len(pairs))
		assert.Equal(tb, u1ID, pairs[0].GetString("player1"))
		assert.Equal(tb, u2ID, pairs[0].GetString("player2"))
	}
	s.Test(t)
}

func TestAdminPairsUpdate(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /admin/pairs/{id} updates pair name",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var pairID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupFullAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		pair := makePairTB(tb, app, "UpdPair")
		pairID = pair.Id
		s.URL = "/admin/pairs/" + pair.Id
		s.Body = strings.NewReader("name=Renamed")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		p, err := app.FindRecordById("pairs", pairID)
		require.NoError(tb, err)
		assert.Equal(tb, "Renamed", p.GetString("name"))
	}
	s.Test(t)
}

func TestAdminCopyPairs(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "POST /admin/competitions/{id}/copy-pairs copies pairs",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"copiadas"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupFullAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "CopyA")
		p2 := makePairTB(tb, app, "CopyB")
		source := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		target := makeCompetitionTB(tb, app, "league", nil)
		s.URL = "/admin/competitions/" + target.Id + "/copy-pairs"
		s.Body = strings.NewReader("source_competition=" + source.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestAdminTogglePaymentAll(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /admin/competitions/{id}/payment-all marks all paid",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupFullAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "PayAllA")
		p2 := makePairTB(tb, app, "PayAllB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		s.URL = "/admin/competitions/" + comp.Id + "/payment-all"
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestAdminInvitationsRevoke(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /admin/invitations/{id}/revoke changes status",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var invID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupFullAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		inv := makeInvitation(t, app, time.Time{})
		invID = inv.Id
		s.URL = "/admin/invitations/" + inv.Id + "/revoke"
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		_, err := app.FindRecordById("invitations", invID)
		assert.Error(tb, err, "invitation should be deleted")
	}
	s.Test(t)
}

func TestDashboardWithIssues(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "GET /admin dashboard exercises issue classification",
		Method:          http.MethodGet,
		URL:             "/admin",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Competiciones"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupFullAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "IssueA")
		p2 := makePairTB(tb, app, "IssueB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("quorum_timeout_hours", 24)
		require.NoError(tb, app.Save(comp))

		// Disputed match → dispute issue
		makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")

		// Confirmed match with old submitted_at → quorum issue
		qm := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		qm.Set("submitted_at", time.Now().Add(-72*time.Hour).UTC().Format("2006-01-02 15:04:05.000Z"))
		require.NoError(tb, app.Save(qm))

		// Pending match with past date → overdue issue
		om := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		om.Set("date", "2020-01-01")
		require.NoError(tb, app.Save(om))

		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}
