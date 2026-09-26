package handlers_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/handlers"
	"padelleague/league"
)

func makeAdminUser(t testing.TB, app core.App) *core.Record {
	t.Helper()
	n := handlers.UserSeq.Add(1)
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
		TestAppFactory: handlers.TestAppFactory,
		Name:           "GET /admin without auth redirects to login",
		Method:         http.MethodGet,
		URL:            "/admin",
		ExpectedStatus: 302,
		BeforeTestFunc: func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
			setupProductionRoutes(tb, app, e)
		},
		AfterTestFunc: func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
			assert.Equal(tb, "/login", res.Header.Get("Location"))
		},
	}
	scenario.Test(t)
}

func TestAdminPlayerAuth(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "GET /admin with player auth redirects home",
		Method:         http.MethodGet,
		URL:            "/admin",
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		player := handlers.MakeUserTB(tb, app, "Player", "")
		s.Headers = handlers.AuthHeaders(tb, player)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		// Real middleware.RequireAppAdmin redirects an authenticated non-admin
		// to "/" (home), not "/login" — the bespoke test router this replaced
		// sent non-admins to /login, which production never did.
		assert.Equal(tb, "/", res.Header.Get("Location"))
	}
	s.Test(t)
}

func TestAdminWithAdminAuth(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /admin/competitions with admin auth returns dashboard",
		Method:          http.MethodGet,
		URL:             "/admin/competitions",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Competiciones"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.Test(t)
}

func TestAdminPlayersPage(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /admin/players returns player list",
		Method:          http.MethodGet,
		URL:             "/admin/players",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Usuarios"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.Test(t)
}

func TestAdminInvitationsPage(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /admin/invitations returns invitations list",
		Method:          http.MethodGet,
		URL:             "/admin/invitations",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Invitaciones"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.Test(t)
}

func TestAdminVenuesPage(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /admin/venues returns venue list",
		Method:          http.MethodGet,
		URL:             "/admin/venues",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Clubes"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.Test(t)
}

func TestAdminCreateCompetition(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/competitions creates and redirects",
		Method:         http.MethodPost,
		URL:            "/admin/competitions",
		Body:           strings.NewReader("name=TestComp&type=league"),
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Headers = handlers.AuthHeaders(tb, admin)
		s.Headers["Content-Type"] = "application/x-www-form-urlencoded"
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		comps, err := app.FindRecordsByFilter("competitions",
			"name = 'TestComp'", "", 0, 0, nil)
		require.NoError(tb, err)
		require.Equal(tb, 1, len(comps))
		assert.Equal(tb, "league", comps[0].GetString("type"))
	}
	handlers.ExpectRedirect(s, func(app core.App) string { return "/admin/competitions/" + newestCompetitionID(app) })
	s.Test(t)
}

func TestAdminCreateInvitation(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/invitations creates invite",
		Method:         http.MethodPost,
		URL:            "/admin/invitations",
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		comp := handlers.MakeCompetitionTB(tb, app, "league", nil)
		s.Body = strings.NewReader("email=invite@test.com&competition=" + comp.Id)
		s.Headers = handlers.AuthHeaders(tb, admin)
		s.Headers["Content-Type"] = "application/x-www-form-urlencoded"
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		invites, err := app.FindRecordsByFilter("invitations",
			"status = 'pending'", "", 0, 0, nil)
		require.NoError(tb, err)
		assert.GreaterOrEqual(tb, len(invites), 1, "invitation must be created")
	}
	handlers.ExpectRedirect(s, redirectTo("/admin/invitations"))
	s.Test(t)
}

func TestDashboardWithIssues(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /admin dashboard exercises issue classification",
		Method:          http.MethodGet,
		URL:             "/admin/competitions",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Competiciones"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "IssueA")
		p2 := handlers.MakePairTB(tb, app, "IssueB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("quorum_timeout_hours", 24)
		require.NoError(tb, app.Save(comp))

		// Disputed match → dispute issue
		handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")

		// Confirmed match with old submitted_at → quorum issue
		qm := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		qm.Set("submitted_at", time.Now().Add(-72*time.Hour).UTC().Format("2006-01-02 15:04:05.000Z"))
		require.NoError(tb, app.Save(qm))

		// Pending match with past date → overdue issue
		om := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		om.Set("date", "2020-01-01")
		require.NoError(tb, app.Save(om))

		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.Test(t)
}

func TestBroadcast_FanOut(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "broadcast notifies all distinct players",
		Method:         http.MethodPost,
		URL:            "/placeholder",
		ExpectedStatus: 204,
	}
	var compID, p1p1ID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		enableSMTP(tb, app)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePair(tb, app, "BroadA")
		p2 := handlers.MakePair(tb, app, "BroadB")
		p1p1ID = p1.GetString("player1")
		comp := handlers.MakeCompetition(tb, app, []*core.Record{p1, p2})
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/broadcast"

		hdrs := handlers.AuthHeaders(tb, admin)
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

		want := league.Notification{
			Type:     "announcement",
			Title:    "Aviso importante",
			Body:     "Se cambia la fecha",
			Link:     "/competition/" + compID + "#avisos",
			CompName: "Test Competition",
		}
		assertNotified(tb, app, p1p1ID, want)
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

func TestBroadcast_NotificationLinksToCompetition(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "broadcast notification links to the competition page",
		Method:         http.MethodPost,
		URL:            "/placeholder",
		ExpectedStatus: 204,
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		enableSMTP(tb, app)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePair(tb, app, "BroadLinkA")
		p2 := handlers.MakePair(tb, app, "BroadLinkB")
		comp := handlers.MakeCompetition(tb, app, []*core.Record{p1, p2})
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/broadcast"

		hdrs := handlers.AuthHeaders(tb, admin)
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
			assert.Equal(tb, "/competition/"+compID+"#avisos", n.GetString("link"))
		}
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

func TestBroadcast_EmptyTitleRejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "broadcast rejects empty title",
		Method:          http.MethodPost,
		URL:             "/placeholder",
		ExpectedStatus:  200,
		ExpectedContent: []string{"obligatorios"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePair(tb, app, "EmptyA")
		comp := handlers.MakeCompetition(tb, app, []*core.Record{p1})
		s.URL = "/admin/competitions/" + comp.Id + "/broadcast"

		hdrs := handlers.AuthHeaders(tb, admin)
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
		TestAppFactory: handlers.TestAppFactory,
		Name:           "non-admin cannot broadcast",
		Method:         http.MethodPost,
		URL:            "/placeholder",
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		user := handlers.MakeUser(tb, app, "Regular", "regular@test.local")
		p1 := handlers.MakePair(tb, app, "DenyA")
		comp := handlers.MakeCompetition(tb, app, []*core.Record{p1})
		s.URL = "/admin/competitions/" + comp.Id + "/broadcast"

		hdrs := handlers.AuthHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
		s.Body = strings.NewReader("title=Hola&body=Test")
	}
	s.Test(t)
}

func TestBroadcast_DedupSharedPlayer(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "broadcast deduplicates shared player across pairs",
		Method:         http.MethodPost,
		URL:            "/placeholder",
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		enableSMTP(tb, app)
		admin := makeAdminUser(tb, app)

		u1 := handlers.MakeUser(tb, app, "SharedP1", fmt.Sprintf("shared%d@test.local", handlers.PairSeq.Add(1)))
		u2 := handlers.MakeUser(tb, app, "SharedP2", fmt.Sprintf("shared%d@test.local", handlers.PairSeq.Add(1)))
		u3 := handlers.MakeUser(tb, app, "SharedP3", fmt.Sprintf("shared%d@test.local", handlers.PairSeq.Add(1)))

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

		comp := handlers.MakeCompetition(tb, app, []*core.Record{p1, p2})
		s.URL = "/admin/competitions/" + comp.Id + "/broadcast"

		hdrs := handlers.AuthHeaders(tb, admin)
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
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

func TestBroadcast_CreatesAnnouncementRecord(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "broadcast creates an announcement record",
		Method:         http.MethodPost,
		URL:            "/placeholder",
		ExpectedStatus: 204,
	}
	var compID, adminID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		enableSMTP(tb, app)
		admin := makeAdminUser(tb, app)
		adminID = admin.Id
		p1 := handlers.MakePair(tb, app, "AnnA")
		comp := handlers.MakeCompetition(tb, app, []*core.Record{p1})
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/broadcast"

		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
		s.Body = strings.NewReader("title=Aviso+guardado&body=Cuerpo+del+anuncio")
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/admin/competitions/"+compID, res.Header.Get("HX-Redirect"))
		anns, err := app.FindRecordsByFilter("announcements",
			"title = 'Aviso guardado'", "", 0, 0, nil)
		require.NoError(tb, err)
		require.Len(tb, anns, 1)
		assert.Equal(tb, compID, anns[0].GetString("competition"))
		assert.Equal(tb, "Cuerpo del anuncio", anns[0].GetString("body"))
		assert.Equal(tb, adminID, anns[0].GetString("created_by"))
	}
	s.Test(t)
}

func TestBroadcast_NotificationType(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "broadcast notification uses type announcement",
		Method:         http.MethodPost,
		URL:            "/placeholder",
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		enableSMTP(tb, app)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePair(tb, app, "AnnTypeA")
		comp := handlers.MakeCompetition(tb, app, []*core.Record{p1})
		s.URL = "/admin/competitions/" + comp.Id + "/broadcast"

		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
		s.Body = strings.NewReader("title=Tipo+anuncio&body=Verifica+el+tipo")
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		notifs, err := app.FindRecordsByFilter("notifications",
			"title = 'Tipo anuncio'", "", 0, 0, nil)
		require.NoError(tb, err)
		require.NotEmpty(tb, notifs)
		for _, n := range notifs {
			assert.Equal(tb, "announcement", n.GetString("type"))
		}
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

func TestBroadcast_LinkIncludesHash(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "broadcast notification link includes #avisos hash",
		Method:         http.MethodPost,
		URL:            "/placeholder",
		ExpectedStatus: 204,
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		enableSMTP(tb, app)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePair(tb, app, "AnnHashA")
		comp := handlers.MakeCompetition(tb, app, []*core.Record{p1})
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/broadcast"

		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
		s.Body = strings.NewReader("title=Aviso+con+hash&body=Revisa+el+enlace")
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		notifs, err := app.FindRecordsByFilter("notifications",
			"title = 'Aviso con hash'", "", 0, 0, nil)
		require.NoError(tb, err)
		require.NotEmpty(tb, notifs)
		for _, n := range notifs {
			assert.Equal(tb, "/competition/"+compID+"#avisos", n.GetString("link"))
		}
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

func TestDeleteAnnouncement_AdminOnly(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "non-admin cannot delete an announcement",
		Method:         http.MethodPost,
		URL:            "/placeholder",
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		user := handlers.MakeUser(tb, app, "AnnDenyUser", "anndeny@test.local")
		p1 := handlers.MakePair(tb, app, "AnnDenyA")
		comp := handlers.MakeCompetition(tb, app, []*core.Record{p1})

		col, err := app.FindCollectionByNameOrId("announcements")
		require.NoError(tb, err)
		ann := core.NewRecord(col)
		ann.Set("competition", comp.Id)
		ann.Set("title", "Deny me")
		ann.Set("body", "body")
		ann.Set("created_by", user.Id)
		require.NoError(tb, app.Save(ann))

		s.URL = "/admin/competitions/" + comp.Id + "/announcements/" + ann.Id + "/delete"

		hdrs := handlers.AuthHeaders(tb, user)
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestDeleteAnnouncement_Success(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "admin can delete an announcement",
		Method:         http.MethodPost,
		URL:            "/placeholder",
		ExpectedStatus: 204,
	}
	var annID, compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePair(tb, app, "AnnDelA")
		comp := handlers.MakeCompetition(tb, app, []*core.Record{p1})
		compID = comp.Id

		col, err := app.FindCollectionByNameOrId("announcements")
		require.NoError(tb, err)
		ann := core.NewRecord(col)
		ann.Set("competition", comp.Id)
		ann.Set("title", "Delete me")
		ann.Set("body", "body")
		ann.Set("created_by", admin.Id)
		require.NoError(tb, app.Save(ann))
		annID = ann.Id

		s.URL = "/admin/competitions/" + comp.Id + "/announcements/" + ann.Id + "/delete"
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/admin/competitions/"+compID, res.Header.Get("HX-Redirect"))
		_, err := app.FindRecordById("announcements", annID)
		assert.Error(tb, err, "announcement should no longer exist")
	}
	s.Test(t)
}

func TestPaymentReminder_SendsToUnpaid(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "payment reminder notifies only unpaid pairs",
		Method:          http.MethodPost,
		URL:             "/placeholder",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Recordatorio enviado"},
	}
	var paidPlayerID, unpaidPlayerID1, unpaidPlayerID2, compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		enableSMTP(tb, app)
		admin := makeAdminUser(tb, app)
		paidPair := handlers.MakePair(tb, app, "ReminderPaid")
		unpaidPair := handlers.MakePair(tb, app, "ReminderUnpaid")
		comp := handlers.MakeCompetition(tb, app, []*core.Record{paidPair, unpaidPair})
		comp.Set("payment_status", map[string]any{paidPair.Id: true, unpaidPair.Id: false})
		require.NoError(tb, app.Save(comp))
		compID = comp.Id

		paidPlayerID = paidPair.GetString("player1")
		unpaidPlayerID1 = unpaidPair.GetString("player1")
		unpaidPlayerID2 = unpaidPair.GetString("player2")

		s.URL = "/admin/competitions/" + comp.Id + "/payment-reminder"
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		want := league.Notification{
			Type:     "payment",
			Title:    "Recordatorio de pago",
			Body:     "Recuerda realizar el pago para Test Competition",
			Link:     "/competition/" + compID,
			CompName: "Test Competition",
		}
		assertNotified(tb, app, unpaidPlayerID1, want)
		assertNotified(tb, app, unpaidPlayerID2, want)
		assertNotNotified(tb, app, paidPlayerID, want.Title)
	}
	s.Test(t)
}

func TestPaymentReminder_AllPaid(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "payment reminder warns when all pairs are paid",
		Method:          http.MethodPost,
		URL:             "/placeholder",
		ExpectedStatus:  200,
		ExpectedContent: []string{"al día"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePair(tb, app, "ReminderAllPaidA")
		comp := handlers.MakeCompetition(tb, app, []*core.Record{p1})
		comp.Set("payment_status", map[string]any{p1.Id: true})
		require.NoError(tb, app.Save(comp))

		s.URL = "/admin/competitions/" + comp.Id + "/payment-reminder"
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		notifs, err := app.FindRecordsByFilter("notifications", "type = 'payment'", "", 0, 0, nil)
		require.NoError(tb, err)
		assert.Empty(tb, notifs, "no notifications sent when all paid")
	}
	s.Test(t)
}

func TestPaymentReminder_NonAdminDenied(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "non-admin cannot send payment reminder",
		Method:         http.MethodPost,
		URL:            "/placeholder",
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		user := handlers.MakeUser(tb, app, "Regular", "regular-payment-reminder@test.local")
		p1 := handlers.MakePair(tb, app, "ReminderDenyA")
		comp := handlers.MakeCompetition(tb, app, []*core.Record{p1})

		s.URL = "/admin/competitions/" + comp.Id + "/payment-reminder"
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}
