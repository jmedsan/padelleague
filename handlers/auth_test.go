package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/filesystem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsInviteExpired_Expired(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	invite := makeInvitation(t, app, time.Now().Add(-1*time.Hour))
	assert.True(t, isInviteExpired(invite))
}

func TestIsInviteExpired_Valid(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	invite := makeInvitation(t, app, time.Now().Add(1*time.Hour))
	assert.False(t, isInviteExpired(invite))
}

func TestIsInviteExpired_ZeroDate(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	invite := makeInvitation(t, app, time.Time{})
	assert.True(t, isInviteExpired(invite), "zero date should be treated as expired")
}

// makeInviteWithUses creates an invitation with explicit max_uses and use_count.
func makeInviteWithUses(tb testing.TB, app core.App, maxUses, useCount int) *core.Record {
	tb.Helper()
	creator := makeUserTB(tb, app, "InvCreator", "")
	inv := makeInvitationTB(tb, app, creator.Id, time.Now().Add(24*time.Hour))
	inv.Set("max_uses", maxUses)
	inv.Set("use_count", useCount)
	require.NoError(tb, app.Save(inv))
	return inv
}

// countUsers returns how many users exist in the DB.
func countUsers(tb testing.TB, app core.App) int {
	tb.Helper()
	users, err := app.FindRecordsByFilter("users", "1=1", "", 0, 0, nil)
	require.NoError(tb, err)
	return len(users)
}

// GET /register boundary

// Single-use invite, use_count=0 → page renders the registration form (Token present)
func TestRegisterPage_SingleUse_Count0_ShowsForm(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET register single-use count=0 shows form",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Crear cuenta"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		inv := makeInviteWithUses(tb, app, 1, 0)
		s.URL = "/register?token=" + inv.GetString("token")
	}
	s.Test(t)
}

// Single-use invite, use_count=1 → page shows invalid
func TestRegisterPage_SingleUse_Count1_Refused(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET register single-use count=1 refused",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"ya fue utilizada"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		inv := makeInviteWithUses(tb, app, 1, 1)
		s.URL = "/register?token=" + inv.GetString("token")
	}
	s.Test(t)
}

// A competition-scoped invitation shows "Únete a <comp>" and, when the
// competition has a logo, the logo hero above the wordmark (oracle B2).
func TestRegisterPage_ShowsCompetitionName(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET register shows the scoped competition name in the subtitle",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Únete a Liga Registro"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		inv := makeInviteWithUses(tb, app, 1, 0)
		comp, err := app.FindRecordById("competitions", inv.GetString("competition"))
		require.NoError(tb, err)
		comp.Set("name", "Liga Registro")
		require.NoError(tb, app.Save(comp))
		s.URL = "/register?token=" + inv.GetString("token")
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.NotContains(tb, body, "rounded-2xl ring-2", "no logo set: no hero image should render")
	}
	s.Test(t)
}

// Same as above but the competition has a logo: the hero block must render.
func TestRegisterPage_ShowsCompetitionLogo(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET register shows the competition logo hero when the competition has a logo",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Únete a Liga Logo"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		inv := makeInviteWithUses(tb, app, 1, 0)
		comp, err := app.FindRecordById("competitions", inv.GetString("competition"))
		require.NoError(tb, err)
		comp.Set("name", "Liga Logo")
		f, err := filesystem.NewFileFromBytes([]byte("fake-logo-bytes"), "logo.png")
		require.NoError(tb, err)
		comp.Set("logo", f)
		require.NoError(tb, app.Save(comp))
		s.URL = "/register?token=" + inv.GetString("token")
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.Contains(tb, body, "rounded-2xl ring-2", "a logo-scoped invitation must show the hero image")
		assert.Contains(tb, body, "/api/files/competitions/", "hero image src must be a served competition file URL")
	}
	s.Test(t)
}

// 5-use invite, use_count=4 → shows form
func TestRegisterPage_FiveUse_Count4_ShowsForm(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET register 5-use count=4 shows form",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Crear cuenta"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		inv := makeInviteWithUses(tb, app, 5, 4)
		s.URL = "/register?token=" + inv.GetString("token")
	}
	s.Test(t)
}

// 5-use invite, use_count=5 → refused
func TestRegisterPage_FiveUse_Count5_Refused(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET register 5-use count=5 refused",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"ya fue utilizada"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		inv := makeInviteWithUses(tb, app, 5, 5)
		s.URL = "/register?token=" + inv.GetString("token")
	}
	s.Test(t)
}

// max_uses=0 in DB → clamped to 1, so use_count=0 should still show form
func TestRegisterPage_MaxUses0_ClampedTo1_ShowsForm(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET register max_uses=0 clamped to 1 shows form",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Crear cuenta"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		inv := makeInviteWithUses(tb, app, 0, 0)
		s.URL = "/register?token=" + inv.GetString("token")
	}
	s.Test(t)
}

// max_uses=0, use_count=1 → clamped to 1, so refused
func TestRegisterPage_MaxUses0_Count1_Refused(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET register max_uses=0 count=1 clamped refused",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"ya fue utilizada"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		inv := makeInviteWithUses(tb, app, 0, 1)
		s.URL = "/register?token=" + inv.GetString("token")
	}
	s.Test(t)
}

// POST /register boundary

// Single-use invite, use_count=0 → registration succeeds, use_count becomes 1
func TestRegisterSubmit_SingleUse_Count0_Succeeds(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST register single-use count=0 succeeds",
		Method:         http.MethodPost,
		URL:            "/register",
		ExpectedStatus: 302,
	}
	var invID string
	var usersBefore int
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		inv := makeInviteWithUses(tb, app, 1, 0)
		invID = inv.Id
		usersBefore = countUsers(tb, app)
		s.Body = strings.NewReader("token=" + inv.GetString("token") +
			"&email=newuser1@test.local&display_name=New+User&password=testpass123456&password_confirm=testpass123456&gender=male")
		s.Headers = map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		// User was created
		usersAfter := countUsers(tb, app)
		assert.Equal(tb, usersBefore+1, usersAfter, "one new user must be created")
		// use_count incremented by exactly 1
		inv, err := app.FindRecordById("invitations", invID)
		require.NoError(tb, err)
		assert.Equal(tb, 1, int(inv.GetFloat("use_count")), "use_count must be exactly 1")
	}
	s.Test(t)
}

// Single-use invite, use_count=1 → registration refused, no user created
func TestRegisterSubmit_SingleUse_Count1_Refused(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST register single-use count=1 refused",
		Method:          http.MethodPost,
		URL:             "/register",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Invitación agotada"},
	}
	var usersBefore int
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		inv := makeInviteWithUses(tb, app, 1, 1)
		usersBefore = countUsers(tb, app)
		s.Body = strings.NewReader("token=" + inv.GetString("token") +
			"&email=rejected1@test.local&display_name=Rejected&password=testpass123456&password_confirm=testpass123456&gender=male")
		s.Headers = map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		usersAfter := countUsers(tb, app)
		assert.Equal(tb, usersBefore, usersAfter, "no user must be created")
	}
	s.Test(t)
}

// 5-use invite, use_count=4 → succeeds, becomes 5
func TestRegisterSubmit_FiveUse_Count4_Succeeds(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST register 5-use count=4 succeeds",
		Method:         http.MethodPost,
		URL:            "/register",
		ExpectedStatus: 302,
	}
	var invID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		inv := makeInviteWithUses(tb, app, 5, 4)
		invID = inv.Id
		s.Body = strings.NewReader("token=" + inv.GetString("token") +
			"&email=fiveuse4@test.local&display_name=Five+Four&password=testpass123456&password_confirm=testpass123456&gender=male")
		s.Headers = map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		inv, err := app.FindRecordById("invitations", invID)
		require.NoError(tb, err)
		assert.Equal(tb, 5, int(inv.GetFloat("use_count")), "use_count must be exactly 5")
		assert.Equal(tb, "used", inv.GetString("status"), "status must be 'used' at max")
	}
	s.Test(t)
}

// 5-use invite, use_count=5 → refused
func TestRegisterSubmit_FiveUse_Count5_Refused(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST register 5-use count=5 refused",
		Method:          http.MethodPost,
		URL:             "/register",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Invitación agotada"},
	}
	var usersBefore int
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		inv := makeInviteWithUses(tb, app, 5, 5)
		usersBefore = countUsers(tb, app)
		s.Body = strings.NewReader("token=" + inv.GetString("token") +
			"&email=fiveuse5@test.local&display_name=Five+Five&password=testpass123456&password_confirm=testpass123456&gender=male")
		s.Headers = map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		usersAfter := countUsers(tb, app)
		assert.Equal(tb, usersBefore, usersAfter, "no user must be created")
	}
	s.Test(t)
}

// max_uses=0, use_count=0 → clamped to 1, so POST succeeds
func TestRegisterSubmit_MaxUses0_Count0_Succeeds(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST register max_uses=0 clamped to 1 succeeds",
		Method:         http.MethodPost,
		URL:            "/register",
		ExpectedStatus: 302,
	}
	var invID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		inv := makeInviteWithUses(tb, app, 0, 0)
		invID = inv.Id
		s.Body = strings.NewReader("token=" + inv.GetString("token") +
			"&email=maxzero@test.local&display_name=Max+Zero&password=testpass123456&password_confirm=testpass123456&gender=male")
		s.Headers = map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		inv, err := app.FindRecordById("invitations", invID)
		require.NoError(tb, err)
		assert.Equal(tb, 1, int(inv.GetFloat("use_count")))
	}
	s.Test(t)
}

// max_uses=0, use_count=1 → clamped to 1, so POST refused
func TestRegisterSubmit_MaxUses0_Count1_Refused(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST register max_uses=0 count=1 refused",
		Method:          http.MethodPost,
		URL:             "/register",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Invitación agotada"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		inv := makeInviteWithUses(tb, app, 0, 1)
		s.Body = strings.NewReader("token=" + inv.GetString("token") +
			"&email=maxzero1@test.local&display_name=Max+Zero+1&password=testpass123456&password_confirm=testpass123456&gender=male")
		s.Headers = map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	}
	s.Test(t)
}

func TestLogout(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
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

func TestRegisterSubmitNoToken(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /register without token shows error",
		Method:          http.MethodPost,
		URL:             "/register",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Invitaci"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		s.Body = strings.NewReader("email=new@test.local&password=testpass123456&password_confirm=testpass123456&gender=male")
		s.Headers = map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	}
	s.Test(t)
}

func TestRegisterSubmitValidInvite(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
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
		body := fmt.Sprintf("token=%s&email=%s&display_name=New+Player&password=testpass123456&password_confirm=testpass123456&gender=male", token, regEmail)
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

func TestRegisterWithValidToken(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /register?token=valid shows form with email",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Crear cuenta"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		invite := makeInvitationTB(tb, app, admin.Id, time.Now().Add(24*time.Hour))
		s.URL = "/register?token=" + invite.GetString("token")
	}
	s.Test(t)
}

func TestRegisterWithExpiredToken(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /register?token=expired shows invalid",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Invitación no válida"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		invite := makeInvitationTB(tb, app, admin.Id, time.Now().Add(-1*time.Hour))
		s.URL = "/register?token=" + invite.GetString("token")
	}
	s.Test(t)
}

func TestRegisterSubmitPasswordMismatch(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /register with mismatched passwords",
		Method:          http.MethodPost,
		URL:             "/register",
		ExpectedStatus:  200,
		ExpectedContent: []string{"no coinciden"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		invite := makeInvitationTB(tb, app, admin.Id, time.Now().Add(24*time.Hour))
		body := fmt.Sprintf("token=%s&email=reg@test.local&display_name=Test&password=abc123456&password_confirm=xyz123456", invite.GetString("token"))
		s.Body = strings.NewReader(body)
		s.Headers = map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	}
	s.Test(t)
}
