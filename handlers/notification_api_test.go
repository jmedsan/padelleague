package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/league"
	"padelleague/middleware"
	"padelleague/notify"
	"padelleague/render"
)

func setupNotifRoutes(_ testing.TB, app *tests.TestApp, e *core.ServeEvent) {
	viewsFS := os.DirFS("..")
	r := render.New(viewsFS, "")
	notifier := notify.NewNotifier(app, "", "")
	svc := league.New(app, notifier)
	_ = svc

	e.Router.BindFunc(middleware.CookieAuth)

	auth := NewAuthHandler(app, r.Page)
	e.Router.GET("/login", auth.Login)

	notif := NewNotificationHandler(app, r.Page)
	e.Router.GET("/notifications/count", notif.Count).BindFunc(requireAuthTest)
	e.Router.GET("/notifications/list", notif.List).BindFunc(requireAuthTest)
	e.Router.POST("/notifications/{id}/read", notif.MarkRead).BindFunc(requireAuthTest)
	e.Router.POST("/notifications/read-all", notif.MarkAllRead).BindFunc(requireAuthTest)
	e.Router.POST("/notifications/{id}/dismiss", notif.Dismiss).BindFunc(requireAuthTest)

	e.Router.GET("/profile/notifications", notif.Prefs).BindFunc(requireAuthTest)
	e.Router.POST("/profile/notifications", notif.PrefsSave).BindFunc(requireAuthTest)

	push := NewPushHandler(app, notifier)
	e.Router.POST("/push/subscribe", push.Subscribe).BindFunc(requireAuthTest)
	e.Router.POST("/push/unsubscribe", push.Unsubscribe).BindFunc(requireAuthTest)
}

func TestNotificationListReturnsNewestFirst(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:     testAppFactory,
		Name:               "GET /notifications/list returns newest 10 of 11",
		Method:             http.MethodGet,
		URL:                "/notifications/list",
		ExpectedStatus:     200,
		ExpectedContent:    []string{"Notif-10", "Notif-01"},
		NotExpectedContent: []string{"Notif-00"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupNotifRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Order User", "")
		s.Headers = authHeaders(tb, user)

		base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		for i := 0; i < 11; i++ {
			col, err := app.FindCollectionByNameOrId("notifications")
			require.NoError(tb, err)
			n := core.NewRecord(col)
			n.Set("user", user.Id)
			n.Set("type", "general")
			n.Set("title", fmt.Sprintf("Notif-%02d", i))
			n.Set("body", "body")
			n.Set("read", false)
			ts, _ := types.ParseDateTime(base.Add(time.Duration(i) * time.Minute))
			n.SetRaw("created", ts)
			require.NoError(tb, app.Save(n))
		}
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body, err := io.ReadAll(res.Body)
		require.NoError(tb, err)
		html := string(body)
		idx10 := strings.Index(html, "Notif-10")
		idx01 := strings.Index(html, "Notif-01")
		assert.Greater(tb, idx10, -1, "newest notification must be present")
		assert.Greater(tb, idx01, -1, "second oldest must be present")
		assert.Less(tb, idx10, idx01, "newest (Notif-10) must appear before oldest kept (Notif-01)")
	}
	s.Test(t)
}

func TestMarkReadNotification(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /notifications/{id}/read marks read and redirects",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var notifID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupNotifRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Notif Reader", "")
		n := makeNotification(t, app, user.Id, "Test", "Body", false)
		notifID = n.Id
		s.URL = "/notifications/" + n.Id + "/read"
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		n, err := app.FindRecordById("notifications", notifID)
		require.NoError(tb, err)
		assert.Equal(tb, true, n.GetBool("read"))
		assert.Equal(tb, "/", res.Header.Get("HX-Redirect"))
	}
	s.Test(t)
}

func TestMarkAllReadNotifications(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /notifications/read-all marks all read",
		Method:         http.MethodPost,
		URL:            "/notifications/read-all",
		ExpectedStatus: 204,
	}
	var userID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupNotifRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Notif Bulk", "")
		userID = user.Id
		makeNotification(t, app, user.Id, "N1", "Body1", false)
		makeNotification(t, app, user.Id, "N2", "Body2", false)
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		unread, err := app.FindRecordsByFilter("notifications",
			"user = {:uid} && read = false", "", 0, 0,
			map[string]any{"uid": userID})
		require.NoError(tb, err)
		assert.Equal(tb, 0, len(unread), "all notifications must be marked read")
		assert.Equal(tb, "/", res.Header.Get("HX-Redirect"))
	}
	s.Test(t)
}

func TestNotificationPrefsPage(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "GET /profile/notifications returns prefs page with toggles",
		Method:         http.MethodGet,
		URL:            "/profile/notifications",
		ExpectedStatus: 200,
		ExpectedContent: []string{
			`name="general"`,
			`name="quorum_request"`,
			`name="dispute"`,
			`name="match_assigned"`,
			`name="scheduling"`,
		},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupNotifRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Prefs User", "")
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestNotificationPrefsSave(t *testing.T) {
	t.Parallel()
	var userID string
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /profile/notifications saves prefs",
		Method:          http.MethodPost,
		URL:             "/profile/notifications",
		Body:            strings.NewReader("quorum_request=on&dispute=on"),
		ExpectedStatus:  200,
		ExpectedContent: []string{"PadelLeague"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupNotifRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Prefs Saver", "")
		userID = user.Id
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		user, err := app.FindRecordById("users", userID)
		require.NoError(tb, err)
		prefs := notify.NotificationPrefs(user)
		// Only the two checked boxes were submitted; the rest must be off.
		assert.Equal(tb, true, prefs["quorum_request"])
		assert.Equal(tb, true, prefs["dispute"])
		assert.Equal(tb, false, prefs["general"])
		assert.Equal(tb, false, prefs["match_assigned"])
		assert.Equal(tb, false, prefs["scheduling"])
	}
	s.Test(t)
}

// The prefs page must render what was saved, not the all-on defaults.
func TestNotificationPrefsPageReflectsSavedPrefs(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:     testAppFactory,
		Name:               "GET /profile/notifications renders a disabled toggle unchecked",
		Method:             http.MethodGet,
		URL:                "/profile/notifications",
		ExpectedStatus:     200,
		ExpectedContent:    []string{`name="dispute" class="toggle toggle-primary" checked`},
		NotExpectedContent: []string{`name="general" class="toggle toggle-primary" checked`},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupNotifRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Prefs Reader", "")
		user.Set("notification_prefs", map[string]any{
			"quorum_request": true,
			"dispute":        true,
			"match_assigned": true,
			"general":        false,
			"scheduling":     true,
		})
		require.NoError(tb, app.Save(user))
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestPushSubscribeHTTPS(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /push/subscribe with https endpoint succeeds",
		Method:         http.MethodPost,
		URL:            "/push/subscribe",
		Body:           strings.NewReader(`{"endpoint":"https://push.example.com/sub","keys":{"p256dh":"key1","auth":"key2"}}`),
		ExpectedStatus: 204,
	}
	var userID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupNotifRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Push User", "")
		userID = user.Id
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/json"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		subs, err := app.FindRecordsByFilter("push_subscriptions",
			"user = {:uid}", "", 0, 0,
			map[string]any{"uid": userID})
		require.NoError(tb, err)
		require.Equal(tb, 1, len(subs))
		assert.Equal(tb, "https://push.example.com/sub", subs[0].GetString("endpoint"))
		assert.Equal(tb, "key1", subs[0].GetString("p256dh"))
		assert.Equal(tb, "key2", subs[0].GetString("auth"))
	}
	s.Test(t)
}

func TestPushSubscribeHTTPRejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /push/subscribe with http endpoint fails",
		Method:          http.MethodPost,
		URL:             "/push/subscribe",
		Body:            strings.NewReader(`{"endpoint":"http://push.example.com/sub","keys":{"p256dh":"key1","auth":"key2"}}`),
		ExpectedStatus:  400,
		ExpectedContent: []string{"Endpoint must use https"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupNotifRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Push Bad", "")
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/json"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestPushUnsubscribe(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /push/unsubscribe removes subscription",
		Method:         http.MethodPost,
		URL:            "/push/unsubscribe",
		Body:           strings.NewReader(`{"endpoint":"https://push.example.com/sub"}`),
		ExpectedStatus: 204,
	}
	var userID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupNotifRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Unsub User", "")
		userID = user.Id
		col, err := app.FindCollectionByNameOrId("push_subscriptions")
		require.NoError(tb, err)
		rec := core.NewRecord(col)
		rec.Set("user", user.Id)
		rec.Set("endpoint", "https://push.example.com/sub")
		rec.Set("p256dh", "key1")
		rec.Set("auth", "key2")
		require.NoError(tb, app.Save(rec))

		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/json"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		subs, err := app.FindRecordsByFilter("push_subscriptions",
			"user = {:uid}", "", 0, 0,
			map[string]any{"uid": userID})
		require.NoError(tb, err)
		assert.Equal(tb, 0, len(subs), "subscription must be deleted")
	}
	s.Test(t)
}
