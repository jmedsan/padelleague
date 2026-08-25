package handlers

import (
	"net/http"
	"strings"
	"testing"

	"errors"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/mailer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/notify"
)

// failingMailer replaces the TestMailer so the send-error branch runs.
type failingMailer struct{}

func (failingMailer) Send(*mailer.Message) error { return errors.New("smtp refused") }

// enableSMTP should live in handlers/testutil_test.go when applying.
// It flips the test app into "mailer configured" mode.
// tests.TestApp hooks OnMailerSend to its TestMailer, so nothing leaves the process.
func enableSMTP(t testing.TB, app *tests.TestApp) {
	t.Helper()
	app.Settings().SMTP.Enabled = true
	app.Settings().SMTP.Host = "smtp.test.local"
	app.Settings().SMTP.Port = 587
	require.NoError(t, app.Save(app.Settings()))
	require.True(t, notify.IsMailerConfigured(app))
}

// --- ForgotPasswordSubmit: SMTP configured, valid email → mail sent ---

func TestForgotPasswordSubmitSMTPValid(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /forgot-password SMTP on + known email sends reset mail",
		Method:          http.MethodPost,
		URL:             "/forgot-password",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Si el email"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		enableSMTP(tb, app)
		makeUserTB(tb, app, "PW User", "pwuser@test.local")
		s.Body = strings.NewReader("email=pwuser@test.local")
		s.Headers = map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		require.Equal(tb, 1, app.TestMailer.TotalSend(), "expected exactly one email sent")
		msg := app.TestMailer.LastMessage()
		assert.Equal(tb, "pwuser@test.local", msg.To[0].Address)
	}
	s.Test(t)
}

// --- ForgotPasswordSubmit: SMTP configured, unknown email → no mail, non-disclosure ---

func TestForgotPasswordSubmitSMTPUnknown(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /forgot-password SMTP on + unknown email sends no mail",
		Method:          http.MethodPost,
		URL:             "/forgot-password",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Si el email"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		enableSMTP(tb, app)
		s.Body = strings.NewReader("email=nobody@test.local")
		s.Headers = map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		assert.Equal(tb, 0, app.TestMailer.TotalSend(), "no mail for unknown address")
	}
	s.Test(t)
}

// --- ForgotPasswordSubmit: SMTP unconfigured → info message, no mail ---

func TestForgotPasswordSubmitNoSMTP(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /forgot-password SMTP off shows info message",
		Method:          http.MethodPost,
		URL:             "/forgot-password",
		ExpectedStatus:  200,
		ExpectedContent: []string{"SMTP no configurado"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		s.Body = strings.NewReader("email=anyone@test.local")
		s.Headers = map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		assert.Equal(tb, 0, app.TestMailer.TotalSend(), "no mail when SMTP is off")
	}
	s.Test(t)
}

// --- ResetPasswordSubmit: token not found ---

func TestResetPasswordSubmitTokenNotFound(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /reset-password with nonexistent token shows error",
		Method:          http.MethodPost,
		URL:             "/reset-password",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Token inválido o expirado"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		s.Body = strings.NewReader("token=nonexistent_token_abc123&password=newpass123456&passwordConfirm=newpass123456")
		s.Headers = map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	}
	s.Test(t)
}

// --- ResetPasswordSubmit: token empty ---

func TestResetPasswordSubmitTokenEmpty(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /reset-password with empty token shows error",
		Method:          http.MethodPost,
		URL:             "/reset-password",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Token inválido o expirado"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		s.Body = strings.NewReader("token=&password=newpass123456&passwordConfirm=newpass123456")
		s.Headers = map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	}
	s.Test(t)
}

// --- ResetPasswordSubmit: valid token + matching passwords → success redirect ---
func TestResetPasswordSubmitValidToken(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /reset-password with valid token redirects to login",
		Method:         http.MethodPost,
		URL:            "/reset-password",
		ExpectedStatus: 204, // redirectHX returns 204 with HX-Redirect header
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Token User", "tokenuser@test.local")
		token, err := user.NewPasswordResetToken()
		require.NoError(tb, err)
		s.Body = strings.NewReader("token=" + token + "&password=newpass123456&passwordConfirm=newpass123456")
		s.Headers = map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/login", res.Header.Get("HX-Redirect"))
	}
	s.Test(t)
}

// --- ForgotPasswordSubmit: SMTP configured, empty email → no mail, still success ---

func TestForgotPasswordSubmitSMTPEmptyEmail(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /forgot-password SMTP on + empty email sends no mail",
		Method:          http.MethodPost,
		URL:             "/forgot-password",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Si el email"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		enableSMTP(tb, app)
		s.Body = strings.NewReader("email=")
		s.Headers = map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		assert.Equal(tb, 0, app.TestMailer.TotalSend(), "no mail for empty email")
	}
	s.Test(t)
}

// --- ForgotPasswordSubmit: SMTP configured, valid email, send fails → error (S-2 fix) ---
// Note: this test covers the S-2 fix where send failure returns alertError
// instead of swallowing the error. If S-2 has not been applied yet, the
// ExpectedContent assertion will need adjusting (current code shows success).
// Note: failingMailer is also defined in worker1's S-2 draft. At apply time
// the leader should deduplicate — keep one copy in the test file.

func TestForgotPasswordSubmitSendFailure(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /forgot-password send failure returns error",
		Method:          http.MethodPost,
		URL:             "/forgot-password",
		ExpectedStatus:  200,
		ExpectedContent: []string{"No se pudo enviar"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		enableSMTP(tb, app)
		makeUserTB(tb, app, "FailMail User", "failmail@test.local")
		app.OnMailerSend().BindFunc(func(ev *core.MailerEvent) error {
			ev.Mailer = failingMailer{}
			return ev.Next()
		})
		s.Body = strings.NewReader("email=failmail@test.local")
		s.Headers = map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		assert.Equal(tb, 0, app.TestMailer.TotalSend(), "failing mailer must not count as sent")
	}
	s.Test(t)
}

func TestForgotPasswordPage(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
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
		TestAppFactory:  testAppFactory,
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
		TestAppFactory:  testAppFactory,
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
		TestAppFactory:  testAppFactory,
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
		TestAppFactory:  testAppFactory,
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

func TestForgotPasswordSubmitWithUser(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /forgot-password with existing user",
		Method:          http.MethodPost,
		URL:             "/forgot-password",
		ExpectedStatus:  200,
		ExpectedContent: []string{"alert-"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		makeUserTB(tb, app, "Reset User", "resetuser@test.local")
		s.Body = strings.NewReader("email=resetuser@test.local")
		s.Headers = map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	}
	s.Test(t)
}
