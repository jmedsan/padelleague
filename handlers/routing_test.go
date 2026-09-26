package handlers_test

// Shared support for every handlers_test file that exercises real HTTP
// routes. setupProductionRoutes is the ONLY sanctioned way to wire routes in
// this package — see TestRoutesGoThroughProductionSetup in
// route_setup_invariant_test.go, which fails a test file that builds its own
// router instead.
//
// Fixture helpers used by BOTH this package and the pure-logic tests that
// stayed in package handlers (makeUserTB, authHeaders, etc.) are bridged via
// handlers.MakeUserTB and friends in export_test.go, so they're defined once.
// Helpers below are exclusive to this package's route-driven tests.

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/handlers"
	"padelleague/league"
	"padelleague/notify"
	"padelleague/render"
	"padelleague/routes"
	"padelleague/search"
)

// setupProductionRoutes registers every route through routes.Register — the
// real production entry point — with a test-constructed routes.Deps. Devtools
// routes are enabled (AppDevTools: true, AppEnv: "test") so tests can exercise
// them; individual tests that need devtools disabled construct their own Deps.
// It returns the *search.Index wired into the routes, so tests that need to
// seed search results (seedSearchIndex, ix.Replace) act on the same instance
// the handler actually queries.
//
// The notifier is built with real VAPID keys, not empty strings: routes.Register
// only wires /push/subscribe and /push/unsubscribe when push.Enabled() is true
// (mirroring production, which skips them without configured VAPID keys), so
// empty keys here would silently 404 every push test instead of exercising the
// real route.
func setupProductionRoutes(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) *search.Index {
	tb.Helper()
	viewsFS := os.DirFS("..")
	r := render.New(viewsFS, "", true)
	vapidPrivate, vapidPublic, err := webpush.GenerateVAPIDKeys()
	require.NoError(tb, err)
	notifier := notify.NewNotifier(app, vapidPublic, vapidPrivate)
	svc := league.New(app, notifier)
	ix := &search.Index{}
	routes.Register(e, routes.Deps{
		App:         app,
		Renderer:    r,
		Notifier:    notifier,
		LeagueSvc:   svc,
		SearchIndex: ix,
		StaticFS:    viewsFS,
		AppDevTools: true,
		AppEnv:      "test",
		Version:     "test",
	})
	// deliver fires push sends in a background goroutine; ApiScenario tears
	// down the TestApp's DB as soon as the request round trip and
	// AfterTestFunc return, so an in-flight push goroutine can otherwise
	// call FindRecordsByFilter on an already-closed app and segfault. A
	// t.Cleanup here would run too late (testApp.Cleanup fires via a defer
	// inside ApiScenario.Test, before the test function itself returns), so
	// drain synchronously right after the handler responds instead.
	e.Router.Bind(&hook.Handler[*core.RequestEvent]{
		Func: func(re *core.RequestEvent) error {
			err := re.Next()
			notifier.WaitPush()
			return err
		},
		Priority: 9999,
	})
	return ix
}

func makeInvitation(t testing.TB, app core.App, expiresAt time.Time) *core.Record {
	t.Helper()
	creator := handlers.MakeUser(t, app, "Inviter", "")
	comp := handlers.MakeCompetition(t, app, nil)
	col, err := app.FindCollectionByNameOrId("invitations")
	require.NoError(t, err)
	n := handlers.UserSeq.Add(1)
	record := core.NewRecord(col)
	record.Set("token", fmt.Sprintf("tok%d", n))
	record.Set("created_by", creator.Id)
	record.Set("competition", comp.Id)
	record.Set("status", "pending")
	if !expiresAt.IsZero() {
		record.Set("expires_at", expiresAt.UTC().Format("2006-01-02 15:04:05.000Z"))
	}
	require.NoError(t, app.Save(record))
	return record
}

func makeNotification(t testing.TB, app core.App, userID, title, body string, read bool) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("notifications")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Set("user", userID)
	record.Set("type", "general")
	record.Set("title", title)
	record.Set("body", body)
	record.Set("read", read)
	require.NoError(t, app.Save(record))
	return record
}

// assertNotified asserts that userID received EXACTLY ONE notification titled
// want.Title, and that it matches want's type/body/comp_name/related_match/
// link exactly. want's literal fields must come from the test's own fixture
// data, never from calling the constructor again — otherwise a handler that
// wires the wrong constructor, or drops a field before calling
// notify.Notifier, still passes because both sides recompute the same
// (possibly wrong) string. Exactly-one (not "at least one, take the newest")
// catches a duplicate send that a >=1 check would silently let through.
func assertNotified(t testing.TB, app core.App, userID string, want league.Notification) *core.Record {
	t.Helper()
	recs, err := app.FindRecordsByFilter("notifications",
		"user = {:user} && title = {:title}", "-created", 0, 0,
		map[string]any{"user": userID, "title": want.Title})
	require.NoError(t, err)
	require.Lenf(t, recs, 1, "expected exactly 1 notification titled %q for user %s, got %d", want.Title, userID, len(recs))
	rec := recs[0]
	assert.Equal(t, want.Type, rec.GetString("type"), "notification type")
	assert.Equal(t, want.Body, rec.GetString("body"), "notification body")
	assert.Equal(t, want.CompName, rec.GetString("comp_name"), "notification comp_name")
	if want.Link != "" {
		assert.Equal(t, want.Link, rec.GetString("link"), "notification link")
	} else if want.MatchID != "" {
		assert.Equal(t, want.MatchID, rec.GetString("related_match"), "notification related_match")
		assert.Equal(t, "/match/"+want.MatchID, rec.GetString("link"), "notification link (derived from MatchID)")
	}
	return rec
}

// assertNotNotified asserts that userID received NO notification titled
// title — the counterpart to assertNotified, for recipients a flow must
// exclude (e.g. the submitter must not get their own "result submitted").
func assertNotNotified(t testing.TB, app core.App, userID, title string) {
	t.Helper()
	recs, err := app.FindRecordsByFilter("notifications",
		"user = {:user} && title = {:title}", "", 0, 0,
		map[string]any{"user": userID, "title": title})
	require.NoError(t, err)
	assert.Emptyf(t, recs, "expected no notification titled %q for user %s, got %d", title, userID, len(recs))
}

// authToken is bridged from package handlers as handlers.AuthToken — it
// stays there because authHeaders (also shared) calls it.

func makePairWithGendersTB(t testing.TB, app core.App, name, g1, g2 string) *core.Record {
	t.Helper()
	n := handlers.PairSeq.Add(1)
	u1 := handlers.MakeUserTB(t, app, name+" P1", fmt.Sprintf("pair%dp1@test.local", n))
	if g1 != "" {
		u1.Set("gender", g1)
		require.NoError(t, app.Save(u1))
	}
	u2 := handlers.MakeUserTB(t, app, name+" P2", fmt.Sprintf("pair%dp2@test.local", n))
	if g2 != "" {
		u2.Set("gender", g2)
		require.NoError(t, app.Save(u2))
	}
	col, err := app.FindCollectionByNameOrId("pairs")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Set("name", name)
	record.Set("player1", u1.Id)
	record.Set("player2", u2.Id)
	require.NoError(t, app.Save(record))
	return record
}

func insertMatchReminder(t testing.TB, app core.App, matchID, userID string) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("match_reminders")
	require.NoError(t, err)
	rec := core.NewRecord(col)
	rec.Set("match", matchID)
	rec.Set("user", userID)
	rec.Set("hours_before", 26)
	require.NoError(t, app.Save(rec))
}

// redirectTo is the handlers.ExpectRedirect want for a fixed target.
func redirectTo(url string) func(core.App) string {
	return func(core.App) string { return url }
}

// competitionDetailURL trims a /admin/competitions/{id}/... action URL to the
// detail page every competition mutation redirects back to.
func competitionDetailURL(url string) string {
	return adminEntityURL(url, "/admin/competitions/")
}

// matchPageURL trims a /match/{id}/... action URL to the match page.
func matchPageURL(url string) string {
	return adminEntityURL(url, "/match/")
}

func adminEntityURL(url, prefix string) string {
	rest := strings.TrimPrefix(url, prefix)
	if i := strings.IndexAny(rest, "/?"); i >= 0 {
		rest = rest[:i]
	}
	return prefix + rest
}

// matchCompetitionID resolves the competition of the match named in a
// /match/{id}/... or /admin/disputes/{id}/... URL.
func matchCompetitionID(app core.App, url string) string {
	id := strings.Split(strings.TrimPrefix(strings.TrimPrefix(url, "/admin/disputes/"), "/match/"), "/")[0]
	m, err := app.FindRecordById("matches", id)
	if err != nil {
		return "(match " + id + " not found)"
	}
	return m.GetString("competition")
}

// newestCompetitionID returns the id of the most recently created
// competition, for redirects to the page the handler itself just created.
func newestCompetitionID(app core.App) string {
	recs, err := app.FindAllRecords("competitions")
	if err != nil || len(recs) == 0 {
		return "(no competitions)"
	}
	newest := recs[0]
	for _, r := range recs[1:] {
		if r.GetDateTime("created").After(newest.GetDateTime("created")) {
			newest = r
		}
	}
	return newest.Id
}

// readBody is bridged from package handlers as handlers.ReadBody — it stays
// there because api_security_test.go (package handlers) also uses it.
