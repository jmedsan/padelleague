package handlers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoginPage(t *testing.T) {
	t.Parallel()
	scenario := tests.ApiScenario{
		Name:            "GET /login returns login page",
		Method:          http.MethodGet,
		URL:             "/login",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Contraseña"},
		BeforeTestFunc:  setupAuthRoutes,
	}
	scenario.Test(t)
}

func TestLoginWrongCreds(t *testing.T) {
	t.Parallel()
	scenario := tests.ApiScenario{
		Name:            "POST /login with wrong creds shows error",
		Method:          http.MethodPost,
		URL:             "/login",
		Body:            strings.NewReader("email=wrong@test.com&password=badpassword"),
		Headers:         map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		ExpectedStatus:  200,
		ExpectedContent: []string{"alert-error"},
		BeforeTestFunc:  setupAuthRoutes,
	}
	scenario.Test(t)
}

func TestRegisterPage(t *testing.T) {
	t.Parallel()
	scenario := tests.ApiScenario{
		Name:            "GET /register returns register page",
		Method:          http.MethodGet,
		URL:             "/register",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Registro"},
		BeforeTestFunc:  setupAuthRoutes,
	}
	scenario.Test(t)
}

func TestLoginValidCreds(t *testing.T) {
	t.Parallel()
	scenario := tests.ApiScenario{
		Name:   "POST /login with valid creds redirects to home",
		Method: http.MethodPost,
		URL:    "/login",
		Body:   strings.NewReader("email=testlogin@test.local&password=testpass123456"),
		Headers: map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
		},
		ExpectedStatus: 302,
		BeforeTestFunc: func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
			setupAuthRoutes(tb, app, e)
			makeUserTB(tb, app, "Login Test", "testlogin@test.local")
		},
		AfterTestFunc: func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
			assert.Equal(tb, "/", res.Header.Get("Location"))
		},
	}
	scenario.Test(t)
}

func TestHomeWithoutAuth(t *testing.T) {
	t.Parallel()
	scenario := tests.ApiScenario{
		Name:           "GET / without auth redirects to login",
		Method:         http.MethodGet,
		URL:            "/",
		ExpectedStatus: 302,
		BeforeTestFunc: func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
			setupAllRoutes(tb, app, e)
		},
		AfterTestFunc: func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
			assert.Equal(tb, "/login", res.Header.Get("Location"))
		},
	}
	scenario.Test(t)
}

func TestNotificationCount(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:     testAppFactory,
		Name:               "GET /notifications/count returns 200",
		Method:             http.MethodGet,
		URL:                "/notifications/count",
		ExpectedStatus:     200,
		NotExpectedContent: []string{"error"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Player", "")
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestNotificationList(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /notifications/list returns page",
		Method:          http.MethodGet,
		URL:             "/notifications/list",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Notificaciones"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Notif User", "")
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestMatchDetail(t *testing.T) {
	t.Parallel()
	var matchID string
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /match/{id} with auth returns match page",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Equipo A", "Equipo B"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Equipo A")
		p2 := makePairTB(tb, app, "Equipo B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id
		s.URL = "/match/" + matchID
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestMatchDetailWithoutAuth(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "GET /match/{id} without auth redirects to login",
		Method:         http.MethodGet,
		URL:            "/match/fakeid",
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/login", res.Header.Get("Location"))
	}
	s.Test(t)
}

func TestAdminDashboard(t *testing.T) {
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
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestAdminDashboardNonAdmin(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "GET /admin with non-admin redirects to login",
		Method:         http.MethodGet,
		URL:            "/admin",
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Regular", "")
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/login", res.Header.Get("Location"))
	}
	s.Test(t)
}

func TestAdminCompetitionDetail(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /admin/competitions/{id} returns detail",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"PadelLeague"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		comp := makeCompetitionTB(tb, app, "league", nil)
		s.URL = "/admin/competitions/" + comp.Id
		s.ExpectedContent = []string{"Test Competition"}
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestPlayerProfile(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /player/{id} returns profile with display name",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Profile Player"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Profile Player", "")
		s.URL = "/player/" + user.Id
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestMatchSubmitValidScore(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/submit with valid score",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, submitterID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Submit A")
		p2 := makePairTB(tb, app, "Submit B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id
		s.URL = "/match/" + match.Id + "/submit"
		s.Body = strings.NewReader("scores=6-3+6-4")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		submitterID = user.Id
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "confirmed", m.GetString("status"))
		assert.Equal(tb, "6-3 6-4", m.GetString("scores"))
		assert.Equal(tb, submitterID, m.GetString("submitted_by"))
	}
	s.Test(t)
}
