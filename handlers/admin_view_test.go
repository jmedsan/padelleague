package handlers_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/handlers"
	"padelleague/notify"
)

// Owner rule: admin actions are seen and run only by admins in the admin
// view. An admin in player view (cookie view_as=player) is a pure player.

func playerViewHeaders(tb testing.TB, user *core.Record) map[string]string {
	tb.Helper()
	hdrs := handlers.AuthHeaders(tb, user)
	hdrs["Cookie"] = "view_as=player"
	return hdrs
}

func TestAdminPages_PlayerViewRedirectsHome(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "GET /admin in player view redirects to /",
		Method:         http.MethodGet,
		URL:            "/admin",
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		s.Headers = playerViewHeaders(tb, makeAdminUser(tb, app))
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/", res.Header.Get("Location"))
	}
	s.Test(t)
}

func TestAdminPages_PlayerViewHXRedirectsHome(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "HX GET /admin in player view answers HX-Redirect /",
		Method:         http.MethodGet,
		URL:            "/admin",
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		hdrs := playerViewHeaders(tb, makeAdminUser(tb, app))
		hdrs["HX-Request"] = "true"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/", res.Header.Get("HX-Redirect"))
		assert.Empty(tb, res.Header.Get("Location"))
	}
	s.Test(t)
}

// The admin view still reaches the admin pages: the cookie, not the role, is
// what flips access.
func TestAdminPages_AdminViewCookieAllowed(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /admin/competitions with view_as=admin renders",
		Method:          http.MethodGet,
		URL:             "/admin/competitions",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Competiciones"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		hdrs := handlers.AuthHeaders(tb, makeAdminUser(tb, app))
		hdrs["Cookie"] = "view_as=admin"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestAdminOverride_PlayerViewRefused(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /match/{id}/admin-override in player view changes nothing",
		Method:         http.MethodPost,
		ExpectedStatus: 302,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "AOPV A")
		p2 := handlers.MakePairTB(tb, app, "AOPV B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		matchID = handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending").Id
		s.URL = "/match/" + matchID + "/admin-override"
		s.Body = strings.NewReader("scores=6-3+6-4")
		hdrs := playerViewHeaders(tb, makeAdminUser(tb, app))
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/", res.Header.Get("Location"))
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "pending", m.GetString("status"))
		assert.Empty(tb, m.GetString("scores"))
		assert.Empty(tb, m.GetString("winner"))
		msgs, err := app.FindRecordsByFilter("match_messages", "match = {:id}", "", 0, 0, map[string]any{"id": matchID})
		require.NoError(tb, err)
		assert.Empty(tb, msgs)
	}
	s.Test(t)
}

func TestAdminRelease_PlayerViewRefused(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /match/{id}/release in player view changes nothing",
		Method:         http.MethodPost,
		ExpectedStatus: 302,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "RelPV A")
		p2 := handlers.MakePairTB(tb, app, "RelPV B")
		p3 := handlers.MakePairTB(tb, app, "RelPV C")
		comp := makeLeveledCompTB(tb, app, []*core.Record{p1, p2, p3})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		m.Set("round_number", 0)
		require.NoError(tb, app.Save(m))
		matchID = m.Id
		s.URL = "/match/" + matchID + "/release"
		s.Headers = playerViewHeaders(tb, makeAdminUser(tb, app))
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/", res.Header.Get("Location"))
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err, "the match must survive a release from the player view")
		assert.Equal(tb, "pending", m.GetString("status"))
		events, err := app.FindRecordsByFilter("competition_events", "kind = 'assignment_released'", "", 0, 0, nil)
		require.NoError(tb, err)
		assert.Empty(tb, events)
	}
	s.Test(t)
}

func TestCloseArbitration_PlayerViewRefused(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /match/{id}/arbitration/close in player view changes nothing",
		Method:         http.MethodPost,
		ExpectedStatus: 302,
	}
	var matchID, requesterID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "CloseArbPV A")
		p2 := handlers.MakePairTB(tb, app, "CloseArbPV B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		requesterID = p1.GetString("player1")
		match.Set("arbitration", "scheduling")
		match.Set("arbitration_by", requesterID)
		match.Set("dispute_notes", "no acordamos fecha")
		require.NoError(tb, app.Save(match))
		matchID = match.Id
		s.URL = "/match/" + matchID + "/arbitration/close"
		s.Headers = playerViewHeaders(tb, makeAdminUser(tb, app))
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/", res.Header.Get("Location"))
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "scheduling", m.GetString("arbitration"))
		assert.Equal(tb, requesterID, m.GetString("arbitration_by"))
		assert.Equal(tb, "no acordamos fecha", m.GetString("dispute_notes"))
		assert.Equal(tb, "pending", m.GetString("status"))
		notifs, err := app.FindRecordsByFilter("notifications", "related_match = {:m}", "", 0, 0, map[string]any{"m": matchID})
		require.NoError(tb, err)
		assert.Empty(tb, notifs)
	}
	s.Test(t)
}

// The test request's Host is example.com (httptest.NewRequest).
func TestViewSwitch_Destination(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, mode, referer, want string
	}{
		{"absolute same-host Referer keeps path and query", "player", "http://example.com/competition/abc?tab=2", "/competition/abc?tab=2"},
		{"absolute foreign-host Referer goes home", "player", "http://evil.example.org/competition/abc", "/"},
		{"scheme without host goes home", "player", "ftp:/competition/abc", "/"},
		{"opaque Referer goes home", "player", "mailto:x@example.com", "/"},
		{"relative path without leading slash goes home", "player", "competition/abc", "/"},
		{"same-host double-slash path goes home", "player", "http://example.com//evil.example.org/x", "/"},
		{"no Referer goes home", "player", "", "/"},
		{"player view from an admin subpage goes home", "player", "http://example.com/admin/competitions", "/"},
		{"player view from the admin root goes home", "player", "http://example.com/admin", "/"},
		{"player view from a non-admin path starting with admin keeps it", "player", "http://example.com/administrar", "/administrar"},
		{"admin view from an admin subpage returns to it", "admin", "http://example.com/admin/competitions", "/admin/competitions"},
		{"unknown mode is the admin view", "bogus", "http://example.com/admin/competitions", "/admin/competitions"},
	}
	for _, tc := range cases {
		s := &tests.ApiScenario{
			TestAppFactory: handlers.TestAppFactory,
			Name:           "view switch: " + tc.name,
			Method:         http.MethodGet,
			URL:            "/view/" + tc.mode,
			ExpectedStatus: 302,
		}
		s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
			setupProductionRoutes(tb, app, e)
			s.Headers = handlers.AuthHeaders(tb, makeAdminUser(tb, app))
			if tc.referer != "" {
				s.Headers["Referer"] = tc.referer
			}
		}
		s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
			assert.Equal(tb, tc.want, res.Header.Get("Location"))
			wantCookie := "view_as=" + tc.mode
			if tc.mode == "bogus" {
				wantCookie = "view_as=admin"
			}
			assert.Contains(tb, res.Header.Get("Set-Cookie"), wantCookie)
		}
		s.Test(t)
	}
}

// The footer lists the active competitions: every one in the admin view, only
// the admin's own in the player view.
func TestFooter_FollowsView(t *testing.T) {
	t.Parallel()
	for _, view := range []string{"player", "admin"} {
		s := &tests.ApiScenario{
			TestAppFactory: handlers.TestAppFactory,
			Name:           "footer competitions in " + view + " view",
			Method:         http.MethodGet,
			URL:            "/profile/notifications",
			ExpectedStatus: 200,
		}
		var foreignID string
		s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
			setupProductionRoutes(tb, app, e)
			own, _, adminPlayer := gateAdminPlayerTB(tb, app, "Footer"+view)
			q1 := handlers.MakePairTB(tb, app, "FooterForeign A "+view)
			q2 := handlers.MakePairTB(tb, app, "FooterForeign B "+view)
			foreignID = handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{q1, q2}).Id
			s.ExpectedContent = []string{`href="/competition/` + own.Id + `"`}
			hdrs := handlers.AuthHeaders(tb, adminPlayer)
			hdrs["Cookie"] = "view_as=" + view
			s.Headers = hdrs
		}
		s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
			body, err := io.ReadAll(res.Body)
			require.NoError(tb, err)
			foreign := `href="/competition/` + foreignID + `"`
			if view == "player" {
				assert.NotContains(tb, string(body), foreign)
			} else {
				assert.Contains(tb, string(body), foreign)
			}
		}
		s.Test(t)
	}
}

// The form hides the admin-only toggles outside the admin view, so a save in
// player view must keep their stored values; in the admin view their absence
// means unchecked.
func TestNotificationPrefsSave_AdminTogglesFollowView(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		view string
		want bool
	}{{"player", true}, {"admin", false}} {
		var userID string
		s := &tests.ApiScenario{
			TestAppFactory: handlers.TestAppFactory,
			Name:           "admin prefs saved in " + tc.view + " view",
			Method:         http.MethodPost,
			URL:            "/profile/notifications",
			Body:           strings.NewReader("general=on"),
			ExpectedStatus: 204,
		}
		s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
			setupProductionRoutes(tb, app, e)
			_, _, adminPlayer := gateAdminPlayerTB(tb, app, "Prefs"+tc.view)
			adminPlayer.Set("notification_prefs", map[string]any{
				"admin_message": true, "user_joined": true, "match_progress": true,
			})
			require.NoError(tb, app.Save(adminPlayer))
			userID = adminPlayer.Id
			hdrs := handlers.AuthHeaders(tb, adminPlayer)
			hdrs["Cookie"] = "view_as=" + tc.view
			hdrs["Content-Type"] = "application/x-www-form-urlencoded"
			s.Headers = hdrs
		}
		s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
			user, err := app.FindRecordById("users", userID)
			require.NoError(tb, err)
			prefs := notify.NotificationPrefs(user)
			for _, k := range []string{"admin_message", "user_joined", "match_progress"} {
				assert.Equal(tb, tc.want, prefs[k], k)
			}
		}
		handlers.ExpectRedirect(s, redirectTo("/profile/notifications"))
		s.Test(t)
	}
}
