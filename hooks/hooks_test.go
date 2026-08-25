package hooks

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
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

func TestTransition_PendingToDisputed_Rejected(t *testing.T) {
	app := newTestApp(t)
	registerHooks(t, app)
	p1 := makePair(t, app, "TrRjA")
	p2 := makePair(t, app, "TrRjB")
	comp := makePlayoffComp(t, app, []*core.Record{p1, p2})
	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, 1)

	err := transitionMatch(t, app, m.Id, league.StatusDisputed, nil)
	assert.ErrorContains(t, err, "invalid transition")
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
	assert.Contains(t, notifs[0].GetString("body"), m2.Id)
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
