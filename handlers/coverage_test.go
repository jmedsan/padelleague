package handlers

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
)

func TestHomeWithCompetitionData(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		Name:            "GET / with active competition shows home data",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Home A", "Home B"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Home A")
		p2 := makePairTB(tb, app, "Home B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		// Pending match — triggers buildNextMatch, buildHomeCompetition
		makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		// Confirmed match — triggers findUnconfirmedScores
		confirmed := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		confirmed.Set("scores", "6-3 6-4")
		confirmed.Set("submitted_by", p1.GetString("player1"))
		require.NoError(tb, app.Save(confirmed))

		// Final match — triggers findRecentResults
		col, _ := app.FindCollectionByNameOrId("matches")
		final := core.NewRecord(col)
		final.Set("competition", comp.Id)
		final.Set("pair1", p1.Id)
		final.Set("pair2", p2.Id)
		final.Set("status", "final")
		final.Set("scores", "6-2 6-1")
		final.Set("winner", p1.Id)
		final.Set("round_number", 1)
		require.NoError(tb, app.Save(final))

		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestCompetitionPageWithMatches(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		Name:            "GET /competition/{id} with matches shows pair names",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Comp A", "Comp B"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Comp A")
		p2 := makePairTB(tb, app, "Comp B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestLogout(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		Name:           "POST /logout redirects to login",
		Method:         http.MethodPost,
		URL:            "/logout",
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

func TestForgotPasswordPage(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		Name:            "GET /forgot-password returns form",
		Method:          http.MethodGet,
		URL:             "/forgot-password",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Restablecer contraseña"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
	}
	s.Test(t)
}

func TestForgotPasswordSubmit(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		Name:            "POST /forgot-password shows success",
		Method:          http.MethodPost,
		URL:             "/forgot-password",
		ExpectedStatus:  200,
		ExpectedContent: []string{"alert-"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		s.Body = strings.NewReader("email=test@test.local")
		s.Headers = map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	}
	s.Test(t)
}

func TestResetPasswordPage(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		Name:            "GET /reset-password returns form",
		Method:          http.MethodGet,
		URL:             "/reset-password?token=test",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Nueva contrase"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
	}
	s.Test(t)
}

func TestResetPasswordSubmitInvalidToken(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		Name:            "POST /reset-password with bad token shows error",
		Method:          http.MethodPost,
		URL:             "/reset-password",
		ExpectedStatus:  200,
		ExpectedContent: []string{"alert-error"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		s.Body = strings.NewReader("token=invalid&password=newpass123456&passwordConfirm=newpass123456")
		s.Headers = map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	}
	s.Test(t)
}

func TestResetPasswordSubmitMismatch(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		Name:            "POST /reset-password with mismatched passwords",
		Method:          http.MethodPost,
		URL:             "/reset-password",
		ExpectedStatus:  200,
		ExpectedContent: []string{"no coinciden"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		s.Body = strings.NewReader("token=test&password=abc&passwordConfirm=xyz")
		s.Headers = map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	}
	s.Test(t)
}

func TestRegisterSubmitNoToken(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		Name:            "POST /register without token shows error",
		Method:          http.MethodPost,
		URL:             "/register",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Invitaci"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		s.Body = strings.NewReader("email=new@test.local&password=testpass123456&password_confirm=testpass123456")
		s.Headers = map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	}
	s.Test(t)
}

func TestRegisterSubmitValidInvite(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		Name:           "POST /register with valid invite creates user",
		Method:         http.MethodPost,
		URL:            "/register",
		ExpectedStatus: 302,
	}
	var inviteID, regEmail string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		invite := makeInvitationTB(tb, app, admin.Id, time.Now().Add(24*time.Hour))
		inviteID = invite.Id
		token := invite.GetString("token")
		n := userSeq.Add(1)
		regEmail = fmt.Sprintf("reg%d@test.local", n)
		body := fmt.Sprintf("token=%s&email=%s&display_name=New+Player&password=testpass123456&password_confirm=testpass123456", token, regEmail)
		s.Body = strings.NewReader(body)
		s.Headers = map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		users, err := app.FindRecordsByFilter("users",
			"email = {:email}", "", 0, 0,
			map[string]any{"email": regEmail})
		require.NoError(tb, err)
		require.Equal(tb, 1, len(users))
		assert.Equal(tb, "New Player", users[0].GetString("display_name"))

		inv, err := app.FindRecordById("invitations", inviteID)
		require.NoError(tb, err)
		assert.Equal(tb, "used", inv.GetString("status"))

		assert.Equal(tb, "/", res.Header.Get("Location"))
	}
	s.Test(t)
}

func TestAdminOverride(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		Name:           "POST /match/{id}/admin-override changes score and finalizes",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, p1ID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Override A")
		p2 := makePairTB(tb, app, "Override B")
		p1ID = p1.Id
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		matchID = m.Id
		m.Set("scores", "6-3 6-4")
		m.Set("submitted_by", p1.GetString("player1"))
		require.NoError(tb, app.Save(m))

		s.URL = "/match/" + m.Id + "/admin-override"
		s.Body = strings.NewReader("scores=6-4+6-3&dispute_notes=Corregido+por+admin")
		admin := makeAdminUserTB(tb, app)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "final", m.GetString("status"))
		assert.Equal(tb, "6-4 6-3", m.GetString("scores"))
		assert.Equal(tb, p1ID, m.GetString("winner"))
	}
	s.Test(t)
}
