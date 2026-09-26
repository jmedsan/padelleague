package hooks

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/league"
	"padelleague/notify"
	"padelleague/search"

	_ "padelleague/migrations"
)

var userSeq atomic.Int64

func newTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)
	return app
}

func registerHooks(t *testing.T, app *tests.TestApp) {
	t.Helper()
	svc := league.New(app, nil)
	Register(app, Deps{Svc: svc})
}

// registerHooksDeterministic registers hooks with a no-op shuffle so
// leveled-assignment count assertions are stable under parallel test load.
func registerHooksDeterministic(t *testing.T, app *tests.TestApp) {
	t.Helper()
	svc := league.New(app, nil)
	svc.SetShuffle(func(_ int, _ func(int, int)) {})
	Register(app, Deps{Svc: svc})
}

func registerHooksWithNotifier(t *testing.T, app *tests.TestApp) {
	t.Helper()
	svc := league.New(app, nil)
	notifier := notify.NewNotifier(app, "", "")
	Register(app, Deps{Svc: svc, Notifier: notifier})
}

func makeAdminUser(t *testing.T, app core.App) {
	t.Helper()
	n := userSeq.Add(1)
	col, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	r := core.NewRecord(col)
	r.Set("email", fmt.Sprintf("hookadmin%d@test.local", n))
	r.Set("username", fmt.Sprintf("hkadmin%d", n))
	r.Set("display_name", fmt.Sprintf("Hook Admin %d", n))
	r.SetPassword("testpass123456")
	r.SetVerified(true)
	r.Set("roles", []string{"admin"})
	require.NoError(t, app.Save(r))
}

func makeUser(t *testing.T, app core.App, role string) *core.Record {
	t.Helper()
	n := userSeq.Add(1)
	col, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	r := core.NewRecord(col)
	r.Set("email", fmt.Sprintf("hookuser%d@test.local", n))
	r.Set("username", fmt.Sprintf("hkuser%d", n))
	r.Set("display_name", fmt.Sprintf("Hook User %d", n))
	r.SetPassword("testpass123456")
	r.SetVerified(true)
	if role != "" {
		r.Set("roles", []string{role})
	}
	require.NoError(t, app.Save(r))
	return r
}

func makePair(t *testing.T, app core.App, name string) *core.Record {
	t.Helper()
	u1 := makeUser(t, app, "player")
	u2 := makeUser(t, app, "player")
	col, err := app.FindCollectionByNameOrId("pairs")
	require.NoError(t, err)
	r := core.NewRecord(col)
	r.Set("name", name)
	r.Set("player1", u1.Id)
	r.Set("player2", u2.Id)
	require.NoError(t, app.Save(r))
	return r
}

func makePlayoffComp(t *testing.T, app core.App, pairs []*core.Record) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("competitions")
	require.NoError(t, err)
	r := core.NewRecord(col)
	r.Set("name", "Hook Test Playoff")
	r.Set("type", "playoff")
	r.Set("status", "active")
	ids := make([]string, len(pairs))
	for i, p := range pairs {
		ids[i] = p.Id
	}
	r.Set("pairs", ids)
	require.NoError(t, app.Save(r))
	return r
}

func makeMatch(t *testing.T, app core.App, compID, p1ID, p2ID string, round int) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("matches")
	require.NoError(t, err)
	r := core.NewRecord(col)
	r.Set("competition", compID)
	if p1ID != "" {
		r.Set("pair1", p1ID)
	}
	if p2ID != "" {
		r.Set("pair2", p2ID)
	}
	r.Set("status", "pending")
	r.Set("round_number", round)
	require.NoError(t, app.Save(r))
	return r
}

// freshMatch re-reads a match from DB so Original() is populated for hook checks.
func freshMatch(t *testing.T, app core.App, id string) *core.Record {
	t.Helper()
	r, err := app.FindRecordById("matches", id)
	require.NoError(t, err)
	return r
}

// transitionMatch is a helper that re-reads, sets status (and optional fields), saves.
func transitionMatch(t *testing.T, app core.App, id, newStatus string, extra map[string]any) error {
	t.Helper()
	m := freshMatch(t, app, id)
	m.Set("status", newStatus)
	for k, v := range extra {
		m.Set(k, v)
	}
	return app.Save(m)
}

// Status transition tests: allowed

func TestTransition_PendingToConfirmed(t *testing.T) {
	app := newTestApp(t)
	registerHooks(t, app)
	p1 := makePair(t, app, "TrA")
	p2 := makePair(t, app, "TrB")
	comp := makePlayoffComp(t, app, []*core.Record{p1, p2})
	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)

	assert.NoError(t, transitionMatch(t, app, m.Id, league.StatusConfirmed, nil))
}

func TestTransition_PendingToFinal(t *testing.T) {
	app := newTestApp(t)
	registerHooks(t, app)
	p1 := makePair(t, app, "TrPFA")
	p2 := makePair(t, app, "TrPFB")
	comp := makePlayoffComp(t, app, []*core.Record{p1, p2})
	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)

	assert.NoError(t, transitionMatch(t, app, m.Id, league.StatusFinal,
		map[string]any{"scores": "6-3 6-4", "winner": p1.Id}))
}

func TestTransition_ConfirmedToFinal(t *testing.T) {
	app := newTestApp(t)
	registerHooks(t, app)
	p1 := makePair(t, app, "TrCFA")
	p2 := makePair(t, app, "TrCFB")
	comp := makePlayoffComp(t, app, []*core.Record{p1, p2})
	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)

	require.NoError(t, transitionMatch(t, app, m.Id, league.StatusConfirmed, nil))
	assert.NoError(t, transitionMatch(t, app, m.Id, league.StatusFinal,
		map[string]any{"scores": "6-3 6-4", "winner": p1.Id}))
}

func TestTransition_ConfirmedToDisputed(t *testing.T) {
	app := newTestApp(t)
	registerHooks(t, app)
	p1 := makePair(t, app, "TrCDA")
	p2 := makePair(t, app, "TrCDB")
	comp := makePlayoffComp(t, app, []*core.Record{p1, p2})
	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)

	require.NoError(t, transitionMatch(t, app, m.Id, league.StatusConfirmed, nil))
	assert.NoError(t, transitionMatch(t, app, m.Id, league.StatusDisputed, nil))
}

func TestTransition_DisputedToFinal(t *testing.T) {
	app := newTestApp(t)
	registerHooks(t, app)
	p1 := makePair(t, app, "TrDFA")
	p2 := makePair(t, app, "TrDFB")
	comp := makePlayoffComp(t, app, []*core.Record{p1, p2})
	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)

	require.NoError(t, transitionMatch(t, app, m.Id, league.StatusConfirmed, nil))
	require.NoError(t, transitionMatch(t, app, m.Id, league.StatusDisputed, nil))
	assert.NoError(t, transitionMatch(t, app, m.Id, league.StatusFinal,
		map[string]any{"scores": "6-3 6-4", "winner": p1.Id}))
}

// Status transition tests: rejected

func TestTransition_PendingToDisputed_Allowed(t *testing.T) {
	app := newTestApp(t)
	registerHooks(t, app)
	p1 := makePair(t, app, "TrRjA")
	p2 := makePair(t, app, "TrRjB")
	comp := makePlayoffComp(t, app, []*core.Record{p1, p2})
	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)

	// A pending match can move directly to disputed: this is the walkover-report
	// path (ReportUnplayed), reported before any score is ever submitted.
	require.NoError(t, transitionMatch(t, app, m.Id, league.StatusDisputed, nil))
}

func TestTransition_FinalToAnything_Rejected(t *testing.T) {
	app := newTestApp(t)
	registerHooks(t, app)
	p1 := makePair(t, app, "TrFnA")
	p2 := makePair(t, app, "TrFnB")
	comp := makePlayoffComp(t, app, []*core.Record{p1, p2})
	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)

	require.NoError(t, transitionMatch(t, app, m.Id, league.StatusFinal,
		map[string]any{"scores": "6-3 6-4", "winner": p1.Id}))

	err := transitionMatch(t, app, m.Id, league.StatusPending, nil)
	assert.ErrorContains(t, err, "invalid transition")
}

func TestTransition_DisputedToPending_Rejected(t *testing.T) {
	app := newTestApp(t)
	registerHooks(t, app)
	p1 := makePair(t, app, "TrDPA")
	p2 := makePair(t, app, "TrDPB")
	comp := makePlayoffComp(t, app, []*core.Record{p1, p2})
	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)

	require.NoError(t, transitionMatch(t, app, m.Id, league.StatusConfirmed, nil))
	require.NoError(t, transitionMatch(t, app, m.Id, league.StatusDisputed, nil))

	err := transitionMatch(t, app, m.Id, league.StatusPending, nil)
	assert.ErrorContains(t, err, "invalid transition")
}

func TestTransition_ConfirmedToPending_Rejected(t *testing.T) {
	app := newTestApp(t)
	registerHooks(t, app)
	p1 := makePair(t, app, "TrCPA")
	p2 := makePair(t, app, "TrCPB")
	comp := makePlayoffComp(t, app, []*core.Record{p1, p2})
	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)

	require.NoError(t, transitionMatch(t, app, m.Id, league.StatusConfirmed, nil))

	err := transitionMatch(t, app, m.Id, league.StatusPending, nil)
	assert.ErrorContains(t, err, "invalid transition")
}

func TestTransition_SameStatus_Allowed(t *testing.T) {
	app := newTestApp(t)
	registerHooks(t, app)
	p1 := makePair(t, app, "TrSSA")
	p2 := makePair(t, app, "TrSSB")
	comp := makePlayoffComp(t, app, []*core.Record{p1, p2})
	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, 2)

	fm := freshMatch(t, app, m.Id)
	fm.Set("scores", "6-3 6-4")
	assert.NoError(t, app.Save(fm), "updating without status change should succeed")
}

// transitionedToFinal gates handleAdvance's playoff-advance and leveled
// top-up so they only run on the save that actually finalizes a match, not
// on every later save of an already-final one.

func TestTransitionedToFinal_PendingToFinal(t *testing.T) {
	app := newTestApp(t)
	p1 := makePair(t, app, "TtfPFA")
	p2 := makePair(t, app, "TtfPFB")
	comp := makePlayoffComp(t, app, []*core.Record{p1, p2})
	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)

	fm := freshMatch(t, app, m.Id)
	fm.Set("status", league.StatusFinal)
	fm.Set("scores", "6-3 6-4")
	fm.Set("winner", p1.Id)
	require.NoError(t, app.Save(fm))

	assert.True(t, transitionedToFinal(fm), "a save that moves pending → final must transition")
}

func TestTransitionedToFinal_AlreadyFinalResave(t *testing.T) {
	app := newTestApp(t)
	p1 := makePair(t, app, "TtfARA")
	p2 := makePair(t, app, "TtfARB")
	comp := makePlayoffComp(t, app, []*core.Record{p1, p2})
	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)

	fm := freshMatch(t, app, m.Id)
	fm.Set("status", league.StatusFinal)
	fm.Set("scores", "6-3 6-4")
	fm.Set("winner", p1.Id)
	require.NoError(t, app.Save(fm))

	// Re-read and re-save the now-final match with an unrelated field
	// changed — status stays final on both sides of this save.
	resaved := freshMatch(t, app, m.Id)
	resaved.Set("scores", "6-3 6-2")
	require.NoError(t, app.Save(resaved))

	assert.False(t, transitionedToFinal(resaved), "a save that leaves an already-final match untouched must not re-transition")
}

func TestTransitionedToFinal_NeverFinal(t *testing.T) {
	app := newTestApp(t)
	p1 := makePair(t, app, "TtfNFA")
	p2 := makePair(t, app, "TtfNFB")
	comp := makePlayoffComp(t, app, []*core.Record{p1, p2})
	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)

	fm := freshMatch(t, app, m.Id)
	assert.False(t, transitionedToFinal(fm), "a pending match must not report as transitioned")
}

// Default role on user creation

func TestDefaultRole_EmptyRole_SetsPlayer(t *testing.T) {
	app := newTestApp(t)
	registerHooks(t, app)

	col, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	r := core.NewRecord(col)
	r.Set("email", "norole@test.local")
	r.Set("display_name", "No Role")
	r.SetPassword("testpass123456")
	r.SetVerified(true)
	require.NoError(t, app.Save(r))

	saved, err := app.FindRecordById("users", r.Id)
	require.NoError(t, err)
	assert.Contains(t, saved.GetStringSlice("roles"), "player")
}

func TestDefaultRole_ExplicitRole_Preserved(t *testing.T) {
	app := newTestApp(t)
	registerHooks(t, app)

	col, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	r := core.NewRecord(col)
	r.Set("email", "hasrole@test.local")
	r.Set("display_name", "Has Role")
	r.Set("roles", []string{"admin"})
	r.SetPassword("testpass123456")
	r.SetVerified(true)
	require.NoError(t, app.Save(r))

	saved, err := app.FindRecordById("users", r.Id)
	require.NoError(t, err)
	assert.Contains(t, saved.GetStringSlice("roles"), "admin")
}

// Cron registration

func TestCronRegistration_QuorumTimeout(t *testing.T) {
	app := newTestApp(t)
	registerHooks(t, app)

	jobs := app.Cron().Jobs()
	var found bool
	for _, j := range jobs {
		if j.Id() == "quorum-timeout" {
			found = true
			assert.Equal(t, "*/5 * * * *", j.Expression())
			break
		}
	}
	assert.True(t, found, "quorum-timeout cron job must be registered")
}

// Playoff advance notification tests (S-3)

func TestAdvancePlayoffSuccess_NoNotification(t *testing.T) {
	app := newTestApp(t)
	makeAdminUser(t, app)
	registerHooksWithNotifier(t, app)

	p1 := makePair(t, app, "HkOkA")
	p2 := makePair(t, app, "HkOkB")
	p3 := makePair(t, app, "HkOkC")
	p4 := makePair(t, app, "HkOkD")
	comp := makePlayoffComp(t, app, []*core.Record{p1, p2, p3, p4})

	m1 := makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)
	m2 := makeMatch(t, app, comp.Id, p3.Id, p4.Id, 1)
	finalMatch := makeMatch(t, app, comp.Id, "", "", 2)

	require.NoError(t, transitionMatch(t, app, m1.Id, league.StatusFinal,
		map[string]any{"scores": "6-3 6-4", "winner": p1.Id}))

	require.NoError(t, transitionMatch(t, app, m2.Id, league.StatusFinal,
		map[string]any{"scores": "6-1 6-2", "winner": p4.Id}))

	notifs, _ := app.FindRecordsByFilter("notifications",
		"type = 'admin_message' && title = 'Error en avance de playoff'",
		"", 0, 0, nil)
	assert.Empty(t, notifs, "no admin notification on successful advance")

	updated, err := app.FindRecordById("matches", finalMatch.Id)
	require.NoError(t, err)
	assert.Equal(t, p1.Id, updated.GetString("pair1"))
	assert.Equal(t, p4.Id, updated.GetString("pair2"))
}

// Scheduling reminder cron tests

func makeLeagueComp(t *testing.T, app core.App, pairs []*core.Record, start, end time.Time, rounds int) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("competitions")
	require.NoError(t, err)
	r := core.NewRecord(col)
	r.Set("name", "Sched Test League")
	r.Set("type", "league")
	r.Set("active", true)
	r.Set("calendar_status", "published")
	r.Set("rounds", rounds)
	r.Set("arrange_grace_days", 3)
	ids := make([]string, len(pairs))
	for i, p := range pairs {
		ids[i] = p.Id
	}
	r.Set("pairs", ids)
	sd, _ := types.ParseDateTime(start)
	r.Set("start_date", sd)
	ed, _ := types.ParseDateTime(end)
	r.Set("end_date", ed)
	r.Set("round_arrange_dates", league.StoreRoundSchedule(start, end, rounds))
	require.NoError(t, app.Save(r))
	return r
}

func TestSchedulingReminder_SendsAndEscalates(t *testing.T) {
	app := newTestApp(t)
	notifier := notify.NewNotifier(app, "", "")

	p1 := makePair(t, app, "ScA")
	p2 := makePair(t, app, "ScB")

	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	comp := makeLeagueComp(t, app, []*core.Record{p1, p2}, start, end, 1)

	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)

	// RecommendedArrangeBy for round 1/1 = end = Sep 1
	// With grace=3, overdue after Sep 4
	// WarnUrgent starts at end - 1 day = Aug 31
	// Simulate "now" as Sep 2 (between deadline and overdue) → WarnUrgent
	// But checkSchedulingReminders uses time.Now() — we can't easily fake it.
	// Instead, set dates so that NOW is past overdue.

	// Set dates so that now is past the overdue window.
	// recommendedBy = end (round 1/1), overdue = end + 3 days
	// If end is 20 days ago, now is well past overdue.
	pastEnd := time.Now().AddDate(0, 0, -20)
	pastStart := pastEnd.AddDate(0, -1, 0)
	compRec, err := app.FindRecordById("competitions", comp.Id)
	require.NoError(t, err)
	sd, _ := types.ParseDateTime(pastStart)
	compRec.Set("start_date", sd)
	ed, _ := types.ParseDateTime(pastEnd)
	compRec.Set("end_date", ed)
	compRec.Set("round_arrange_dates", league.StoreRoundSchedule(pastStart, pastEnd, 1))
	compRec.Set("recovery_days", 60) // stay out of the finished-by-date phase
	require.NoError(t, app.Save(compRec))

	// Run the cron function
	checkSchedulingReminders(app, notifier)

	// Check notification was sent — 4 players (2 pairs × 2 players each)
	notifs, err := app.FindRecordsByFilter("notifications",
		"type = 'scheduling'", "", 0, 0, nil)
	require.NoError(t, err)
	require.Len(t, notifs, 4, "should send one scheduling reminder per player (2 pairs × 2)")
	for _, n := range notifs {
		assert.Equal(t, "Recordatorio: organiza tu partido", n.GetString("title"))
		assert.Contains(t, n.GetString("body"), "El plazo ha vencido")
	}
	allPlayerIDs := append(
		league.PlayersForPair(app, p1.Id),
		league.PlayersForPair(app, p2.Id)...,
	)
	notifUserIDs := make([]string, len(notifs))
	for i, n := range notifs {
		notifUserIDs[i] = n.GetString("user")
	}
	assert.ElementsMatch(t, allPlayerIDs, notifUserIDs, "all 4 players must be notified")

	// Check last_warn_level was bumped
	updated := freshMatch(t, app, m.Id)
	assert.Equal(t, int(league.WarnOverdue), updated.GetInt("last_warn_level"))

	// Count notifications
	firstCount := len(notifs)

	// Run again — should NOT send another notification (escalation guard)
	checkSchedulingReminders(app, notifier)

	notifs2, err := app.FindRecordsByFilter("notifications",
		"type = 'scheduling'", "", 0, 0, nil)
	require.NoError(t, err)
	assert.Equal(t, firstCount, len(notifs2), "second run must not send duplicate reminders")
}

func TestSchedulingReminder_SkipsDraftCalendar(t *testing.T) {
	app := newTestApp(t)
	notifier := notify.NewNotifier(app, "", "")

	p1 := makePair(t, app, "DraftA")
	p2 := makePair(t, app, "DraftB")

	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	comp := makeLeagueComp(t, app, []*core.Record{p1, p2}, start, end, 1)
	comp.Set("calendar_status", "draft")
	require.NoError(t, app.Save(comp))

	pastEnd := time.Now().AddDate(0, 0, -20)
	pastStart := pastEnd.AddDate(0, -1, 0)
	sd, _ := types.ParseDateTime(pastStart)
	comp.Set("start_date", sd)
	ed, _ := types.ParseDateTime(pastEnd)
	comp.Set("end_date", ed)
	comp.Set("round_arrange_dates", league.StoreRoundSchedule(pastStart, pastEnd, 1))
	comp.Set("recovery_days", 60)
	require.NoError(t, app.Save(comp))

	makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)

	checkSchedulingReminders(app, notifier)

	notifs, err := app.FindRecordsByFilter("notifications", "type = 'scheduling'", "", 0, 0, nil)
	require.NoError(t, err)
	assert.Empty(t, notifs, "a draft calendar's matches are invisible to players, so no reminder should fire")
}

func TestSchedulingReminder_SkipsNoDateComp(t *testing.T) {
	app := newTestApp(t)
	notifier := notify.NewNotifier(app, "", "")

	p1 := makePair(t, app, "NdA")
	p2 := makePair(t, app, "NdB")

	col, err := app.FindCollectionByNameOrId("competitions")
	require.NoError(t, err)
	r := core.NewRecord(col)
	r.Set("name", "No Date League")
	r.Set("type", "league")
	r.Set("active", true)
	r.Set("rounds", 1)
	r.Set("pairs", []string{p1.Id, p2.Id})
	require.NoError(t, app.Save(r))

	makeMatch(t, app, r.Id, p1.Id, p2.Id, 1)

	checkSchedulingReminders(app, notifier)

	notifs, err := app.FindRecordsByFilter("notifications",
		"type = 'scheduling'", "", 0, 0, nil)
	require.NoError(t, err)
	assert.Empty(t, notifs, "no reminders for competition without dates")
}

func TestSchedulingReminder_DivergentStoredDate(t *testing.T) {
	app := newTestApp(t)
	notifier := notify.NewNotifier(app, "", "")

	p1 := makePair(t, app, "DivA")
	p2 := makePair(t, app, "DivB")

	// Start far in the past so interpolated deadline is also past.
	start := time.Now().AddDate(0, -3, 0)
	end := time.Now().AddDate(0, -1, 0)
	comp := makeLeagueComp(t, app, []*core.Record{p1, p2}, start, end, 2)
	makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)

	// Overwrite round 1's date to far in the future — if the cron reads the
	// stored value, no reminder fires; if it recomputes, the past interpolated
	// date triggers a reminder.
	futureDate := time.Now().AddDate(1, 0, 0)
	schedule := fmt.Sprintf(`{"1":"%s","2":"%s"}`,
		futureDate.Format(time.RFC3339),
		end.Format(time.RFC3339))
	compRec, err := app.FindRecordById("competitions", comp.Id)
	require.NoError(t, err)
	compRec.Set("round_arrange_dates", schedule)
	require.NoError(t, app.Save(compRec))

	checkSchedulingReminders(app, notifier)

	notifs, err := app.FindRecordsByFilter("notifications",
		"type = 'scheduling'", "", 0, 0, nil)
	require.NoError(t, err)
	assert.Empty(t, notifs, "stored future date must suppress reminder — proves stored read is used")
}

func TestSchedulingReminder_DenominatorDrift(t *testing.T) {
	app := newTestApp(t)
	notifier := notify.NewNotifier(app, "", "")

	p1 := makePair(t, app, "DriftA")
	p2 := makePair(t, app, "DriftB")
	p3 := makePair(t, app, "DriftC")

	// 3 rounds, start well in the past so round 1 deadline (1/3 of window) is past.
	start := time.Now().AddDate(0, -6, 0)
	end := time.Now().AddDate(0, -2, 0)
	comp := makeLeagueComp(t, app, []*core.Record{p1, p2, p3}, start, end, 3)
	compRec, err := app.FindRecordById("competitions", comp.Id)
	require.NoError(t, err)
	compRec.Set("recovery_days", 90) // stay out of the finished-by-date phase
	require.NoError(t, app.Save(compRec))

	// Round 1 pending, rounds 2+3 played (final).
	makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)
	m2 := makeMatch(t, app, comp.Id, p1.Id, p3.Id, 2)
	m2.Set("status", "final")
	require.NoError(t, app.Save(m2))
	m3 := makeMatch(t, app, comp.Id, p2.Id, p3.Id, 3)
	m3.Set("status", "final")
	require.NoError(t, app.Save(m3))

	checkSchedulingReminders(app, notifier)

	// Round 1's stored deadline uses rounds=3 (full denominator), not
	// countRounds(pending)=1. With the full denominator the deadline is at
	// start + 1/3*(end-start), which is well past → reminder fires.
	// 3 pairs → match involves p1+p2 → 4 players notified.
	notifs, err := app.FindRecordsByFilter("notifications",
		"type = 'scheduling'", "", 0, 0, nil)
	require.NoError(t, err)
	require.Len(t, notifs, 4, "round 1 deadline must use stored rounds=3 — all 4 players notified")
	for _, n := range notifs {
		assert.Equal(t, "Recordatorio: organiza tu partido", n.GetString("title"))
		assert.Contains(t, n.GetString("body"), "El plazo ha vencido")
	}

	updated := freshMatch(t, app, m2.Id)
	assert.Equal(t, 0, updated.GetInt("last_warn_level"), "final match must not be reminded")
}

func TestSchedulingReminder_RecoveryPhase_StillReminds(t *testing.T) {
	app := newTestApp(t)
	notifier := notify.NewNotifier(app, "", "")

	p1 := makePair(t, app, "RecA")
	p2 := makePair(t, app, "RecB")

	start := time.Now().AddDate(0, 0, -40)
	end := time.Now().AddDate(0, 0, -20)
	comp := makeLeagueComp(t, app, []*core.Record{p1, p2}, start, end, 1)
	compRec, err := app.FindRecordById("competitions", comp.Id)
	require.NoError(t, err)
	compRec.Set("recovery_days", 30) // now (end+20) is still inside end+30 -> recovery
	require.NoError(t, app.Save(compRec))

	makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)

	checkSchedulingReminders(app, notifier)

	notifs, err := app.FindRecordsByFilter("notifications",
		"type = 'scheduling'", "", 0, 0, nil)
	require.NoError(t, err)
	require.Len(t, notifs, 4, "recovery-phase must still remind — 4 players (2 pairs × 2)")
	for _, n := range notifs {
		assert.Equal(t, "Recordatorio: organiza tu partido", n.GetString("title"))
		assert.Contains(t, n.GetString("body"), "El plazo ha vencido")
	}
}

func TestSchedulingReminder_FinishedByDate_NoReminder(t *testing.T) {
	app := newTestApp(t)
	notifier := notify.NewNotifier(app, "", "")

	p1 := makePair(t, app, "FinA")
	p2 := makePair(t, app, "FinB")

	start := time.Now().AddDate(0, 0, -60)
	end := time.Now().AddDate(0, 0, -30)
	comp := makeLeagueComp(t, app, []*core.Record{p1, p2}, start, end, 1)
	compRec, err := app.FindRecordById("competitions", comp.Id)
	require.NoError(t, err)
	compRec.Set("recovery_days", 14) // now (end+30) is past end+14 -> finished
	require.NoError(t, app.Save(compRec))

	makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)

	checkSchedulingReminders(app, notifier)

	notifs, err := app.FindRecordsByFilter("notifications",
		"type = 'scheduling'", "", 0, 0, nil)
	require.NoError(t, err)
	assert.Empty(t, notifs, "a competition finished by date must not remind")
}

// setupMatchReminder creates a scheduled match 'untilStart' from 'now' with
// the given time string, and returns (app, notifier, match, now).
func setupMatchReminder(t *testing.T, untilStart time.Duration, matchTime string) (*tests.TestApp, *notify.Notifier, *core.Record, time.Time) {
	t.Helper()
	app := newTestApp(t)
	notifier := notify.NewNotifier(app, "", "")

	p1 := makePair(t, app, t.Name()+"A")
	p2 := makePair(t, app, t.Name()+"B")
	comp := makeLeagueComp(t, app, []*core.Record{p1, p2},
		time.Now().AddDate(0, 0, -30), time.Now().AddDate(0, 0, 30), 1)

	start := time.Date(2026, 6, 15, 18, 0, 0, 0, league.Madrid)
	now := start.Add(-untilStart)

	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)
	m.Set("status", league.StatusScheduled)
	d, _ := types.ParseDateTime(start)
	m.Set("date", d)
	m.Set("time", matchTime)
	m.Set("club", "Padel 360")
	require.NoError(t, app.Save(m))

	return app, notifier, m, now
}

func matchReminderNotifs(t *testing.T, app core.App) []*core.Record {
	t.Helper()
	notifs, err := app.FindRecordsByFilter("notifications", "type = 'match_reminder'", "", 0, 0, nil)
	require.NoError(t, err)
	return notifs
}

func TestMatchReminders_Fires26h(t *testing.T) {
	app, notifier, _, now := setupMatchReminder(t, 26*time.Hour, "18:00")
	checkMatchReminders(app, notifier, now)

	notifs := matchReminderNotifs(t, app)
	require.Len(t, notifs, 4, "4 players (2 pairs × 2) get the 26h reminder")
	for _, n := range notifs {
		assert.Equal(t, "Próximo partido", n.GetString("title"))
		assert.Contains(t, n.GetString("body"), "18:00")
		assert.Contains(t, n.GetString("body"), "Padel 360")
		assert.Contains(t, n.GetString("body"), "vs")
	}
}

func TestMatchReminders_Fires1h(t *testing.T) {
	app, notifier, _, now := setupMatchReminder(t, 1*time.Hour, "18:00")
	checkMatchReminders(app, notifier, now)

	notifs := matchReminderNotifs(t, app)
	require.Len(t, notifs, 4)
	for _, n := range notifs {
		assert.Equal(t, "Tu partido empieza pronto", n.GetString("title"))
		assert.Contains(t, n.GetString("body"), "en 1 hora")
	}
}

func TestMatchReminders_BothDue(t *testing.T) {
	app, notifier, _, now := setupMatchReminder(t, 50*time.Minute, "18:00")
	checkMatchReminders(app, notifier, now)

	notifs := matchReminderNotifs(t, app)
	require.Len(t, notifs, 4, "both 26h and 1h tiers are due, but only the smallest triggers a notification")

	reminders, err := app.FindRecordsByFilter("match_reminders", "1=1", "", 0, 0, nil)
	require.NoError(t, err)
	assert.Equal(t, 8, len(reminders), "2 tiers × 4 players = 8 claim rows")
}

func TestMatchReminders_SecondRunNoResend(t *testing.T) {
	app, notifier, _, now := setupMatchReminder(t, 26*time.Hour, "18:00")
	checkMatchReminders(app, notifier, now)
	require.Len(t, matchReminderNotifs(t, app), 4)

	checkMatchReminders(app, notifier, now)
	assert.Len(t, matchReminderNotifs(t, app), 4, "second run must not duplicate notifications")
}

func TestMatchReminders_DraftCalendarSkipped(t *testing.T) {
	app, notifier, m, now := setupMatchReminder(t, 26*time.Hour, "18:00")

	comp, err := app.FindRecordById("competitions", m.GetString("competition"))
	require.NoError(t, err)
	comp.Set("calendar_status", "draft")
	require.NoError(t, app.Save(comp))

	checkMatchReminders(app, notifier, now)
	assert.Empty(t, matchReminderNotifs(t, app))
}

func TestMatchReminders_UnparsableTimeSkipped(t *testing.T) {
	app, notifier, m, now := setupMatchReminder(t, 26*time.Hour, "")
	m.Set("time", "")
	require.NoError(t, app.Save(m))

	checkMatchReminders(app, notifier, now)
	assert.Empty(t, matchReminderNotifs(t, app))
}

func TestMatchReminders_UserCustomPrefs(t *testing.T) {
	app := newTestApp(t)
	notifier := notify.NewNotifier(app, "", "")

	p1 := makePair(t, app, "CustA")
	p2 := makePair(t, app, "CustB")
	comp := makeLeagueComp(t, app, []*core.Record{p1, p2},
		time.Now().AddDate(0, 0, -30), time.Now().AddDate(0, 0, 30), 1)

	// Set custom reminder hours on pair1's player1: only 2h
	pair1, err := app.FindRecordById("pairs", p1.Id)
	require.NoError(t, err)
	u1, err := app.FindRecordById("users", pair1.GetString("player1"))
	require.NoError(t, err)
	u1.Set("notification_prefs", `{"match_reminder_hours":[2]}`)
	require.NoError(t, app.Save(u1))

	start := time.Date(2026, 6, 15, 18, 0, 0, 0, league.Madrid)
	now := start.Add(-26 * time.Hour)

	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)
	m.Set("status", league.StatusScheduled)
	d, _ := types.ParseDateTime(start)
	m.Set("date", d)
	m.Set("time", "18:00")
	m.Set("club", "Padel 360")
	require.NoError(t, app.Save(m))

	checkMatchReminders(app, notifier, now)

	notifs := matchReminderNotifs(t, app)
	// u1 has custom [2], so 26h is not due for them. 3 other players get default [26,1] → 26 fires.
	assert.Len(t, notifs, 3, "user with custom 2h pref should not fire at 26h")
}

func TestMatchReminders_UserEmptyPrefs(t *testing.T) {
	app := newTestApp(t)
	notifier := notify.NewNotifier(app, "", "")

	p1 := makePair(t, app, "EmptyA")
	p2 := makePair(t, app, "EmptyB")
	comp := makeLeagueComp(t, app, []*core.Record{p1, p2},
		time.Now().AddDate(0, 0, -30), time.Now().AddDate(0, 0, 30), 1)

	pair1, err := app.FindRecordById("pairs", p1.Id)
	require.NoError(t, err)
	u1, err := app.FindRecordById("users", pair1.GetString("player1"))
	require.NoError(t, err)
	u1.Set("match_reminder_hours", "")
	require.NoError(t, app.Save(u1))

	start := time.Date(2026, 6, 15, 18, 0, 0, 0, league.Madrid)
	now := start.Add(-26 * time.Hour)

	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)
	m.Set("status", league.StatusScheduled)
	d, _ := types.ParseDateTime(start)
	m.Set("date", d)
	m.Set("time", "18:00")
	m.Set("club", "Padel 360")
	require.NoError(t, app.Save(m))

	checkMatchReminders(app, notifier, now)

	notifs := matchReminderNotifs(t, app)
	assert.Len(t, notifs, 4, "empty user pref falls through to global defaults")
}

func TestMatchReminders_CompOverride(t *testing.T) {
	app := newTestApp(t)
	notifier := notify.NewNotifier(app, "", "")

	p1 := makePair(t, app, "CompOvrA")
	p2 := makePair(t, app, "CompOvrB")
	comp := makeLeagueComp(t, app, []*core.Record{p1, p2},
		time.Now().AddDate(0, 0, -30), time.Now().AddDate(0, 0, 30), 1)
	comp.Set("match_reminder_hours", []int{2})
	require.NoError(t, app.Save(comp))

	start := time.Date(2026, 6, 15, 18, 0, 0, 0, league.Madrid)
	now := start.Add(-26 * time.Hour)

	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)
	m.Set("status", league.StatusScheduled)
	d, _ := types.ParseDateTime(start)
	m.Set("date", d)
	m.Set("time", "18:00")
	m.Set("club", "Padel 360")
	require.NoError(t, app.Save(m))

	checkMatchReminders(app, notifier, now)

	notifs := matchReminderNotifs(t, app)
	assert.Empty(t, notifs, "comp override [2] means nothing fires at 26h — 2h tier not yet due")
}

func TestMatchReminders_ClearThenResend(t *testing.T) {
	app, notifier, m, now := setupMatchReminder(t, 26*time.Hour, "18:00")
	checkMatchReminders(app, notifier, now)
	require.Len(t, matchReminderNotifs(t, app), 4)

	league.ClearMatchReminders(app, m.Id)

	checkMatchReminders(app, notifier, now)
	assert.Len(t, matchReminderNotifs(t, app), 8, "after clearing, reminders fire again")
}

func TestMatchReminders_MadridNearMidnight(t *testing.T) {
	app := newTestApp(t)
	notifier := notify.NewNotifier(app, "", "")

	p1 := makePair(t, app, "MdnA")
	p2 := makePair(t, app, "MdnB")
	comp := makeLeagueComp(t, app, []*core.Record{p1, p2},
		time.Now().AddDate(0, 0, -30), time.Now().AddDate(0, 0, 30), 1)

	// Match at 20:00 Madrid on June 16. "now" is 23:00 UTC June 15 = 01:00 Madrid June 16.
	// Until start: ~19h. The default 26h tier should NOT fire, but 1h won't either.
	// With untilStart=19h, only hours ≥19 are due from [26,1] → 26 fires.
	start := time.Date(2026, 6, 16, 20, 0, 0, 0, league.Madrid)
	now := time.Date(2026, 6, 15, 23, 0, 0, 0, time.UTC) // 01:00 Madrid June 16

	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)
	m.Set("status", league.StatusScheduled)
	d, _ := types.ParseDateTime(start)
	m.Set("date", d)
	m.Set("time", "20:00")
	m.Set("club", "Padel 360")
	require.NoError(t, app.Save(m))

	checkMatchReminders(app, notifier, now)

	notifs := matchReminderNotifs(t, app)
	require.Len(t, notifs, 4, "26h tier fires when untilStart ≈ 19h (26h ≥ 19h)")
	for _, n := range notifs {
		assert.Equal(t, "Próximo partido", n.GetString("title"))
	}
}

func TestCronRegistration_SchedulingReminders(t *testing.T) {
	app := newTestApp(t)
	registerHooksWithNotifier(t, app)

	jobs := app.Cron().Jobs()
	var found bool
	for _, j := range jobs {
		if j.Id() == "scheduling-reminders" {
			found = true
			assert.Equal(t, "0 9 * * *", j.Expression())
			break
		}
	}
	assert.True(t, found, "scheduling-reminders cron job must be registered")
}

func TestCronRegistration_MatchReminders(t *testing.T) {
	app := newTestApp(t)
	registerHooksWithNotifier(t, app)

	jobs := app.Cron().Jobs()
	var found bool
	for _, j := range jobs {
		if j.Id() == "match-reminders" {
			found = true
			assert.Equal(t, "*/5 * * * *", j.Expression())
			break
		}
	}
	assert.True(t, found, "match-reminders cron job must be registered")
}

// Search index real-time upsert tests (W10): a record created/updated on a
// hooked collection must be searchable immediately, without waiting for the
// periodic Rebuild cron.

func registerHooksWithSearch(t *testing.T, app *tests.TestApp) *search.Index {
	t.Helper()
	svc := league.New(app, nil)
	ix := &search.Index{}
	Register(app, Deps{Svc: svc, SearchIndex: ix})
	return ix
}

func makeVenue(t *testing.T, app core.App, name string) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("venues")
	require.NoError(t, err)
	r := core.NewRecord(col)
	r.Set("name", name)
	require.NoError(t, app.Save(r))
}

func TestSearchUpsert_UserCreateIsImmediatelySearchable(t *testing.T) {
	app := newTestApp(t)
	ix := registerHooksWithSearch(t, app)

	u := makeUser(t, app, "player")
	u.Set("display_name", "Hookindex Player")
	require.NoError(t, app.Save(u))

	admin := search.Viewer{IsAdmin: true}
	results := ix.Search("hookindex", admin, 10)
	require.NotEmpty(t, results, "newly created user must be searchable without a rebuild")
	assert.Equal(t, "Hookindex Player", results[0].Label)
}

func TestSearchUpsert_CompetitionUpdateRefreshesEntry(t *testing.T) {
	app := newTestApp(t)
	ix := registerHooksWithSearch(t, app)

	p1 := makePair(t, app, "SuA")
	p2 := makePair(t, app, "SuB")
	comp := makePlayoffComp(t, app, []*core.Record{p1, p2})
	comp.Set("name", "Renamed Competition Xyz")
	require.NoError(t, app.Save(comp))

	admin := search.Viewer{IsAdmin: true}
	results := ix.Search("renamed competition xyz", admin, 10)
	require.NotEmpty(t, results, "updated competition name must be searchable without a rebuild")
	assert.Equal(t, "Renamed Competition Xyz", results[0].Label)
}

func TestSearchUpsert_MatchCreateIsImmediatelySearchable(t *testing.T) {
	app := newTestApp(t)
	ix := registerHooksWithSearch(t, app)

	p1 := makePair(t, app, "SuMatchA")
	p2 := makePair(t, app, "SuMatchB")
	comp := makePlayoffComp(t, app, []*core.Record{p1, p2})
	makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)

	admin := search.Viewer{IsAdmin: true}
	results := ix.Search("sumatcha", admin, 10)
	require.NotEmpty(t, results, "newly created match must be searchable without a rebuild")
}

func TestSearchUpsert_VenueCreateDoesNotEvictOtherVenues(t *testing.T) {
	app := newTestApp(t)
	ix := registerHooksWithSearch(t, app)

	makeVenue(t, app, "First Venue Xyz")
	makeVenue(t, app, "Second Venue Xyz")

	admin := search.Viewer{IsAdmin: true}
	results := ix.Search("venue xyz", admin, 10)
	labels := make(map[string]bool, len(results))
	for _, r := range results {
		labels[r.Label] = true
	}
	assert.True(t, labels["First Venue Xyz"], "creating a second venue must not evict the first venue's entry (shared URL)")
	assert.True(t, labels["Second Venue Xyz"])
}

func TestSearchUpsert_PairUpdateRefreshesEntry(t *testing.T) {
	app := newTestApp(t)
	ix := registerHooksWithSearch(t, app)

	p := makePair(t, app, "Quokka Ferrari")
	p.Set("name", "Zebra Mongoose")
	require.NoError(t, app.Save(p))

	admin := search.Viewer{IsAdmin: true}
	results := ix.Search("zebra mongoose", admin, 10)
	require.NotEmpty(t, results, "updated pair name must be searchable without a rebuild")
	assert.Equal(t, "Zebra Mongoose", results[0].Label)

	stale := ix.Search("quokka ferrari", admin, 10)
	assert.Empty(t, stale, "the pair's old name must no longer match")
}

// -- Task 7 helpers ---------------------------------------------------------

func makeLeveledComp(t *testing.T, app core.App, pairs []*core.Record, target, open int) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("competitions")
	require.NoError(t, err)
	r := core.NewRecord(col)
	r.Set("name", "Hook Leveled")
	r.Set("type", "league")
	r.Set("active", true)
	r.Set("calendar_status", "published")
	r.Set("target_matches", target)
	r.Set("open_assignments", open)
	ids := make([]string, len(pairs))
	for i, p := range pairs {
		ids[i] = p.Id
	}
	r.Set("pairs", ids)
	require.NoError(t, app.Save(r))
	return r
}

func pendingMatchesFor(t *testing.T, app core.App, pairID, compID string) []*core.Record {
	t.Helper()
	recs, err := app.FindRecordsByFilter("matches",
		"competition = {:c} && (pair1 = {:p} || pair2 = {:p}) && status = 'pending'",
		"", 0, 0, map[string]any{"c": compID, "p": pairID})
	require.NoError(t, err)
	return recs
}

// -- TestFinalTransition_StampsFinalizedAt ----------------------------------

func TestFinalTransition_StampsFinalizedAt(t *testing.T) {
	app := newTestApp(t)
	registerHooks(t, app)
	p1 := makePair(t, app, "FzA")
	p2 := makePair(t, app, "FzB")
	comp := makePlayoffComp(t, app, []*core.Record{p1, p2})
	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)

	require.NoError(t, transitionMatch(t, app, m.Id, league.StatusFinal,
		map[string]any{"scores": "6-3 6-4", "winner": p1.Id}))

	updated, err := app.FindRecordById("matches", m.Id)
	require.NoError(t, err)
	assert.False(t, updated.GetDateTime("finalized_at").IsZero(),
		"finalized_at must be set on transition to final")
}

// -- TestCreateFinal_StampsFinalizedAt --------------------------------------

func TestCreateFinal_StampsFinalizedAt(t *testing.T) {
	app := newTestApp(t)
	registerHooks(t, app)
	p1 := makePair(t, app, "CrFzA")
	p2 := makePair(t, app, "CrFzB")
	comp := makePlayoffComp(t, app, []*core.Record{p1, p2})

	col, err := app.FindCollectionByNameOrId("matches")
	require.NoError(t, err)
	r := core.NewRecord(col)
	r.Set("competition", comp.Id)
	r.Set("pair1", p1.Id)
	r.Set("pair2", p2.Id)
	r.Set("status", "final")
	r.Set("scores", "6-3 6-4")
	r.Set("winner", p1.Id)
	require.NoError(t, app.Save(r))

	saved, err := app.FindRecordById("matches", r.Id)
	require.NoError(t, err)
	assert.False(t, saved.GetDateTime("finalized_at").IsZero(),
		"finalized_at must be set when a match is created with status=final")
}

// -- TestFinalTransition_AssignsNextOpponent --------------------------------

func TestFinalTransition_AssignsNextOpponent(t *testing.T) {
	app := newTestApp(t)
	registerHooks(t, app)

	pairs := make([]*core.Record, 6)
	for i := range pairs {
		pairs[i] = makePair(t, app, "LvlAdv")
	}
	comp := makeLeveledComp(t, app, pairs, 4, 2)

	// Seed initial assignments manually so both pairs have 1 pending each.
	m := makeMatch(t, app, comp.Id, pairs[0].Id, pairs[1].Id, 0)

	countBefore := len(pendingMatchesFor(t, app, pairs[0].Id, comp.Id)) +
		len(pendingMatchesFor(t, app, pairs[1].Id, comp.Id))

	require.NoError(t, transitionMatch(t, app, m.Id, league.StatusFinal,
		map[string]any{"scores": "6-3 6-4", "winner": pairs[0].Id}))

	countAfter := len(pendingMatchesFor(t, app, pairs[0].Id, comp.Id)) +
		len(pendingMatchesFor(t, app, pairs[1].Id, comp.Id))

	assert.Greater(t, countAfter, countBefore,
		"finalizing a leveled match must trigger new assignments for the freed slots")
}

// -- TestFinalTransition_RoundRobinUnchanged --------------------------------

func TestFinalTransition_RoundRobinUnchanged(t *testing.T) {
	app := newTestApp(t)
	registerHooks(t, app)

	p1 := makePair(t, app, "RrNoA")
	p2 := makePair(t, app, "RrNoB")
	comp := makeLeagueComp(t, app, []*core.Record{p1, p2},
		time.Now().AddDate(0, -1, 0), time.Now().AddDate(0, 1, 0), 1)
	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)

	require.NoError(t, transitionMatch(t, app, m.Id, league.StatusFinal,
		map[string]any{"scores": "6-3 6-4", "winner": p1.Id}))

	// Round-robin: no new matches should be created.
	allMatches, err := app.FindRecordsByFilter("matches",
		"competition = {:c}", "", 0, 0, map[string]any{"c": comp.Id})
	require.NoError(t, err)
	assert.Len(t, allMatches, 1, "round-robin must not generate extra matches on finalization")
}

// -- TestRelease_AssignsReplacement -----------------------------------------

func TestRelease_AssignsReplacement(t *testing.T) {
	app := newTestApp(t)
	registerHooksDeterministic(t, app)

	// 6 pairs, target=3, open=2 — each pair has open=2 pending at a time but
	// capacity for 3 total matches. With 6 pairs there are always unmet
	// opponents available after deleting one pending match.
	pairs := make([]*core.Record, 6)
	for i := range pairs {
		pairs[i] = makePair(t, app, "Rel")
	}
	comp := makeLeveledComp(t, app, pairs, 3, 2)

	pa, pb := pairs[0], pairs[1]

	// Create one pending match between pa and pb.
	mAB := makeMatch(t, app, comp.Id, pa.Id, pb.Id, 0)
	// Give pa and pb one other pending match each (saturating open=2).
	makeMatch(t, app, comp.Id, pa.Id, pairs[2].Id, 0)
	makeMatch(t, app, comp.Id, pb.Id, pairs[3].Id, 0)

	// Delete the pa-pb match; delete hook should top up both (avoid pa-pb).
	require.NoError(t, app.Delete(mAB))

	// eligible() (leveled.go) allows an opponent at pending == open — only
	// a requester needs a free slot, an opponent may be pushed to open+1
	// (recipe §3.2) — so the real invariant is pending in [0, open+1] for
	// every pair, not an exact count for any one pair.
	for _, p := range pairs {
		ms := pendingMatchesFor(t, app, p.Id, comp.Id)
		assert.LessOrEqual(t, len(ms), 3, "pair %s: %d pending exceeds open+1=3", p.Id, len(ms))
		for _, m := range ms {
			p1, p2 := m.GetString("pair1"), m.GetString("pair2")
			isPairedWithFormer := (p1 == pa.Id && p2 == pb.Id) || (p1 == pb.Id && p2 == pa.Id)
			assert.False(t, isPairedWithFormer, "replacement must not recreate the deleted pairing")
		}
	}

	aMatches := pendingMatchesFor(t, app, pa.Id, comp.Id)
	bMatches := pendingMatchesFor(t, app, pb.Id, comp.Id)
	assert.GreaterOrEqual(t, len(aMatches), 2, "pa should have been given a replacement, back up to at least open=2")
	assert.GreaterOrEqual(t, len(bMatches), 2, "pb should have been given a replacement, back up to at least open=2")

	// Every pair still under target must have been offered a chance to
	// reach open — nobody is left stuck below it while a valid opponent
	// exists (the whole point of the delete hook's top-up).
	for _, p := range pairs {
		ms := pendingMatchesFor(t, app, p.Id, comp.Id)
		assert.GreaterOrEqual(t, len(ms), 2, "pair %s: %d pending, expected to reach open=2 after top-up", p.Id, len(ms))
	}

	all, err := app.FindRecordsByFilter("matches", "competition = {:c}", "", 0, 0, map[string]any{"c": comp.Id})
	require.NoError(t, err)
	assert.Len(t, all, 7, "expected exactly 7 matches: 2 surviving originals + 5 created by the delete-hook top-up")
}

// -- TestRegeneratePublished_NoTopUp ----------------------------------------

func TestRegeneratePublished_NoTopUp(t *testing.T) {
	app := newTestApp(t)
	// Do NOT register hooks — we call GenerateInitialAssignments manually and
	// verify that a subsequent registerHooks + delete does not double-create.
	svc := league.New(app, nil)

	pairs := make([]*core.Record, 6)
	for i := range pairs {
		pairs[i] = makePair(t, app, "Regen")
	}
	comp := makeLeveledComp(t, app, pairs, 4, 2)

	// Simulate what the fixture handler does on re-generate:
	// delete existing matches then create fresh ones.
	initial, err := svc.GenerateInitialAssignments(app, comp, time.Now())
	require.NoError(t, err)
	require.Greater(t, initial, 0)

	// Count matches created by GenerateInitialAssignments.
	allMatches, err := app.FindRecordsByFilter("matches",
		"competition = {:c}", "", 0, 0, map[string]any{"c": comp.Id})
	require.NoError(t, err)
	countAfterGenerate := len(allMatches)
	assert.Equal(t, initial, countAfterGenerate)
}

// -- TestLeveledCron_Registered ---------------------------------------------

func TestLeveledCron_Registered(t *testing.T) {
	app := newTestApp(t)
	registerHooks(t, app)

	jobs := app.Cron().Jobs()
	var found bool
	for _, j := range jobs {
		if j.Id() == "leveled-assignments" {
			found = true
			assert.Equal(t, "30 0 * * *", j.Expression())
			break
		}
	}
	assert.True(t, found, "leveled-assignments cron must be registered")
}
