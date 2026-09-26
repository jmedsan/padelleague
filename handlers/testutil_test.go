package handlers

import (
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/internal/testapp"
	"padelleague/league"
	"padelleague/middleware"
	_ "padelleague/migrations"
	"padelleague/notify"
	"padelleague/render"
)

var (
	pairSeq atomic.Int64
	userSeq atomic.Int64
)

func TestMain(m *testing.M) {
	os.Exit(testapp.Run(m))
}

func makeUserTB(t testing.TB, app core.App, displayName, email string) *core.Record {
	t.Helper()
	n := userSeq.Add(1)
	col, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	if email == "" {
		email = fmt.Sprintf("user%d@test.local", n)
	}
	record := core.NewRecord(col)
	record.Set("email", email)
	record.Set("username", fmt.Sprintf("huser%d", n))
	record.Set("display_name", displayName)
	record.Set("roles", []string{"player"})
	record.SetPassword("testpass123456")
	record.SetVerified(true)
	require.NoError(t, app.Save(record))
	return record
}

func makeUser(t testing.TB, app core.App, displayName, email string) *core.Record {
	t.Helper()
	n := userSeq.Add(1)
	col, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	if email == "" {
		email = fmt.Sprintf("user%d@test.local", n)
	}
	record := core.NewRecord(col)
	record.Set("email", email)
	record.Set("username", fmt.Sprintf("huser%d", n))
	record.Set("display_name", displayName)
	record.Set("roles", []string{"player"})
	record.SetPassword("testpass123456")
	record.SetVerified(true)
	require.NoError(t, app.Save(record))
	return record
}

// testAppFactory gives an ApiScenario an app built from the pre-migrated
// template. Without it ApiScenario calls tests.NewTestApp() itself and pays
// the full migration cost on every scenario.
func testAppFactory(t testing.TB) *tests.TestApp {
	return testapp.Factory(t)
}

func newTestApp(t *testing.T) core.App {
	t.Helper()
	return testapp.New(t)
}

func makePair(t testing.TB, app core.App, name string) *core.Record {
	t.Helper()
	n := pairSeq.Add(1)
	u1 := makeUser(t, app, name+" P1", fmt.Sprintf("pair%dp1@test.local", n))
	u2 := makeUser(t, app, name+" P2", fmt.Sprintf("pair%dp2@test.local", n))
	col, err := app.FindCollectionByNameOrId("pairs")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Set("name", name)
	record.Set("player1", u1.Id)
	record.Set("player2", u2.Id)
	require.NoError(t, app.Save(record))
	return record
}

func makeCompetition(t testing.TB, app core.App, pairs []*core.Record) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("competitions")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Set("name", "Test Competition")
	record.Set("type", "league")
	record.Set("active", true)
	pairIDs := make([]string, len(pairs))
	for i, p := range pairs {
		pairIDs[i] = p.Id
	}
	record.Set("pairs", pairIDs)
	require.NoError(t, app.Save(record))
	return record
}

func makeMatch(t testing.TB, app core.App, compID, p1ID, p2ID, status string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("matches")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Set("competition", compID)
	record.Set("pair1", p1ID)
	record.Set("pair2", p2ID)
	record.Set("status", status)
	record.Set("round_number", 1)
	require.NoError(t, app.Save(record))
	return record
}

func makeFinalMatch(t testing.TB, app core.App, compID, p1ID, p2ID, score, winnerID string) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("matches")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Set("competition", compID)
	record.Set("pair1", p1ID)
	record.Set("pair2", p2ID)
	record.Set("status", "final")
	record.Set("scores", score)
	record.Set("winner", winnerID)
	record.Set("round_number", 1)
	require.NoError(t, app.Save(record))
}

func makeInvitation(t testing.TB, app core.App, expiresAt time.Time) *core.Record {
	t.Helper()
	creator := makeUser(t, app, "Inviter", "")
	comp := makeCompetition(t, app, nil)
	col, err := app.FindCollectionByNameOrId("invitations")
	require.NoError(t, err)
	n := userSeq.Add(1)
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

// assertNotified finds the notification record for userID with the given
// title (the most recently created one, since a flow can leave several
// notifications on the same user) and asserts its type, body, comp_name, and
// related_match/link exactly match want. want's literal fields must come
// from the test's own fixture data (the same values the flow under test was
// seeded/driven with), never from calling the constructor again — otherwise
// a handler that wires the wrong constructor, or drops a field before
// calling notify.Notifier, still passes because both sides recompute the
// same (possibly wrong) string.
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

func authToken(t testing.TB, user *core.Record) string {
	t.Helper()
	token, err := user.NewAuthToken()
	require.NoError(t, err)
	return token
}

func authHeaders(t testing.TB, user *core.Record) map[string]string {
	t.Helper()
	return map[string]string{
		"Authorization": authToken(t, user),
	}
}

func setupAuthRoutes(_ testing.TB, app *tests.TestApp, e *core.ServeEvent) {
	viewsFS := os.DirFS("..")
	r := render.New(viewsFS, "", true)
	auth := NewAuthHandler(app, nil, r.Page)
	e.Router.GET("/login", auth.Login)
	e.Router.POST("/login", auth.LoginSubmit)
	e.Router.GET("/register", auth.Register)
	e.Router.POST("/register", auth.RegisterSubmit)
	e.Router.POST("/logout", auth.Logout)
}

func setupAllRoutes(_ testing.TB, app *tests.TestApp, e *core.ServeEvent) {
	viewsFS := os.DirFS("..")
	r := render.New(viewsFS, "", true)
	notifier := notify.NewNotifier(app, "", "")
	svc := league.New(app, notifier)

	e.Router.BindFunc(middleware.CookieAuth)

	auth := NewAuthHandler(app, nil, r.Page)
	e.Router.GET("/login", auth.Login)
	e.Router.POST("/login", auth.LoginSubmit)
	e.Router.GET("/register", auth.Register)
	e.Router.POST("/register", auth.RegisterSubmit)
	e.Router.POST("/logout", auth.Logout)

	pwReset := NewPasswordResetHandler(app, r.Page)
	e.Router.GET("/forgot-password", pwReset.ForgotPassword)
	e.Router.POST("/forgot-password", pwReset.ForgotPasswordSubmit)
	e.Router.GET("/reset-password", pwReset.ResetPassword)
	e.Router.POST("/reset-password", pwReset.ResetPasswordSubmit)

	pub := NewPublicHandler(app, svc, PublicRenderers{Page: r.Page, ErrorPage: r.ErrorPage})
	e.Router.GET("/", pub.Home).BindFunc(requireAuthTest)
	e.Router.GET("/competition/{id}", pub.Competition).BindFunc(requireAuthTest)

	player := NewPlayerHandler(app, svc, PlayerRenderers{Page: r.Page, Partial: r.Partial, ErrorPage: r.ErrorPage})
	e.Router.GET("/player/{id}", player.Player).BindFunc(requireAuthTest)
	e.Router.POST("/player/{id}/avatar", player.PlayerAvatarUpload).BindFunc(requireAuthTest)

	match := NewMatchHandler(app, notifier, r.Page, r.ErrorPage)
	e.Router.GET("/match/{id}", match.MatchDetail).BindFunc(requireAuthTest)
	e.Router.POST("/match/{id}/submit", match.MatchSubmit).BindFunc(requireAuthTest)
	e.Router.POST("/match/{id}/correct", match.MatchCorrect).BindFunc(requireAuthTest)
	e.Router.POST("/match/{id}/admin-override", match.AdminOverride).BindFunc(requireAuthTest)
	e.Router.POST("/match/{id}/release", match.AdminRelease).BindFunc(requireAdminTest)
	e.Router.POST("/match/{id}/report-unplayed", match.ReportUnplayed).BindFunc(requireAuthTest)
	e.Router.POST("/match/{id}/cancel-date", match.CancelDate).BindFunc(requireAuthTest)

	thread := NewThreadHandler(ThreadDeps{
		App: app, Notifier: notifier, Svc: svc,
		RenderPage: r.Page, RenderPartial: r.Partial,
	})
	e.Router.GET("/match/{id}/thread", thread.Thread).BindFunc(requireAuthTest)
	e.Router.GET("/match/{id}/thread-messages", thread.ThreadMessages).BindFunc(requireAuthTest)
	e.Router.POST("/match/{id}/thread/message", thread.PostMessage).BindFunc(requireAuthTest)
	e.Router.POST("/match/{id}/thread/proposal", thread.PostProposal).BindFunc(requireAuthTest)
	e.Router.POST("/match/{id}/thread/proposal/{msgId}/respond", thread.RespondProposal).BindFunc(requireAuthTest)
	e.Router.POST("/match/{id}/thread/proposal/{msgId}/withdraw", thread.WithdrawProposal).BindFunc(requireAuthTest)
	e.Router.POST("/match/{id}/thread/proposal/{msgId}/reject-and-counter", thread.RejectAndCounterPropose).BindFunc(requireAuthTest)

	notif := NewNotificationHandler(app, r.Page, r.Partial)
	e.Router.GET("/notifications/count", notif.Count).BindFunc(requireAuthTest)
	e.Router.GET("/notifications/list", notif.List).BindFunc(requireAuthTest)

	comp := NewCompetitionHandler(app, svc, notifier, r.Page)
	dash := NewCompetitionDashboardHandler(app, r.Page)
	pairs := NewCompetitionPairsHandler(app)
	payments := NewCompetitionPaymentsHandler(app, notifier)
	fixture := NewFixtureHandler(app, svc, r.Page)
	g := e.Router.Group("/admin")
	g.BindFunc(requireAuthTest)
	g.BindFunc(requireAdminTest)
	g.GET("", dash.AdminEntry)
	g.GET("/competitions", dash.Dashboard)
	g.GET("/competitions/{id}", comp.Detail)
	g.POST("/competitions", comp.Create)
	g.POST("/competitions/{id}", comp.Update)
	g.POST("/competitions/{id}/logo", comp.LogoUpload)
	g.POST("/competitions/{id}/logo/delete", comp.LogoDelete)
	g.POST("/competitions/{id}/generate", fixture.GenerateFixtures)
	g.POST("/competitions/{id}/publish", comp.PublishCalendar)
	g.POST("/competitions/{id}/delete-calendar", comp.DeleteCalendar)
	g.POST("/competitions/{id}/toggle", comp.Toggle)
	g.POST("/competitions/{id}/finalize", comp.FinalizeCompetition)
	g.POST("/competitions/{id}/pairs", pairs.AddPair)
	g.POST("/competitions/{id}/copy-pairs", pairs.CopyPairs)
	g.POST("/competitions/{id}/remove-pair", pairs.RemovePair)
	g.POST("/competitions/{id}/payment", payments.TogglePayment)
	g.POST("/competitions/{id}/payment-all", payments.TogglePaymentAll)
	g.POST("/competitions/{id}/balls", payments.ToggleBalls)
	g.POST("/competitions/{id}/balls-all", payments.ToggleBallsAll)
	g.POST("/competitions/{id}/penalty", comp.ApplyPenalty)
	g.POST("/competitions/{id}/withdraw-pair", comp.WithdrawPair)
	g.POST("/competitions/{id}/broadcast", comp.AdminBroadcast)
}

func requireAuthTest(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.Redirect(302, "/login")
	}
	if e.Auth.GetString("display_name") == "" &&
		e.Request.URL.Path != "/profile/complete" {
		return e.Redirect(302, "/profile/complete")
	}
	return e.Next()
}

func requireAdminTest(e *core.RequestEvent) error {
	if e.Auth == nil || !slices.Contains(e.Auth.GetStringSlice("roles"), "admin") {
		return e.Redirect(302, "/login")
	}
	return e.Next()
}

func makePairTB(t testing.TB, app core.App, name string) *core.Record {
	t.Helper()
	n := pairSeq.Add(1)
	u1 := makeUserTB(t, app, name+" P1", fmt.Sprintf("pair%dp1@test.local", n))
	u2 := makeUserTB(t, app, name+" P2", fmt.Sprintf("pair%dp2@test.local", n))
	col, err := app.FindCollectionByNameOrId("pairs")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Set("name", name)
	record.Set("player1", u1.Id)
	record.Set("player2", u2.Id)
	record.Set("captain", u1.Id)
	require.NoError(t, app.Save(record))
	return record
}

func makePairWithGendersTB(t testing.TB, app core.App, name, g1, g2 string) *core.Record {
	t.Helper()
	n := pairSeq.Add(1)
	u1 := makeUserTB(t, app, name+" P1", fmt.Sprintf("pair%dp1@test.local", n))
	if g1 != "" {
		u1.Set("gender", g1)
		require.NoError(t, app.Save(u1))
	}
	u2 := makeUserTB(t, app, name+" P2", fmt.Sprintf("pair%dp2@test.local", n))
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

func makeCompetitionTB(t testing.TB, app core.App, compType string, pairs []*core.Record) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("competitions")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Set("name", "Test Competition")
	record.Set("type", compType)
	record.Set("active", true)
	record.Set("calendar_status", "published")
	now := time.Now().UTC()
	record.Set("start_date", now.Add(-7*24*time.Hour))
	record.Set("end_date", now.Add(7*24*time.Hour))
	pairIDs := make([]string, len(pairs))
	for i, p := range pairs {
		pairIDs[i] = p.Id
	}
	record.Set("pairs", pairIDs)
	require.NoError(t, app.Save(record))
	return record
}

func makeMatchTB(t testing.TB, app core.App, compID, p1ID, p2ID, status string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("matches")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Set("competition", compID)
	record.Set("pair1", p1ID)
	record.Set("pair2", p2ID)
	record.Set("status", status)
	record.Set("round_number", 1)
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

func makeAdminUserTB(t testing.TB, app core.App) *core.Record {
	t.Helper()
	n := userSeq.Add(1)
	col, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Set("email", fmt.Sprintf("admin%d@test.local", n))
	record.Set("username", fmt.Sprintf("hadmin%d", n))
	record.Set("display_name", "Admin")
	record.Set("roles", []string{"admin"})
	record.SetPassword("testpass123456")
	record.SetVerified(true)
	require.NoError(t, app.Save(record))
	return record
}

func makeVenueTB(t testing.TB, app core.App, name string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("venues")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Set("name", name)
	require.NoError(t, app.Save(record))
	return record
}

func makeInvitationTB(t testing.TB, app core.App, creatorID string, expiresAt time.Time) *core.Record {
	t.Helper()
	comp := makeCompetitionTB(t, app, "league", nil)
	col, err := app.FindCollectionByNameOrId("invitations")
	require.NoError(t, err)
	n := userSeq.Add(1)
	record := core.NewRecord(col)
	record.Set("token", fmt.Sprintf("tok%d", n))
	record.Set("created_by", creatorID)
	record.Set("competition", comp.Id)
	record.Set("status", "pending")
	if !expiresAt.IsZero() {
		record.Set("expires_at", expiresAt.UTC().Format("2006-01-02 15:04:05.000Z"))
	}
	require.NoError(t, app.Save(record))
	return record
}

func makeDocumentTB(t testing.TB, app core.App, title string, mandatory bool, url string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("documents")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Set("title", title)
	record.Set("is_mandatory", mandatory)
	record.Set("url", url)
	require.NoError(t, app.Save(record))
	return record
}

func makePenaltyTB(t testing.TB, app core.App, competitionID, pairID string, amount float64, reason, appliedBy string, voided bool) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("penalties")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Set("competition", competitionID)
	record.Set("pair", pairID)
	record.Set("amount", amount)
	record.Set("reason", reason)
	record.Set("applied_by", appliedBy)
	record.Set("voided", voided)
	require.NoError(t, app.Save(record))
}

func TestNewTestApp(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, err := app.FindCollectionByNameOrId("pairs")
	require.NoError(t, err, "pairs collection should exist")

	_, err = app.FindCollectionByNameOrId("competitions")
	require.NoError(t, err, "competitions collection should exist")

	_, err = app.FindCollectionByNameOrId("matches")
	require.NoError(t, err, "matches collection should exist")
}

func setupFullAdminRoutes(_ testing.TB, app *tests.TestApp, e *core.ServeEvent) {
	viewsFS := os.DirFS("..")
	r := render.New(viewsFS, "", true)
	notifier := notify.NewNotifier(app, "", "")
	svc := league.New(app, notifier)

	e.Router.BindFunc(middleware.CookieAuth)

	auth := NewAuthHandler(app, nil, r.Page)
	e.Router.GET("/login", auth.Login)

	comp := NewCompetitionHandler(app, svc, notifier, r.Page)
	dash := NewCompetitionDashboardHandler(app, r.Page)
	cpairs := NewCompetitionPairsHandler(app)
	cpayments := NewCompetitionPaymentsHandler(app, notifier)
	fixture := NewFixtureHandler(app, svc, r.Page)
	dispute := NewDisputeHandler(app, notifier, r.Page)
	inv := NewInvitationHandler(app, r.Page)
	pair := NewPairHandler(app, r.Page)
	player := NewAdminPlayerHandler(app, notifier, r.Page, r.Partial)
	venue := NewVenueHandler(app, r.Page, r.Partial)

	g := e.Router.Group("/admin")
	g.BindFunc(requireAuthTest)
	g.BindFunc(requireAdminTest)
	g.GET("", dash.AdminEntry)
	g.GET("/competitions", dash.Dashboard)
	g.GET("/competitions/{id}", comp.Detail)
	g.POST("/competitions", comp.Create)
	g.POST("/competitions/{id}", comp.Update)
	g.POST("/competitions/{id}/toggle", comp.Toggle)
	g.POST("/competitions/{id}/finalize", comp.FinalizeCompetition)
	g.POST("/competitions/{id}/pairs", cpairs.AddPair)
	g.POST("/competitions/{id}/copy-pairs", cpairs.CopyPairs)
	g.POST("/competitions/{id}/remove-pair", cpairs.RemovePair)
	g.POST("/competitions/{id}/payment", cpayments.TogglePayment)
	g.POST("/competitions/{id}/payment-all", cpayments.TogglePaymentAll)
	g.POST("/competitions/{id}/balls", cpayments.ToggleBalls)
	g.POST("/competitions/{id}/balls-all", cpayments.ToggleBallsAll)
	g.POST("/competitions/{id}/penalty", comp.ApplyPenalty)
	g.POST("/competitions/{id}/withdraw-pair", comp.WithdrawPair)
	g.POST("/competitions/{id}/generate", fixture.GenerateFixtures)
	g.POST("/competitions/{id}/publish", comp.PublishCalendar)
	g.POST("/competitions/{id}/delete-calendar", comp.DeleteCalendar)
	g.POST("/competitions/{id}/round-dates", comp.UpdateRoundDates)
	g.POST("/competitions/{id}/round-dates/regenerate", comp.RegenerateRoundDates)
	g.GET("/players", player.Players)
	g.GET("/players/{id}/edit", player.PlayerEditForm)
	g.POST("/players/pre-create", player.PlayerPreCreate)
	g.POST("/players/{id}", player.PlayerUpdate)
	g.GET("/pairs", pair.Pairs)
	g.POST("/pairs", pair.PairsCreate)
	g.POST("/pairs/{id}", pair.PairsUpdate)
	g.POST("/pairs/{id}/level", pair.SetLevel)
	g.GET("/outstanding", inv.Outstanding)
	g.GET("/disputes", dispute.Disputes)
	g.POST("/disputes/{id}/resolve", dispute.DisputesResolve)
	g.POST("/disputes/{id}/walkover-approve", dispute.WalkoverApprove)
	g.GET("/invitations", inv.InvitationsList)
	g.POST("/invitations", inv.InvitationsCreate)
	g.POST("/invitations/{id}/revoke", inv.InvitationsRevoke)
	g.GET("/venues", venue.Venues)
	g.GET("/venues/{id}/edit", venue.VenueEditForm)
	g.POST("/venues", venue.VenuesCreate)
	g.POST("/venues/{id}", venue.VenuesUpdate)
	g.POST("/venues/{id}/delete", venue.VenuesDelete)
}

func readBody(tb testing.TB, res *http.Response) string {
	tb.Helper()
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 1024)
	for {
		n, err := res.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return string(buf)
}

// expectRedirect chains an exact HX-Redirect assertion onto a scenario's
// AfterTestFunc. want runs after the request, so it can derive the target
// from s.URL or from records the handler created.
func expectRedirect(s *tests.ApiScenario, want func(app core.App) string) {
	prev := s.AfterTestFunc
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		assert.Equal(tb, want(app), res.Header.Get("HX-Redirect"), "HX-Redirect target")
		if prev != nil {
			prev(tb, app, res)
		}
	}
}

// redirectTo is the expectRedirect want for a fixed target.
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
