package notify

import (
	"errors"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/mailer"

	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// enableSMTP flips the app into "mailer configured" mode. tests.TestApp binds
// OnMailerSend to its TestMailer, so nothing leaves the process.
func enableSMTP(t *testing.T, app *tests.TestApp) {
	t.Helper()
	app.Settings().SMTP.Enabled = true
	app.Settings().SMTP.Host = "smtp.test.local"
	app.Settings().SMTP.Port = 587
	require.NoError(t, app.Save(app.Settings()))
	require.True(t, IsMailerConfigured(app))
}

func TestBuildNotificationEmail_Basic(t *testing.T) {
	t.Parallel()
	html := BuildNotificationEmail("Juan", "Tu partido ha sido confirmado", "")
	assert.Contains(t, html, "Juan")
	assert.Contains(t, html, "Tu partido ha sido confirmado")
	assert.Contains(t, html, "PadelLeague")
	assert.NotContains(t, html, "Ver partido")
}

func TestBuildNotificationEmail_WithLink(t *testing.T) {
	t.Parallel()
	html := BuildNotificationEmail("Ana", "Resultado enviado", "https://example.com/match/123")
	assert.Contains(t, html, "Ana")
	assert.Contains(t, html, "Ver partido")
	assert.Contains(t, html, "https://example.com/match/123")
}

func TestBuildNotificationEmail_EscapesHTML(t *testing.T) {
	t.Parallel()
	html := BuildNotificationEmail("<script>alert(1)</script>", "Body", "")
	assert.NotContains(t, html, "<script>")
	assert.Contains(t, html, "&lt;script&gt;")
}

func TestEmailNotifyPlayers_NoSMTP(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	user := makeUser(t, app, "player")

	NewNotifier(app, "", "").EmailPlayers([]string{user.Id}, "Test", "Body", "")

	assert.Equal(t, 0, app.TestMailer.TotalSend(), "nothing may be sent while SMTP is off")
}

func TestEmailNotifyPlayers_InvalidUser(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	enableSMTP(t, app)

	NewNotifier(app, "", "").EmailPlayers([]string{"nonexistent"}, "Test", "Body", "")

	assert.Equal(t, 0, app.TestMailer.TotalSend())
}

// failingMailer replaces the TestMailer so the send-error branch runs.
type failingMailer struct{}

func (failingMailer) Send(*mailer.Message) error { return errors.New("smtp refused") }

func TestSendEmail_SendFailureIsContained(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	enableSMTP(t, app)
	app.OnMailerSend().BindFunc(func(e *core.MailerEvent) error {
		e.Mailer = failingMailer{}
		return e.Next()
	})

	// A failing transport must not panic or propagate; it is logged and dropped.
	assert.NotPanics(t, func() {
		SendEmail(app, "player@test.local", "Asunto", "<p>Cuerpo</p>")
	})
	assert.Equal(t, 0, app.TestMailer.TotalSend(), "the failing mailer replaced TestMailer")
}

func TestSendEmail_Sends(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	enableSMTP(t, app)

	SendEmail(app, "player@test.local", "Asunto", "<p>Cuerpo</p>")

	require.Equal(t, 1, app.TestMailer.TotalSend())
	msg := app.TestMailer.LastMessage()
	assert.Equal(t, "Asunto", msg.Subject)
	assert.Equal(t, "<p>Cuerpo</p>", msg.HTML)
	require.Len(t, msg.To, 1)
	assert.Equal(t, "player@test.local", msg.To[0].Address)
	assert.Equal(t, app.Settings().Meta.AppName, msg.From.Name)
	assert.Equal(t, app.Settings().Meta.SenderAddress, msg.From.Address)
}

func TestEmailNotifyPlayers_SendsOnePerPlayer(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	enableSMTP(t, app)
	one := makeUser(t, app, "player")
	two := makeUser(t, app, "player")

	NewNotifier(app, "", "").EmailPlayers([]string{one.Id, two.Id}, "Partido confirmado", "Body", "https://example.com/match/1")

	require.Equal(t, 2, app.TestMailer.TotalSend())

	got := make(map[string]string, 2)
	for _, msg := range app.TestMailer.Messages() {
		require.Len(t, msg.To, 1)
		assert.Equal(t, "Partido confirmado", msg.Subject)
		got[msg.To[0].Address] = msg.HTML
	}

	require.Contains(t, got, one.Email())
	require.Contains(t, got, two.Email())
	// The body is personalized per recipient and carries the match link.
	assert.Contains(t, got[one.Email()], one.GetString("display_name"))
	assert.Contains(t, got[two.Email()], two.GetString("display_name"))
	assert.Contains(t, got[one.Email()], "https://example.com/match/1")
}

func TestEmailNotifyPlayers_SkipsPlayerWithoutEmail(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	enableSMTP(t, app)
	withEmail := makeUser(t, app, "player")
	withoutEmail := makeUser(t, app, "player")
	withoutEmail.Set("email", "")
	require.NoError(t, app.Save(withoutEmail))

	NewNotifier(app, "", "").EmailPlayers([]string{withoutEmail.Id, withEmail.Id}, "Test", "Body", "")

	require.Equal(t, 1, app.TestMailer.TotalSend())
	assert.Equal(t, withEmail.Email(), app.TestMailer.LastMessage().To[0].Address)
}

func TestMaskEmail_Boundaries(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		input, want string
	}{
		"no @":           {"noemail", "***"},
		"@ at start":     {"@example.com", "***"},
		"single char":    {"a@example.com", "a***@example.com"},
		"two chars":      {"ab@example.com", "ab***@example.com"},
		"normal":         {"john@example.com", "jo***@example.com"},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, maskEmail(tc.input))
		})
	}
}

func TestSendEmail_SendError_LogsError(t *testing.T) {
	cap := withLogCapture(t)
	app := newTestApp(t)
	enableSMTP(t, app)
	app.OnMailerSend().BindFunc(func(e *core.MailerEvent) error {
		e.Mailer = failingMailer{}
		return e.Next()
	})

	SendEmail(app, "player@test.local", "Asunto", "<p>Cuerpo</p>")

	assert.True(t, cap.hasMessage("send email failed"),
		"send failure must be logged")
}
