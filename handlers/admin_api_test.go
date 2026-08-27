package handlers

import (
	"fmt"
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

func setupAdminRoutes(_ testing.TB, app *tests.TestApp, e *core.ServeEvent) {
	viewsFS := os.DirFS("..")
	r := render.New(viewsFS, "")
	notifier := notify.NewNotifier(app, "", "")
	svc := league.New(app, notifier)

	e.Router.BindFunc(middleware.CookieAuth)

	auth := NewAuthHandler(app, r.Page)
	e.Router.GET("/login", auth.Login)

	comp := NewCompetitionHandler(app, svc, r.Page)
	player := NewAdminPlayerHandler(app, r.Page)
	pair := NewPairHandler(app, r.Page)
	inv := NewInvitationHandler(app, r.Page)
	venue := NewVenueHandler(app, r.Page)

	g := e.Router.Group("/admin")
	g.BindFunc(requireAuthTest)
	g.BindFunc(requireAdminTest)
	g.GET("", comp.Dashboard)
	g.GET("/competitions/{id}", comp.Detail)
	g.POST("/competitions", comp.Create)
	g.GET("/players", player.Players)
	g.POST("/players/pre-create", player.PlayerPreCreate)
	g.POST("/players/{id}", player.PlayerUpdate)
	g.GET("/pairs", pair.Pairs)
	g.POST("/pairs", pair.PairsCreate)
	g.POST("/pairs/{id}", pair.PairsUpdate)
	g.GET("/invitations", inv.InvitationsList)
	g.POST("/invitations", inv.InvitationsCreate)
	g.POST("/invitations/{id}/revoke", inv.InvitationsRevoke)
	g.GET("/venues", venue.Venues)
	g.POST("/venues", venue.VenuesCreate)
	g.POST("/venues/{id}", venue.VenuesUpdate)
	g.POST("/venues/{id}/delete", venue.VenuesDelete)
}

func makeAdminUser(t testing.TB, app core.App) *core.Record {
	t.Helper()
	n := userSeq.Add(1)
	col, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Set("email", fmt.Sprintf("admin%d@test.local", n))
	record.Set("display_name", "Test Admin")
	record.Set("roles", []string{"admin"})
	record.SetPassword("testpass123456")
	record.SetVerified(true)
	require.NoError(t, app.Save(record))
	return record
}

func TestAdminNoAuth(t *testing.T) {
	t.Parallel()
	scenario := tests.ApiScenario{
		Name:           "GET /admin without auth redirects to login",
		Method:         http.MethodGet,
		URL:            "/admin",
		ExpectedStatus: 302,
		BeforeTestFunc: setupAdminRoutes,
		AfterTestFunc: func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
			assert.Equal(tb, "/login", res.Header.Get("Location"))
		},
	}
	scenario.Test(t)
}

func TestAdminPlayerAuth(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "GET /admin with player auth redirects to login",
		Method:         http.MethodGet,
		URL:            "/admin",
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		player := makeUserTB(tb, app, "Player", "")
		s.Headers = authHeaders(tb, player)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/login", res.Header.Get("Location"))
	}
	s.Test(t)
}

func TestAdminWithAdminAuth(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /admin with admin auth returns dashboard",
		Method:          http.MethodGet,
		URL:             "/admin",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Competiciones"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestAdminPlayersPage(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /admin/players returns player list",
		Method:          http.MethodGet,
		URL:             "/admin/players",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Jugadores"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestAdminInvitationsPage(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /admin/invitations returns invitation list",
		Method:          http.MethodGet,
		URL:             "/admin/invitations",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Invitaciones"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestAdminVenuesPage(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /admin/venues returns venue list",
		Method:          http.MethodGet,
		URL:             "/admin/venues",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Pistas"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestAdminCreateCompetition(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
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
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		comps, err := app.FindRecordsByFilter("competitions",
			"name = 'TestComp'", "", 0, 0, nil)
		require.NoError(tb, err)
		require.Equal(tb, 1, len(comps))
		assert.Equal(tb, "league", comps[0].GetString("type"))
	}
	s.Test(t)
}

func TestAdminCreateInvitation(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
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
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		invites, err := app.FindRecordsByFilter("invitations",
			"status = 'pending'", "", 0, 0, nil)
		require.NoError(tb, err)
		assert.GreaterOrEqual(tb, len(invites), 1, "invitation must be created")
	}
	s.Test(t)
}

func TestDashboardWithIssues(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
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
