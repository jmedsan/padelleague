package notify

import (
	"log/slog"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/league"
)

// logged returns the attributes and level of the single captured record with
// message msg whose "user" attribute is userID.
func (c *logCapture) logged(t *testing.T, msg, userID string) (map[string]string, slog.Level) {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	var attrs map[string]string
	var level slog.Level
	found := 0
	for _, r := range c.records {
		if r.Message != msg {
			continue
		}
		m := map[string]string{}
		r.Attrs(func(a slog.Attr) bool { m[a.Key] = a.Value.String(); return true })
		if m["user"] == userID {
			attrs, level = m, r.Level
			found++
		}
	}
	require.Equal(t, 1, found, "log lines %q for user %s", msg, userID)
	return attrs, level
}

// Every path that stops a notification or its email leaves one log line with
// the type, the recipient's user id and the reason, and no email address.
func TestNotifyPlayers_EveryEmailSkipPathIsLogged(t *testing.T) {
	notif := league.Notification{Type: "message", Title: "T", Body: "B"}
	tests := []struct {
		name    string
		setup   func(t *testing.T, app *tests.TestApp, u *core.Record)
		msg     string
		reason  string
		level   slog.Level
		noEmail bool // the recipient id does not exist
	}{
		{"smtp off", func(*testing.T, *tests.TestApp, *core.Record) {}, "email skipped", "smtp not configured", slog.LevelInfo, false},
		{"no address", func(t *testing.T, app *tests.TestApp, u *core.Record) {
			enableSMTP(t, app)
			u.SetEmail("")
		}, "email skipped", "user has no email address", slog.LevelInfo, false},
		{"unverified", func(t *testing.T, app *tests.TestApp, u *core.Record) {
			enableSMTP(t, app)
			u.SetVerified(false)
		}, "email skipped", "email not verified", slog.LevelInfo, false},
		{"email channel off", func(t *testing.T, app *tests.TestApp, u *core.Record) {
			enableSMTP(t, app)
			u.Set("notification_prefs", map[string]any{"email": false})
		}, "email skipped", "email channel switched off in prefs", slog.LevelInfo, false},
		{"type switched off", func(t *testing.T, app *tests.TestApp, u *core.Record) {
			enableSMTP(t, app)
			u.Set("notification_prefs", map[string]any{"message": false})
		}, "notification skipped", "type switched off in prefs", slog.LevelInfo, false},
		{"recipient not found", func(*testing.T, *tests.TestApp, *core.Record) {}, "notification skipped", "recipient not found", slog.LevelWarn, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cap := withLogCapture(t)
			app := newTestApp(t)
			u := makeUser(t, app, "player")
			tt.setup(t, app, u)
			if tt.name == "no address" {
				// PocketBase requires an email on save, so drive the record directly.
				NewNotifier(app, "", "").emailNotification(u, notif, "")
			} else {
				require.NoError(t, app.Save(u))
				id := u.Id
				if tt.noEmail {
					id = "missing-user"
				}
				NewNotifier(app, "", "").NotifyPlayers([]string{id}, notif)
			}
			id := u.Id
			if tt.noEmail {
				id = "missing-user"
			}
			attrs, level := cap.logged(t, tt.msg, id)
			assert.Equal(t, tt.reason, attrs["reason"])
			assert.Equal(t, "message", attrs["type"])
			assert.Equal(t, tt.level, level)
			assert.Equal(t, 0, app.TestMailer.TotalSend(), "a skipped email must not be sent")
			for k, v := range attrs {
				assert.NotContains(t, v, "@", "attr %s must not carry an email address", k)
			}
		})
	}
}

func TestEmailNotification_SendFailureAndSuccessAreLogged(t *testing.T) {
	cap := withLogCapture(t)
	app := newTestApp(t)
	enableSMTP(t, app)
	u := makeUser(t, app, "player")
	notif := league.Notification{Type: "message", Title: "T", Body: "B"}

	NewNotifier(app, "", "").emailNotification(u, notif, "")
	attrs, level := cap.logged(t, "email sent", u.Id)
	assert.Equal(t, "message", attrs["type"])
	assert.Equal(t, slog.LevelInfo, level)

	app.OnMailerSend().BindFunc(func(e *core.MailerEvent) error {
		e.Mailer = failingMailer{}
		return e.Next()
	})
	NewNotifier(app, "", "").emailNotification(u, notif, "")
	attrs, level = cap.logged(t, "email send failed", u.Id)
	assert.Equal(t, "message", attrs["type"])
	assert.Equal(t, "smtp refused", attrs["err"])
	assert.Equal(t, slog.LevelError, level)
	cap.logged(t, "email sent", u.Id) // still the one from the first send: a failure is not logged as sent
	assert.NotContains(t, attrs["err"], "@")
}

func TestNotifyAdmins_SkipsAreLogged(t *testing.T) {
	cap := withLogCapture(t)
	app := newTestApp(t)
	excluded := makeUser(t, app, "admin")
	off := makeUser(t, app, "admin")
	off.Set("notification_prefs", map[string]any{"dispute": false})
	require.NoError(t, app.Save(off))

	require.NoError(t, NewNotifier(app, "", "").NotifyAdmins(league.Notification{Type: "dispute", Title: "T", Body: "B"}, excluded.Id))

	attrs, _ := cap.logged(t, "notification skipped", excluded.Id)
	assert.Equal(t, "already notified as a player", attrs["reason"])
	attrs, _ = cap.logged(t, "notification skipped", off.Id)
	assert.Equal(t, "type switched off in prefs", attrs["reason"])
	assert.Equal(t, "dispute", attrs["type"])
}
