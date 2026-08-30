package handlers

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildResetURL: X-Forwarded-Proto: https → https:// link

func TestPreCreateResetURLWithForwardedProto(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/players/pre-create with X-Forwarded-Proto: https produces https link",
		Method:          http.MethodPost,
		URL:             "/admin/players/pre-create",
		ExpectedStatus:  200,
		ExpectedContent: []string{"reset-password"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		hdrs["X-Forwarded-Proto"] = "https"
		s.Headers = hdrs
		s.Body = strings.NewReader("email=fwdproto@test.local&display_name=FwdProto")
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body, err := io.ReadAll(res.Body)
		require.NoError(tb, err)
		assert.Contains(tb, string(body), "https://",
			"reset link must use https when X-Forwarded-Proto is https")
		assert.NotContains(tb, string(body), "http://",
			"reset link must not contain http:// when forwarded proto is https")
	}
	s.Test(t)
}

// buildResetURL: no header, no TLS → http:// link
// Note: ApiScenario test client has no TLS, so e.Request.TLS==nil is always
// true here. The TLS!=nil→https path is not reachable through ApiScenario.
// However, both mutants on lines 129 and 131 are killed by these two tests:
// the nil-flip mutant on 131 would yield https here (killed by http assert),
// and the ==""→!="" mutant on 129 would enter the block and fall to http
// (killed by the forwarded-proto test asserting https).

func TestPreCreateResetURLNoTLS(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/players/pre-create without TLS produces http link",
		Method:          http.MethodPost,
		URL:             "/admin/players/pre-create",
		ExpectedStatus:  200,
		ExpectedContent: []string{"reset-password"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		// No X-Forwarded-Proto, test HTTP client has no TLS → scheme = http
		s.Headers = hdrs
		s.Body = strings.NewReader("email=notls@test.local&display_name=NoTLS")
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body, err := io.ReadAll(res.Body)
		require.NoError(tb, err)
		assert.Contains(tb, string(body), "http://",
			"reset link must use http when no TLS and no forwarded proto")
	}
	s.Test(t)
}

// PlayerUpdate: invalid role "superadmin" → rejected, user unchanged

func TestPlayerUpdateInvalidRole(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/players/{id} rejects invalid role",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Rol inválido"},
	}
	var playerID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		player := makeUserTB(tb, app, "Role Test", "roletest@test.local")
		playerID = player.Id
		s.URL = "/admin/players/" + player.Id
		s.Body = strings.NewReader("display_name=Role+Test&roles=superadmin")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		p, err := app.FindRecordById("users", playerID)
		require.NoError(tb, err)
		assert.Contains(tb, p.GetStringSlice("roles"), "player",
			"role must not change when invalid role submitted")
	}
	s.Test(t)
}

// PlayerUpdate: no roles submitted → defaults to player

func TestPlayerUpdateEmptyRolesDefaultsToPlayer(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/players/{id} defaults to player when no roles submitted",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var playerID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		player := makeUserTB(tb, app, "Empty Role", "emptyrole@test.local")
		playerID = player.Id
		s.URL = "/admin/players/" + player.Id
		s.Body = strings.NewReader("display_name=Empty+Role")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		p, err := app.FindRecordById("users", playerID)
		require.NoError(tb, err)
		assert.Contains(tb, p.GetStringSlice("roles"), "player",
			"roles must default to player when none submitted")
	}
	s.Test(t)
}

// createPlayerInvitation: expiry is ~48h (2*24*time.Hour)

func TestPreCreateInvitationExpiry48h(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/players/pre-create invitation expires in ~48h",
		Method:          http.MethodPost,
		URL:             "/admin/players/pre-create",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Usuario creado"},
	}
	var beforeCreate time.Time
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		beforeCreate = time.Now()
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
		s.Body = strings.NewReader("email=expiry48@test.local&display_name=Expiry48")
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		invites, err := app.FindRecordsByFilter("invitations",
			"email = 'expiry48@test.local'", "", 1, 0, nil)
		require.NoError(tb, err)
		require.Equal(tb, 1, len(invites))
		expiresAt := invites[0].GetDateTime("expires_at").Time()
		earliest := beforeCreate.Add(47 * time.Hour)
		latest := beforeCreate.Add(49 * time.Hour)
		assert.True(tb, expiresAt.After(earliest),
			"expires_at %v should be after %v (47h)", expiresAt, earliest)
		assert.True(tb, expiresAt.Before(latest),
			"expires_at %v should be before %v (49h)", expiresAt, latest)
	}
	s.Test(t)
}

func TestPreCreateSendsOnboardingEmail(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/players/pre-create sends onboarding email",
		Method:          http.MethodPost,
		URL:             "/admin/players/pre-create",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Usuario creado"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		enableSMTP(tb, app)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
		s.Body = strings.NewReader("email=onboard@test.local&display_name=OnboardUser")
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		require.Equal(tb, 1, app.TestMailer.TotalSend(), "expected one onboarding email")
		msg := app.TestMailer.LastMessage()
		assert.Equal(tb, "onboard@test.local", msg.To[0].Address)
		assert.Contains(tb, msg.Subject, "Bienvenido")
		assert.Contains(tb, msg.HTML, "reset-password?token=")
	}
	s.Test(t)
}
