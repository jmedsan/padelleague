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
	r := render.New(viewsFS, "", true)
	notifier := notify.NewNotifier(app, "", "")
	svc := league.New(app, notifier)

	e.Router.BindFunc(middleware.CookieAuth)

	auth := NewAuthHandler(app, r.Page)
	e.Router.GET("/login", auth.Login)

	comp := NewCompetitionHandler(app, svc, notifier, r.Page)
	dash := NewCompetitionDashboardHandler(app, r.Page)
	player := NewAdminPlayerHandler(app, r.Page)
	pair := NewPairHandler(app, r.Page)
	inv := NewInvitationHandler(app, r.Page)
	venue := NewVenueHandler(app, r.Page)

	g := e.Router.Group("/admin")
	g.BindFunc(requireAuthTest)
	g.BindFunc(requireAdminTest)
	g.GET("", dash.AdminEntry)
	g.GET("/competitions", dash.Dashboard)
	g.GET("/competitions/{id}", comp.Detail)
	g.POST("/competitions", comp.Create)
	g.GET("/players", player.Players)
	g.POST("/players/pre-create", player.PlayerPreCreate)
	g.POST("/players/{id}", player.PlayerUpdate)
	g.POST("/players/{id}/regenerate-link", player.RegenerateLink)
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
	g.POST("/competitions/{id}/broadcast", comp.AdminBroadcast)
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
		Name:            "GET /admin/competitions with admin auth returns dashboard",
		Method:          http.MethodGet,
		URL:             "/admin/competitions",
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
		TestAppFactory: testAppFactory,
		Name:           "GET /admin/invitations redirects to /admin/competitions",
		Method:         http.MethodGet,
		URL:            "/admin/invitations",
		ExpectedStatus: http.StatusFound,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/admin/competitions", res.Header.Get("Location"))
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
		ExpectedContent: []string{"Clubes"},
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
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		comp := makeCompetitionTB(tb, app, "league", nil)
		s.Body = strings.NewReader("email=invite@test.com&competition=" + comp.Id)
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
		URL:             "/admin/competitions",
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

func TestBroadcast_FanOut(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "broadcast notifies all distinct players",
		Method:          http.MethodPost,
		URL:             "/placeholder",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Anuncio enviado"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		enableSMTP(tb, app)
		admin := makeAdminUser(tb, app)
		p1 := makePair(t, app, "BroadA")
		p2 := makePair(t, app, "BroadB")
		comp := makeCompetition(t, app, []*core.Record{p1, p2})
		s.URL = "/admin/competitions/" + comp.Id + "/broadcast"

		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
		s.Body = strings.NewReader("title=Aviso+importante&body=Se+cambia+la+fecha")
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		notifs, err := app.FindRecordsByFilter("notifications",
			"title = 'Aviso importante'", "", 0, 0, nil)
		require.NoError(tb, err)
		assert.Equal(tb, 4, len(notifs), "4 distinct players should get in-app notification")
		assert.Equal(tb, 4, app.TestMailer.TotalSend(), "4 distinct players should get email")
	}
	s.Test(t)
}

func TestBroadcast_NotificationLinksToCompetition(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "broadcast notification links to the competition page",
		Method:          http.MethodPost,
		URL:             "/placeholder",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Anuncio enviado"},
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		enableSMTP(tb, app)
		admin := makeAdminUser(tb, app)
		p1 := makePair(t, app, "BroadLinkA")
		p2 := makePair(t, app, "BroadLinkB")
		comp := makeCompetition(t, app, []*core.Record{p1, p2})
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/broadcast"

		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
		s.Body = strings.NewReader("title=Aviso+con+link&body=Revisa+la+competicion")
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		notifs, err := app.FindRecordsByFilter("notifications",
			"title = 'Aviso con link'", "", 0, 0, nil)
		require.NoError(tb, err)
		require.NotEmpty(tb, notifs)
		for _, n := range notifs {
			assert.Equal(tb, "/competition/"+compID, n.GetString("link"))
		}
	}
	s.Test(t)
}

func TestBroadcast_EmptyTitleRejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "broadcast rejects empty title",
		Method:          http.MethodPost,
		URL:             "/placeholder",
		ExpectedStatus:  200,
		ExpectedContent: []string{"obligatorios"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePair(t, app, "EmptyA")
		comp := makeCompetition(t, app, []*core.Record{p1})
		s.URL = "/admin/competitions/" + comp.Id + "/broadcast"

		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
		s.Body = strings.NewReader("title=&body=Some+body")
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		notifs, _ := app.FindRecordsByFilter("notifications",
			"type = 'general'", "", 0, 0, nil)
		assert.Equal(tb, 0, len(notifs), "no notifications on validation failure")
	}
	s.Test(t)
}

func TestBroadcast_NonAdminDenied(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "non-admin cannot broadcast",
		Method:         http.MethodPost,
		URL:            "/placeholder",
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		user := makeUser(t, app, "Regular", "regular@test.local")
		p1 := makePair(t, app, "DenyA")
		comp := makeCompetition(t, app, []*core.Record{p1})
		s.URL = "/admin/competitions/" + comp.Id + "/broadcast"

		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
		s.Body = strings.NewReader("title=Hola&body=Test")
	}
	s.Test(t)
}

func TestBroadcast_DedupSharedPlayer(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "broadcast deduplicates shared player across pairs",
		Method:          http.MethodPost,
		URL:             "/placeholder",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Anuncio enviado"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		enableSMTP(tb, app)
		admin := makeAdminUser(tb, app)

		u1 := makeUser(t, app, "SharedP1", fmt.Sprintf("shared%d@test.local", pairSeq.Add(1)))
		u2 := makeUser(t, app, "SharedP2", fmt.Sprintf("shared%d@test.local", pairSeq.Add(1)))
		u3 := makeUser(t, app, "SharedP3", fmt.Sprintf("shared%d@test.local", pairSeq.Add(1)))

		col, _ := app.FindCollectionByNameOrId("pairs")
		p1 := core.NewRecord(col)
		p1.Set("name", "DedupPair1")
		p1.Set("player1", u1.Id)
		p1.Set("player2", u2.Id)
		require.NoError(tb, app.Save(p1))

		p2 := core.NewRecord(col)
		p2.Set("name", "DedupPair2")
		p2.Set("player1", u2.Id)
		p2.Set("player2", u3.Id)
		require.NoError(tb, app.Save(p2))

		comp := makeCompetition(t, app, []*core.Record{p1, p2})
		s.URL = "/admin/competitions/" + comp.Id + "/broadcast"

		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
		s.Body = strings.NewReader("title=Dedup+test&body=Should+be+3+not+4")
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		notifs, err := app.FindRecordsByFilter("notifications",
			"title = 'Dedup test'", "", 0, 0, nil)
		require.NoError(tb, err)
		assert.Equal(tb, 3, len(notifs), "shared player u2 should receive only one notification")
		assert.Equal(tb, 3, app.TestMailer.TotalSend(), "shared player u2 should receive only one email")
	}
	s.Test(t)
}
