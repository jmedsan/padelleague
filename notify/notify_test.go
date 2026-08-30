package notify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/league"
	_ "padelleague/migrations"
)

type logCapture struct {
	mu      sync.Mutex
	records []slog.Record
}

func (c *logCapture) Enabled(context.Context, slog.Level) bool { return true }
func (c *logCapture) Handle(_ context.Context, r slog.Record) error {
	c.mu.Lock()
	c.records = append(c.records, r.Clone())
	c.mu.Unlock()
	return nil
}
func (c *logCapture) WithAttrs([]slog.Attr) slog.Handler { return c }
func (c *logCapture) WithGroup(string) slog.Handler      { return c }

func (c *logCapture) hasMessage(msg string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, r := range c.records {
		if r.Message == msg {
			return true
		}
	}
	return false
}

func withLogCapture(t *testing.T) *logCapture {
	t.Helper()
	cap := &logCapture{}
	old := slog.Default()
	slog.SetDefault(slog.New(cap))
	t.Cleanup(func() { slog.SetDefault(old) })
	return cap
}

var userSeq atomic.Int64

func newTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)
	return app
}

func makeUser(t *testing.T, app core.App, role string) *core.Record {
	t.Helper()
	n := userSeq.Add(1)
	col, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Set("email", fmt.Sprintf("notifyuser%d@test.local", n))
	record.Set("display_name", fmt.Sprintf("User %d", n))
	record.Set("roles", []string{role})
	record.SetPassword("testpass123456")
	record.SetVerified(true)
	require.NoError(t, app.Save(record))
	return record
}

func TestNotifyPlayers_CreatesNotification(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	notifier := NewNotifier(app, "", "")

	user := makeUser(t, app, "player")
	notifier.NotifyPlayers([]string{user.Id}, league.Notification{Type: "general", Title: "Test Title", Body: "Test Body"})

	notifs, err := app.FindRecordsByFilter("notifications",
		"user = {:uid}", "", 0, 0, map[string]any{"uid": user.Id})
	require.NoError(t, err)
	require.Len(t, notifs, 1)
	assert.Equal(t, "Test Title", notifs[0].GetString("title"))
	assert.Equal(t, "Test Body", notifs[0].GetString("body"))
	assert.Equal(t, "general", notifs[0].GetString("type"))
}

func makeMatch(t *testing.T, app core.App) *core.Record {
	t.Helper()
	n := userSeq.Add(1)
	u1 := makeUser(t, app, "player")
	u2 := makeUser(t, app, "player")
	pairCol, _ := app.FindCollectionByNameOrId("pairs")
	pair1 := core.NewRecord(pairCol)
	pair1.Set("name", fmt.Sprintf("P%d", n))
	pair1.Set("player1", u1.Id)
	pair1.Set("player2", u2.Id)
	require.NoError(t, app.Save(pair1))
	u3 := makeUser(t, app, "player")
	u4 := makeUser(t, app, "player")
	pair2 := core.NewRecord(pairCol)
	pair2.Set("name", fmt.Sprintf("P%d-b", n))
	pair2.Set("player1", u3.Id)
	pair2.Set("player2", u4.Id)
	require.NoError(t, app.Save(pair2))
	compCol, _ := app.FindCollectionByNameOrId("competitions")
	comp := core.NewRecord(compCol)
	comp.Set("name", "Test")
	comp.Set("type", "league")
	comp.Set("active", true)
	comp.Set("pairs", []string{pair1.Id, pair2.Id})
	require.NoError(t, app.Save(comp))
	matchCol, _ := app.FindCollectionByNameOrId("matches")
	match := core.NewRecord(matchCol)
	match.Set("competition", comp.Id)
	match.Set("pair1", pair1.Id)
	match.Set("pair2", pair2.Id)
	match.Set("status", "pending")
	match.Set("round_number", 1)
	require.NoError(t, app.Save(match))
	return match
}

func TestNotifyPlayers_WithMatchID(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	notifier := NewNotifier(app, "", "")

	user := makeUser(t, app, "player")
	match := makeMatch(t, app)
	notifier.NotifyPlayers([]string{user.Id}, league.Notification{Type: "general", Title: "Match", Body: "Score submitted", MatchID: match.Id})

	notifs, _ := app.FindRecordsByFilter("notifications",
		"user = {:uid}", "", 0, 0, map[string]any{"uid": user.Id})
	require.Len(t, notifs, 1)
	assert.Equal(t, match.Id, notifs[0].GetString("related_match"))
}

func TestNotifyPlayers_MultipleUsers(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	notifier := NewNotifier(app, "", "")

	u1 := makeUser(t, app, "player")
	u2 := makeUser(t, app, "player")
	notifier.NotifyPlayers([]string{u1.Id, u2.Id}, league.Notification{Type: "general", Title: "Broadcast", Body: "To all"})

	n1, _ := app.FindRecordsByFilter("notifications", "user = {:uid}", "", 0, 0, map[string]any{"uid": u1.Id})
	n2, _ := app.FindRecordsByFilter("notifications", "user = {:uid}", "", 0, 0, map[string]any{"uid": u2.Id})
	assert.Len(t, n1, 1)
	assert.Len(t, n2, 1)
}

func TestNotifyPlayers_InvalidUser(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	notifier := NewNotifier(app, "", "")

	notifier.NotifyPlayers([]string{"nonexistent"}, league.Notification{Type: "general", Title: "Title", Body: "Body"})

	notifs, _ := app.FindRecordsByFilter("notifications", "title = 'Title'", "", 0, 0, nil)
	assert.Empty(t, notifs, "no notification should be created for a nonexistent user")
}

func TestNotifyAdmins_CreatesNotification(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	notifier := NewNotifier(app, "", "")

	admin := makeUser(t, app, "admin")

	match := makeMatch(t, app)
	err := notifier.NotifyAdmins(league.Notification{Type: "dispute", Title: "Dispute", Body: "A dispute was filed", MatchID: match.Id})
	require.NoError(t, err)

	notifs, _ := app.FindRecordsByFilter("notifications",
		"user = {:uid}", "", 0, 0, map[string]any{"uid": admin.Id})
	require.Len(t, notifs, 1)
	assert.Equal(t, "Dispute", notifs[0].GetString("title"))
	assert.Equal(t, "dispute", notifs[0].GetString("type"))
	assert.Equal(t, "A dispute was filed", notifs[0].GetString("body"))
	assert.Equal(t, match.Id, notifs[0].GetString("related_match"),
		"the admin notification must link back to the match")
}

func TestNotificationPrefs_NilReturnsDefaults(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	user := makeUser(t, app, "player")

	prefs := NotificationPrefs(user)
	assert.Equal(t, true, prefs["general"])
	assert.Equal(t, true, prefs["dispute"])
	assert.Equal(t, true, prefs["scheduling"])
}

func TestNotificationPrefs_WithPrefsSet(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	user := makeUser(t, app, "player")
	user.Set("notification_prefs", map[string]any{"general": false})

	prefs := NotificationPrefs(user)
	assert.Equal(t, false, prefs["general"])
	assert.Equal(t, true, prefs["dispute"], "unset keys fall back to the default")
	assert.Equal(t, true, prefs["scheduling"])
}

// Regression: PocketBase returns a saved JSONField as types.JSONRaw, so a
// stored preference has to survive the round-trip through the database.
func TestNotificationPrefs_SurvivesRoundTrip(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	user := makeUser(t, app, "player")
	user.Set("notification_prefs", map[string]any{"general": false, "dispute": false})
	require.NoError(t, app.Save(user))

	fresh, err := app.FindRecordById("users", user.Id)
	require.NoError(t, err)
	require.IsType(t, types.JSONRaw{}, fresh.Get("notification_prefs"))

	prefs := NotificationPrefs(fresh)
	assert.Equal(t, false, prefs["general"])
	assert.Equal(t, false, prefs["dispute"])
	assert.Equal(t, true, prefs["quorum_request"])
	assert.Equal(t, true, prefs["match_assigned"])
	assert.Equal(t, true, prefs["scheduling"])
}

func TestNotificationPrefs_MalformedFallsBackToDefaults(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	user := makeUser(t, app, "player")

	for name, raw := range map[string]any{
		"empty JSONRaw":   types.JSONRaw{},
		"invalid JSON":    types.JSONRaw("{not json"),
		"JSON null":       types.JSONRaw("null"),
		"wrong JSON type": types.JSONRaw("[1,2,3]"),
	} {
		t.Run(name, func(t *testing.T) {
			user.Set("notification_prefs", raw)
			prefs := NotificationPrefs(user)
			assert.Equal(t, true, prefs["general"])
			assert.Equal(t, true, prefs["match_progress"])
			assert.Len(t, prefs, 6)
		})
	}
}

// A disabled preference must actually suppress the notification record.
func TestNotifyPlayers_RespectsDisabledPref(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	notifier := NewNotifier(app, "", "")
	user := makeUser(t, app, "player")
	user.Set("notification_prefs", map[string]any{"general": false, "dispute": true})
	require.NoError(t, app.Save(user))

	notifier.NotifyPlayers([]string{user.Id}, league.Notification{Type: "general", Title: "Suppressed", Body: "Body"})
	notifier.NotifyPlayers([]string{user.Id}, league.Notification{Type: "dispute", Title: "Delivered", Body: "Body"})

	notifs, err := app.FindRecordsByFilter("notifications",
		"user = {:uid}", "", 0, 0, map[string]any{"uid": user.Id})
	require.NoError(t, err)
	require.Len(t, notifs, 1)
	assert.Equal(t, "dispute", notifs[0].GetString("type"))
	assert.Equal(t, "Delivered", notifs[0].GetString("title"))
}

func TestPushEnabled(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		public, private string
		want            bool
	}{
		"both keys set":   {"pub", "priv", true},
		"public missing":  {"", "priv", false},
		"private missing": {"pub", "", false},
		"both missing":    {"", "", false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, NewNotifier(nil, tc.public, tc.private).PushEnabled())
		})
	}
}

func TestIsMailerConfigured_Disabled(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	assert.False(t, IsMailerConfigured(app))
}

func TestSendEmail_NoSMTP(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	SendEmail(app, "test@test.local", "Subject", "<p>Body</p>")

	assert.Equal(t, 0, app.TestMailer.TotalSend(), "nothing may be sent while SMTP is off")
}

func TestNotifyPlayers_WithVAPIDNoSubs(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	notifier := NewNotifier(app, "BFakePublicKey123456789012345678901234567890123", "fakeprivatekey")

	user := makeUser(t, app, "player")
	notifier.NotifyPlayers([]string{user.Id}, league.Notification{Type: "general", Title: "Push Test", Body: "Body"})

	time.Sleep(200 * time.Millisecond)

	notifs, _ := app.FindRecordsByFilter("notifications",
		"user = {:uid}", "", 0, 0, map[string]any{"uid": user.Id})
	assert.Len(t, notifs, 1)
}

func TestNotifyPlayers_WithPushSub(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	notifier := NewNotifier(app, "BFakePublicKey123456789012345678901234567890123", "fakeprivatekey")

	user := makeUser(t, app, "player")

	subCol, _ := app.FindCollectionByNameOrId("push_subscriptions")
	sub := core.NewRecord(subCol)
	sub.Set("user", user.Id)
	sub.Set("endpoint", "https://fake-push-service.invalid/push/123")
	sub.Set("p256dh", "BFakeP256dhKey1234567890123456789012345678901234567890123456789012345678901234567890123456")
	sub.Set("auth", "fakeauthkey12345678")
	require.NoError(t, app.Save(sub))

	notifier.NotifyPlayers([]string{user.Id}, league.Notification{Type: "general", Title: "Push", Body: "With sub"})
	time.Sleep(500 * time.Millisecond)

	notifs, _ := app.FindRecordsByFilter("notifications",
		"user = {:uid}", "", 0, 0, map[string]any{"uid": user.Id})
	assert.Len(t, notifs, 1)
}

func TestEmailNotifyPlayers_NoEmail(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	user := makeUser(t, app, "player")
	user.Set("email", "")
	require.NoError(t, app.Save(user))

	NewNotifier(app, "", "").EmailPlayers([]string{user.Id}, "Test", "Body", "link")

	assert.Equal(t, 0, app.TestMailer.TotalSend())
}

func TestNotifyAdmins_NoMatchID(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	notifier := NewNotifier(app, "", "")
	admin := makeUser(t, app, "admin")

	err := notifier.NotifyAdmins(league.Notification{Type: "general", Title: "Admin Notice", Body: "Something happened"})
	require.NoError(t, err)

	notifs, _ := app.FindRecordsByFilter("notifications",
		"user = {:uid}", "", 0, 0, map[string]any{"uid": admin.Id})
	require.Len(t, notifs, 1)
	assert.Equal(t, "", notifs[0].GetString("related_match"))
}

func TestNotifyPlayers_SaveError_LogsError(t *testing.T) {
	cap := withLogCapture(t)
	app := newTestApp(t)
	notifier := NewNotifier(app, "", "")
	notifier.save = func(*core.Record) error { return errors.New("injected") }

	user := makeUser(t, app, "player")
	notifier.NotifyPlayers([]string{user.Id}, league.Notification{Type: "general", Title: "T", Body: "B"})

	assert.True(t, cap.hasMessage("notify player failed"),
		"save failure must be logged")

	notifs, _ := app.FindRecordsByFilter("notifications",
		"user = {:uid}", "", 0, 0, map[string]any{"uid": user.Id})
	assert.Empty(t, notifs, "no notification persisted when save fails")
}

func TestNotifyAdmins_SaveError_LogsError(t *testing.T) {
	cap := withLogCapture(t)
	app := newTestApp(t)
	notifier := NewNotifier(app, "", "")
	notifier.save = func(*core.Record) error { return errors.New("injected") }

	admin := makeUser(t, app, "admin")
	err := notifier.NotifyAdmins(league.Notification{Type: "general", Title: "Notice", Body: "Body"})
	require.NoError(t, err, "individual save failures must not propagate")

	assert.True(t, cap.hasMessage("notify admin failed"),
		"save failure must be logged")

	notifs, _ := app.FindRecordsByFilter("notifications",
		"user = {:uid}", "", 0, 0, map[string]any{"uid": admin.Id})
	assert.Empty(t, notifs, "no notification persisted when save fails")
}

func TestNotifyAdmins_ExcludesParticipants(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	notifier := NewNotifier(app, "", "")

	admin1 := makeUser(t, app, "admin")
	admin2 := makeUser(t, app, "admin")

	err := notifier.NotifyAdmins(league.Notification{Type: "match_progress", Title: "Progreso", Body: "Body"}, admin1.Id)
	require.NoError(t, err)

	notifs1, _ := app.FindRecordsByFilter("notifications",
		"user = {:uid}", "", 0, 0, map[string]any{"uid": admin1.Id})
	assert.Empty(t, notifs1, "excluded admin must not receive notification")

	notifs2, _ := app.FindRecordsByFilter("notifications",
		"user = {:uid}", "", 0, 0, map[string]any{"uid": admin2.Id})
	assert.Len(t, notifs2, 1, "non-excluded admin must receive notification")
}

func TestNotifyAdmins_HonorsPrefs(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	notifier := NewNotifier(app, "", "")

	admin := makeUser(t, app, "admin")
	admin.Set("notification_prefs", map[string]any{"match_progress": false})
	require.NoError(t, app.Save(admin))

	err := notifier.NotifyAdmins(league.Notification{Type: "match_progress", Title: "Progreso", Body: "Body"})
	require.NoError(t, err)

	notifs, _ := app.FindRecordsByFilter("notifications",
		"user = {:uid}", "", 0, 0, map[string]any{"uid": admin.Id})
	assert.Empty(t, notifs, "admin with match_progress=false must not be notified")
}

func TestPushTargetURL(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/", pushTargetURL(""))
	assert.Equal(t, "/match/abc123", pushTargetURL("abc123"))
}

func TestDeliverPush_DeleteError_Logs(t *testing.T) {
	cap := withLogCapture(t)
	app := newTestApp(t)
	srv, hits := pushServer(t, 410)
	priv, pub := vapidKeys(t)
	user := makeUser(t, app, "player")
	sub := makeSubscription(t, app, user.Id, srv.URL)

	notifier := NewNotifier(app, pub, priv)
	notifier.delete = func(*core.Record) error { return errors.New("injected") }

	notifier.sendPush(user.Id, "T", "B", "")

	require.Equal(t, int32(1), hits.Load())
	assert.True(t, cap.hasMessage("push delete subscription failed"),
		"delete failure must be logged")
	_, err := app.FindRecordById("push_subscriptions", sub.Id)
	assert.NoError(t, err, "subscription must survive when delete fails")
}
