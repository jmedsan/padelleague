package handlers

import (
	"io"
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationCount_UnreadOnly(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	user := makeUser(t, app, "Notif User", "")

	makeNotification(t, app, user.Id, "Unread 1", "body1", false)
	makeNotification(t, app, user.Id, "Unread 2", "body2", false)
	makeNotification(t, app, user.Id, "Read 1", "body3", true)

	unread, err := app.FindRecordsByFilter("notifications",
		"user = {:uid} && read = false",
		"", 0, 0,
		map[string]any{"uid": user.Id})
	require.NoError(t, err)
	assert.Equal(t, 2, len(unread))
}

func TestNotificationList_OrderAndLimit(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	user := makeUser(t, app, "List User", "")

	for i := 0; i < 12; i++ {
		makeNotification(t, app, user.Id, "Notif", "body", false)
	}

	records, err := app.FindRecordsByFilter("notifications",
		"user = {:uid}",
		"", 10, 0,
		map[string]any{"uid": user.Id})
	require.NoError(t, err)
	assert.Equal(t, 10, len(records), "should return at most 10")
}

func TestNotificationCount_OtherUserExcluded(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	user1 := makeUser(t, app, "User 1", "")
	user2 := makeUser(t, app, "User 2", "")

	makeNotification(t, app, user1.Id, "For user1", "body", false)
	makeNotification(t, app, user2.Id, "For user2", "body", false)

	unread, err := app.FindRecordsByFilter("notifications",
		"user = {:uid} && read = false",
		"", 0, 0,
		map[string]any{"uid": user1.Id})
	require.NoError(t, err)
	assert.Equal(t, 1, len(unread))
}

// MarkRead: notification with related_match redirects to /match/{id} (line 102)

func TestMarkReadNotificationWithRelatedMatch(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /notifications/{id}/read with related_match redirects to match",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var notifID, matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupNotifRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Match Notif", "")
		p1 := makePairTB(tb, app, "MNA")
		p2 := makePairTB(tb, app, "MNB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id

		n := makeNotification(t, app, user.Id, "Partido asignado", "", false)
		n.Set("related_match", match.Id)
		require.NoError(tb, app.Save(n))
		notifID = n.Id

		s.URL = "/notifications/" + n.Id + "/read"
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/match/"+matchID, res.Header.Get("HX-Redirect"))
		n, err := app.FindRecordById("notifications", notifID)
		require.NoError(tb, err)
		assert.True(tb, n.GetBool("read"), "notification must be marked read")
		assert.True(tb, n.GetBool("dismissed"), "notification must be dismissed")
	}
	s.Test(t)
}

// MarkRead: notification without related_match redirects to / (existing but assert DB state)

func TestMarkReadNotificationNoRelatedMatch(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /notifications/{id}/read without related_match redirects to /",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var notifID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupNotifRoutes(tb, app, e)
		user := makeUserTB(tb, app, "No Match Notif", "")
		n := makeNotification(t, app, user.Id, "General", "Some body", false)
		notifID = n.Id
		s.URL = "/notifications/" + n.Id + "/read"
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/", res.Header.Get("HX-Redirect"))
		n, err := app.FindRecordById("notifications", notifID)
		require.NoError(tb, err)
		assert.True(tb, n.GetBool("read"), "notification must be marked read")
		assert.True(tb, n.GetBool("dismissed"), "notification must be dismissed")
	}
	s.Test(t)
}

// Count: zero unread returns empty badge (line 37)

func TestNotificationCountZero(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "GET /notifications/count with zero unread returns empty",
		Method:         http.MethodGet,
		URL:            "/notifications/count",
		ExpectedStatus: 200,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupNotifRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Zero Count", "")
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		// Empty response — no badge element
		assert.NotContains(tb, res.Header.Get("Content-Type"), "badge")
	}
	s.Test(t)
}

// Count: has unread returns badge with count (lines 33-34)

func TestNotificationCountWithUnread(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /notifications/count with unread returns badge",
		Method:          http.MethodGet,
		URL:             "/notifications/count",
		ExpectedStatus:  200,
		ExpectedContent: []string{"badge", "2"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupNotifRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Has Unread", "")
		makeNotification(t, app, user.Id, "N1", "", false)
		makeNotification(t, app, user.Id, "N2", "", false)
		makeNotification(t, app, user.Id, "N3", "", true) // read, should not count
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

// List: notification with body text (line 70)

func TestNotificationListWithBody(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /notifications/list shows body text",
		Method:          http.MethodGet,
		URL:             "/notifications/list",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Título de prueba", "Cuerpo del mensaje"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupNotifRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Body Notif", "")
		makeNotification(t, app, user.Id, "Título de prueba", "Cuerpo del mensaje", false)
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

// List: empty notification list (line 60)

func TestNotificationListEmpty(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /notifications/list with no notifications shows empty message",
		Method:          http.MethodGet,
		URL:             "/notifications/list",
		ExpectedStatus:  200,
		ExpectedContent: []string{"No tienes notificaciones"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupNotifRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Empty List", "")
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

// MarkAllRead: asserts notifications actually marked read in DB

func TestMarkAllReadVerifyDB(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /notifications/read-all marks all read in DB",
		Method:         http.MethodPost,
		URL:            "/notifications/read-all",
		ExpectedStatus: 204,
	}
	var userID string
	var notifIDs []string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupNotifRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Bulk Read DB", "")
		userID = user.Id
		n1 := makeNotification(t, app, user.Id, "B1", "", false)
		n2 := makeNotification(t, app, user.Id, "B2", "", false)
		n3 := makeNotification(t, app, user.Id, "B3", "", false)
		notifIDs = []string{n1.Id, n2.Id, n3.Id}
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		for _, id := range notifIDs {
			n, err := app.FindRecordById("notifications", id)
			require.NoError(tb, err)
			assert.True(tb, n.GetBool("read"), "notification %s must be marked read", id)
			assert.True(tb, n.GetBool("dismissed"), "notification %s must be dismissed", id)
		}
		unread, err := app.FindRecordsByFilter("notifications",
			"user = {:uid} && read = false", "", 0, 0,
			map[string]any{"uid": userID})
		require.NoError(tb, err)
		assert.Equal(tb, 0, len(unread))
	}
	s.Test(t)
}

// List: unread notification has highlight class, read does not

func TestNotificationListReadVsUnread(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /notifications/list distinguishes read/unread",
		Method:          http.MethodGet,
		URL:             "/notifications/list",
		ExpectedStatus:  200,
		ExpectedContent: []string{"bg-primary/5", "Unread Title"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupNotifRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Read Unread", "")
		makeNotification(t, app, user.Id, "Unread Title", "", false)
		makeNotification(t, app, user.Id, "Read Title", "", true)
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestDismissNotification(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /notifications/{id}/dismiss sets dismissed and returns OOB badges",
		Method:         http.MethodPost,
		ExpectedStatus: 200,
		ExpectedContent: []string{
			`id="notif-badge"`,
			`id="notif-badge-mobile"`,
			`hx-swap-oob="innerHTML"`,
		},
	}
	var notifID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupNotifRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Dismiss User", "")
		n1 := makeNotification(t, app, user.Id, "To Dismiss", "", false)
		makeNotification(t, app, user.Id, "Keep", "", false)
		notifID = n1.Id
		s.URL = "/notifications/" + n1.Id + "/dismiss"
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		n, err := app.FindRecordById("notifications", notifID)
		require.NoError(tb, err)
		assert.True(tb, n.GetBool("dismissed"), "notification must be dismissed")
		assert.False(tb, n.GetBool("read"), "dismiss should not mark as read")

		body, _ := io.ReadAll(res.Body)
		assert.Contains(tb, string(body), ">1<", "badge should show 1 remaining unread")
	}
	s.Test(t)
}

func TestDismissNotificationOtherUser(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /notifications/{id}/dismiss by non-owner returns 204",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupNotifRoutes(tb, app, e)
		owner := makeUserTB(tb, app, "Owner", "")
		other := makeUserTB(tb, app, "Other", "")
		n := makeNotification(t, app, owner.Id, "Private", "", false)
		s.URL = "/notifications/" + n.Id + "/dismiss"
		s.Headers = authHeaders(tb, other)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		recs, err := app.FindRecordsByFilter("notifications",
			"dismissed = true", "", 0, 0, nil)
		require.NoError(tb, err)
		assert.Equal(tb, 0, len(recs), "non-owner dismiss must not modify anything")
	}
	s.Test(t)
}
