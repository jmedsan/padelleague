package handlers

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
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

	e.Router.GET("/profile/notifications", notif.Prefs).BindFunc(requireAuthTest)
	e.Router.POST("/profile/notifications", notif.PrefsSave).BindFunc(requireAuthTest)

	push := NewPushHandler(app, notifier)
	e.Router.POST("/push/subscribe", push.Subscribe).BindFunc(requireAuthTest)
	e.Router.POST("/push/unsubscribe", push.Unsubscribe).BindFunc(requireAuthTest)
}

func TestMarkReadNotification(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /notifications/{id}/read marks read and redirects",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupNotifRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Notif Reader", "")
		n := makeNotification(t, app, user.Id, "Test", "Body", false)
		s.URL = "/notifications/" + n.Id + "/read"
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestMarkAllReadNotifications(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /notifications/read-all marks all read",
		Method:         http.MethodPost,
		URL:            "/notifications/read-all",
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupNotifRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Notif Bulk", "")
		makeNotification(t, app, user.Id, "N1", "Body1", false)
		makeNotification(t, app, user.Id, "N2", "Body2", false)
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestNotificationPrefsPage(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "GET /profile/notifications returns prefs page",
		Method:          http.MethodGet,
		URL:             "/profile/notifications",
		ExpectedStatus:  200,
		ExpectedContent: []string{"PadelLeague"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupNotifRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Prefs User", "")
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestNotificationPrefsSave(t *testing.T) {
	s := &tests.ApiScenario{
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
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestPushSubscribeHTTPS(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /push/subscribe with https endpoint succeeds",
		Method:         http.MethodPost,
		URL:            "/push/subscribe",
		Body:           strings.NewReader(`{"endpoint":"https://push.example.com/sub","keys":{"p256dh":"key1","auth":"key2"}}`),
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupNotifRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Push User", "")
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/json"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestPushSubscribeHTTPRejected(t *testing.T) {
	s := &tests.ApiScenario{
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
	s := &tests.ApiScenario{
		Name:           "POST /push/unsubscribe removes subscription",
		Method:         http.MethodPost,
		URL:            "/push/unsubscribe",
		Body:           strings.NewReader(`{"endpoint":"https://push.example.com/sub"}`),
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupNotifRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Unsub User", "")
		// Create a subscription first
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
	s.Test(t)
}
