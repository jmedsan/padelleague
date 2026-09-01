package handlers

import (
	"fmt"
	"os"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"

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

// tmplDataDir holds a data directory with all migrations already applied.
// tests.NewTestApp() re-runs every migration on each call, which costs ~240ms
// a test; copying an already-migrated directory costs ~10ms. With ~150 tests
// in this package that is the difference between a 23s suite and a 3s one,
// and mutation testing multiplies it by the mutant count.
var tmplDataDir string

func TestMain(m *testing.M) {
	seed, err := tests.NewTestApp()
	if err != nil {
		fmt.Fprintln(os.Stderr, "build test template:", err)
		os.Exit(1)
	}
	dir, err := os.MkdirTemp("", "pbtmpl-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "temp dir:", err)
		os.Exit(1)
	}
	// CopyFS needs the destination absent; MkdirTemp already created it.
	if err := os.RemoveAll(dir); err != nil {
		fmt.Fprintln(os.Stderr, "clear temp dir:", err)
		os.Exit(1)
	}
	if err := os.CopyFS(dir, os.DirFS(seed.DataDir())); err != nil {
		fmt.Fprintln(os.Stderr, "copy template:", err)
		os.Exit(1)
	}
	seed.Cleanup()
	tmplDataDir = dir

	code := m.Run()
	if err := os.RemoveAll(dir); err != nil {
		fmt.Fprintln(os.Stderr, "remove template:", err)
	}
	os.Exit(code)
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

func makeUser(t *testing.T, app core.App, displayName, email string) *core.Record {
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
	app, err := tests.NewTestApp(tmplDataDir)
	require.NoError(t, err)
	return app
}

func newTestApp(t *testing.T) core.App {
	t.Helper()
	app, err := tests.NewTestApp(tmplDataDir)
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)
	return app
}

func makePair(t *testing.T, app core.App, name string) *core.Record {
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

func makeCompetition(t *testing.T, app core.App, pairs []*core.Record) *core.Record {
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

func makeMatch(t *testing.T, app core.App, compID, p1ID, p2ID, status string) *core.Record {
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

func makeFinalMatch(t *testing.T, app core.App, compID, p1ID, p2ID, score, winnerID string) {
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

func makeInvitation(t *testing.T, app core.App, expiresAt time.Time) *core.Record {
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

func makeNotification(t *testing.T, app core.App, userID, title, body string, read bool) *core.Record {
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
	r := render.New(viewsFS, "")
	auth := NewAuthHandler(app, r.Page)
	e.Router.GET("/login", auth.Login)
	e.Router.POST("/login", auth.LoginSubmit)
	e.Router.GET("/register", auth.Register)
	e.Router.POST("/register", auth.RegisterSubmit)
	e.Router.POST("/logout", auth.Logout)
}

func setupAllRoutes(_ testing.TB, app *tests.TestApp, e *core.ServeEvent) {
	viewsFS := os.DirFS("..")
	r := render.New(viewsFS, "")
	notifier := notify.NewNotifier(app, "", "")
	svc := league.New(app, notifier)

	e.Router.BindFunc(middleware.CookieAuth)

	auth := NewAuthHandler(app, r.Page)
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

	pub := NewPublicHandler(app, svc, r.Page, r.ErrorPage)
	e.Router.GET("/", pub.Home).BindFunc(requireAuthTest)
	e.Router.GET("/competition/{id}", pub.Competition).BindFunc(requireAuthTest)

	player := NewPlayerHandler(app, r.Page, r.ErrorPage)
	e.Router.GET("/player/{id}", player.Player).BindFunc(requireAuthTest)

	match := NewMatchHandler(app, notifier, r.Page, r.ErrorPage)
	e.Router.GET("/match/{id}", match.MatchDetail).BindFunc(requireAuthTest)
	e.Router.POST("/match/{id}/submit", match.MatchSubmit).BindFunc(requireAuthTest)
	e.Router.POST("/match/{id}/correct", match.MatchCorrect).BindFunc(requireAuthTest)
	e.Router.POST("/match/{id}/admin-override", match.AdminOverride).BindFunc(requireAuthTest)
	e.Router.POST("/match/{id}/report-unplayed", match.ReportUnplayed).BindFunc(requireAuthTest)

	thread := NewThreadHandler(app, notifier, r.Page, r.Partial)
	e.Router.GET("/match/{id}/thread", thread.Thread).BindFunc(requireAuthTest)
	e.Router.GET("/match/{id}/thread-messages", thread.ThreadMessages).BindFunc(requireAuthTest)
	e.Router.POST("/match/{id}/thread/message", thread.PostMessage).BindFunc(requireAuthTest)
	e.Router.POST("/match/{id}/thread/proposal", thread.PostProposal).BindFunc(requireAuthTest)
	e.Router.POST("/match/{id}/thread/proposal/{msgId}/respond", thread.RespondProposal).BindFunc(requireAuthTest)
	e.Router.POST("/match/{id}/thread/proposal/{msgId}/change-decision", thread.ProposalChangeDecision).BindFunc(requireAuthTest)

	notif := NewNotificationHandler(app, r.Page)
	e.Router.GET("/notifications/count", notif.Count).BindFunc(requireAuthTest)
	e.Router.GET("/notifications/list", notif.List).BindFunc(requireAuthTest)

	comp := NewCompetitionHandler(app, svc, notifier, r.Page)
	dash := NewCompetitionDashboardHandler(app, r.Page)
	pairs := NewCompetitionPairsHandler(app)
	payments := NewCompetitionPaymentsHandler(app)
	fixture := NewFixtureHandler(app, svc, r.Page)
	g := e.Router.Group("/admin")
	g.BindFunc(requireAuthTest)
	g.BindFunc(requireAdminTest)
	g.GET("", dash.AdminEntry)
	g.GET("/competitions", dash.Dashboard)
	g.GET("/competitions/{id}", comp.Detail)
	g.POST("/competitions", comp.Create)
	g.POST("/competitions/{id}", comp.Update)
	g.POST("/competitions/{id}/generate", fixture.GenerateFixtures)
	g.POST("/competitions/{id}/toggle", comp.Toggle)
	g.POST("/competitions/{id}/finalize", comp.FinalizeCompetition)
	g.POST("/competitions/{id}/pairs", pairs.AddPair)
	g.POST("/competitions/{id}/copy-pairs", pairs.CopyPairs)
	g.POST("/competitions/{id}/remove-pair", pairs.RemovePair)
	g.POST("/competitions/{id}/payment", payments.TogglePayment)
	g.POST("/competitions/{id}/payment-all", payments.TogglePaymentAll)
	g.POST("/competitions/{id}/penalty", comp.ApplyPenalty)
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
	r := render.New(viewsFS, "")
	notifier := notify.NewNotifier(app, "", "")
	svc := league.New(app, notifier)

	e.Router.BindFunc(middleware.CookieAuth)

	auth := NewAuthHandler(app, r.Page)
	e.Router.GET("/login", auth.Login)

	comp := NewCompetitionHandler(app, svc, notifier, r.Page)
	dash := NewCompetitionDashboardHandler(app, r.Page)
	cpairs := NewCompetitionPairsHandler(app)
	cpayments := NewCompetitionPaymentsHandler(app)
	fixture := NewFixtureHandler(app, svc, r.Page)
	dispute := NewDisputeHandler(app, notifier, r.Page)
	inv := NewInvitationHandler(app, r.Page)
	pair := NewPairHandler(app, r.Page)
	player := NewAdminPlayerHandler(app, r.Page)
	venue := NewVenueHandler(app, r.Page)

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
	g.POST("/competitions/{id}/penalty", comp.ApplyPenalty)
	g.POST("/competitions/{id}/generate", fixture.GenerateFixtures)
	g.POST("/competitions/{id}/round-dates", comp.UpdateRoundDates)
	g.POST("/competitions/{id}/round-dates/regenerate", comp.RegenerateRoundDates)
	g.GET("/players", player.Players)
	g.POST("/players/pre-create", player.PlayerPreCreate)
	g.POST("/players/{id}", player.PlayerUpdate)
	g.GET("/pairs", pair.Pairs)
	g.POST("/pairs", pair.PairsCreate)
	g.POST("/pairs/{id}", pair.PairsUpdate)
	g.GET("/outstanding", inv.Outstanding)
	g.GET("/disputes", dispute.Disputes)
	g.POST("/disputes/{id}/resolve", dispute.DisputesResolve)
	g.POST("/disputes/{id}/walkover-approve", dispute.WalkoverApprove)
	g.GET("/invitations", inv.InvitationsList)
	g.POST("/invitations", inv.InvitationsCreate)
	g.POST("/invitations/{id}/revoke", inv.InvitationsRevoke)
	g.GET("/venues", venue.Venues)
	g.POST("/venues", venue.VenuesCreate)
	g.POST("/venues/{id}", venue.VenuesUpdate)
	g.POST("/venues/{id}/delete", venue.VenuesDelete)
}
