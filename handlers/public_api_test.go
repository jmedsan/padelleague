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

func setupPublicRoutes(_ testing.TB, app *tests.TestApp, e *core.ServeEvent) {
	viewsFS := os.DirFS("..")
	r := render.New(viewsFS, "", true)
	notifier := notify.NewNotifier(app, "", "")
	svc := league.New(app, notifier)

	e.Router.BindFunc(middleware.CookieAuth)

	auth := NewAuthHandler(app, r.Page)
	e.Router.GET("/login", auth.Login)
	e.Router.GET("/profile/complete", auth.ProfileComplete).BindFunc(requireAuthTest)
	e.Router.POST("/profile/complete", auth.ProfileCompleteSubmit).BindFunc(requireAuthTest)

	pub := NewPublicHandler(app, svc, r.Page, r.ErrorPage)
	e.Router.GET("/", pub.Home).BindFunc(requireAuthTest)
	e.Router.GET("/competition/{id}", pub.Competition).BindFunc(requireAuthTest)
	e.Router.POST("/competition/{id}/accept-docs", pub.AcceptDocs).BindFunc(requireAuthTest)

	player := NewPlayerHandler(app, svc, PlayerRenderers{Page: r.Page, Partial: r.Partial, ErrorPage: r.ErrorPage})
	e.Router.GET("/player/{id}", player.Player).BindFunc(requireAuthTest)
	e.Router.POST("/player/{id}/avatar", player.PlayerAvatarUpload).BindFunc(requireAuthTest)

	pair := NewPairPageHandler(app, svc, r.Page, r.ErrorPage)
	e.Router.GET("/pair/{id}", pair.PairPage).BindFunc(requireAuthTest)

	ical := NewICalHandler(app)
	e.Router.GET("/ical/match/{id}", ical.Match).BindFunc(requireAuthTest)
	e.Router.GET("/ical/competition/{id}", ical.Competition).BindFunc(requireAuthTest)
}

func TestHomeWithAuth(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET / with auth returns home with player name",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Home Player"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Home Player", "")
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestCompetitionPage(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /competition/{id} shows pair names",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"CompA", "CompB", "Clasificación"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "CompA")
		p2 := makePairTB(tb, app, "CompB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")
		m.Set("scores", "6-3 6-3")
		m.Set("winner", p1.Id)
		require.NoError(tb, app.Save(m))
		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestCompetitionMineOnly(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /competition/{id} default hides non-own matches",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"MineA", "MineB"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "MineA")
		p2 := makePairTB(tb, app, "MineB")
		p3 := makePairTB(tb, app, "OtherC")
		p4 := makePairTB(tb, app, "OtherD")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2, p3, p4})
		makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		makeMatchTB(tb, app, comp.Id, p3.Id, p4.Id, "pending")
		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		matchLinks := strings.Count(body, "href=\"/match/")
		assert.Equal(tb, 1, matchLinks, "only own match link in rounds")
		assert.Contains(tb, body, "Mis partidos")
		assert.Contains(tb, body, "Todos")
	}
	s.Test(t)
}

func TestCompetitionShowAll(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /competition/{id}?all=1 shows all matches",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Dale Fuerte a la Bola"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "AllA")
		p2 := makePairTB(tb, app, "AllB")
		p3 := makePairTB(tb, app, "AllC")
		p4 := makePairTB(tb, app, "AllD")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2, p3, p4})
		makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		makeMatchTB(tb, app, comp.Id, p3.Id, p4.Id, "pending")
		s.URL = "/competition/" + comp.Id + "?all=1"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		matchLinks := strings.Count(body, "href=\"/match/")
		assert.Equal(tb, 2, matchLinks, "all=1 shows both matches")
	}
	s.Test(t)
}

func TestPlayerProfilePage(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /player/{id} shows display name",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Profile Viewer"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Profile Viewer", "")
		s.URL = "/player/" + user.Id
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestICalMatch(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /ical/match/{id} with date returns ics",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"VCALENDAR"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "IcalA")
		p2 := makePairTB(tb, app, "IcalB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		match.Set("date", "2026-09-01")
		match.Set("time", "18:00")
		require.NoError(tb, app.Save(match))
		s.URL = "/ical/match/" + match.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestICalCompetition(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /ical/competition/{id} with auth returns ics",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"VCALENDAR"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "IcalCompA")
		p2 := makePairTB(tb, app, "IcalCompB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		match.Set("date", "2026-09-01")
		match.Set("time", "18:00")
		require.NoError(tb, app.Save(match))
		s.URL = "/ical/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestProfileCompletePage(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /profile/complete shows name form",
		Method:          http.MethodGet,
		URL:             "/profile/complete",
		ExpectedStatus:  200,
		ExpectedContent: []string{"display_name"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Profile User", "")
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestProfileCompleteSubmit(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /profile/complete sets display name",
		Method:         http.MethodPost,
		URL:            "/profile/complete",
		Body:           strings.NewReader("display_name=NuevoNombre&gender=female"),
		ExpectedStatus: 302,
	}
	var userID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Old Name", "")
		userID = user.Id
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		u, err := app.FindRecordById("users", userID)
		require.NoError(tb, err)
		assert.Equal(tb, "NuevoNombre", u.GetString("display_name"))
		assert.Equal(tb, "/", res.Header.Get("Location"))
	}
	s.Test(t)
}

func TestCompetitionNotFound(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /competition/{id} with bad ID returns error",
		Method:          http.MethodGet,
		ExpectedStatus:  404,
		ExpectedContent: []string{"no encontrada"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		user := makeUserTB(tb, app, "NotFound Viewer", "")
		s.URL = "/competition/nonexistent_id"
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestCompetitionPlayoffNoStandings(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /competition/{id} playoff has no standings",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"PlayA"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "PlayA")
		p2 := makePairTB(tb, app, "PlayB")
		comp := makeCompetitionTB(tb, app, "playoff", []*core.Record{p1, p2})
		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestCompetitionArchivedShowsAwards(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /competition/{id} archived shows awards section",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Archivada"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "ArcA")
		p2 := makePairTB(tb, app, "ArcB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("active", false)
		require.NoError(tb, app.Save(comp))
		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}
