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
	Register(app, svc, nil)
}

func registerHooksWithNotifier(t *testing.T, app *tests.TestApp) {
	t.Helper()
	svc := league.New(app, nil)
	notifier := notify.NewNotifier(app, "", "")
	Register(app, svc, notifier)
}

func makeAdminUser(t *testing.T, app core.App) {
	t.Helper()
	n := userSeq.Add(1)
	col, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	r := core.NewRecord(col)
	r.Set("email", fmt.Sprintf("hookadmin%d@test.local", n))
	r.Set("display_name", fmt.Sprintf("Hook Admin %d", n))
	r.SetPassword("testpass123456")
	r.SetVerified(true)
	r.Set("role", "admin")
	require.NoError(t, app.Save(r))
}

func makeUser(t *testing.T, app core.App, role string) *core.Record {
	t.Helper()
	n := userSeq.Add(1)
	col, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	r := core.NewRecord(col)
	r.Set("email", fmt.Sprintf("hookuser%d@test.local", n))
	r.Set("display_name", fmt.Sprintf("Hook User %d", n))
	r.SetPassword("testpass123456")
	r.SetVerified(true)
	if role != "" {
		r.Set("role", role)
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

// --- Status transition tests: allowed ---

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

// --- Status transition tests: rejected ---

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

// --- Default role on user creation ---

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
	assert.Equal(t, "player", saved.GetString("role"))
}

func TestDefaultRole_ExplicitRole_Preserved(t *testing.T) {
	app := newTestApp(t)
	registerHooks(t, app)

	col, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	r := core.NewRecord(col)
	r.Set("email", "hasrole@test.local")
	r.Set("display_name", "Has Role")
	r.Set("role", "admin")
	r.SetPassword("testpass123456")
	r.SetVerified(true)
	require.NoError(t, app.Save(r))

	saved, err := app.FindRecordById("users", r.Id)
	require.NoError(t, err)
	assert.Equal(t, "admin", saved.GetString("role"))
}

// --- Cron registration ---

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

// --- Playoff advance notification tests (S-3) ---

func TestAdvancePlayoffFailure_NotifiesAdmins(t *testing.T) {
	app := newTestApp(t)
	makeAdminUser(t, app)
	registerHooksWithNotifier(t, app)

	p1 := makePair(t, app, "HkA")
	p2 := makePair(t, app, "HkB")
	p3 := makePair(t, app, "HkC")
	p4 := makePair(t, app, "HkD")
	comp := makePlayoffComp(t, app, []*core.Record{p1, p2, p3, p4})

	m1 := makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)
	m2 := makeMatch(t, app, comp.Id, p3.Id, p4.Id, 1)
	_ = makeMatch(t, app, comp.Id, "", "", 2)

	require.NoError(t, transitionMatch(t, app, m1.Id, league.StatusFinal,
		map[string]any{"scores": "6-3 6-4", "winner": p1.Id}))

	app.OnRecordUpdate("matches").BindFunc(func(e *core.RecordEvent) error {
		if int(e.Record.GetFloat("round_number")) == 2 {
			return fmt.Errorf("injected failure for round-2 seeding")
		}
		return e.Next()
	})

	require.NoError(t, transitionMatch(t, app, m2.Id, league.StatusFinal,
		map[string]any{"scores": "6-1 6-2", "winner": p4.Id}))

	notifs, err := app.FindRecordsByFilter("notifications",
		"type = 'admin_message' && title = 'Error en avance de playoff'",
		"", 0, 0, nil)
	require.NoError(t, err)
	require.Equal(t, 1, len(notifs), "expected exactly one admin notification for playoff failure")
	assert.Contains(t, notifs[0].GetString("body"), "Revisa el panel de administración")
}

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

// --- Scheduling reminder cron tests ---

func makeLeagueComp(t *testing.T, app core.App, pairs []*core.Record, start, end time.Time, rounds int) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("competitions")
	require.NoError(t, err)
	r := core.NewRecord(col)
	r.Set("name", "Sched Test League")
	r.Set("type", "league")
	r.Set("active", true)
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

	// Check notification was sent
	notifs, err := app.FindRecordsByFilter("notifications",
		"type = 'scheduling'", "", 0, 0, nil)
	require.NoError(t, err)
	require.NotEmpty(t, notifs, "should send at least one scheduling reminder")

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
	notifs, err := app.FindRecordsByFilter("notifications",
		"type = 'scheduling'", "", 0, 0, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, notifs, "round 1 deadline must use stored rounds=3, not pending count")

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
	assert.NotEmpty(t, notifs, "a recovery-phase competition must still remind")
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
