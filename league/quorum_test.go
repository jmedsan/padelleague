package league

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeNotifier struct {
	calls []notifyCall
}

type notifyCall struct {
	playerIDs []string
	notifType string
	title     string
}

func (f *fakeNotifier) NotifyPlayers(playerUserIDs []string, notifType, title, _, _ string) {
	f.calls = append(f.calls, notifyCall{
		playerIDs: playerUserIDs,
		notifType: notifType,
		title:     title,
	})
}

func (f *fakeNotifier) EmailPlayers(playerUserIDs []string, subject, _, _ string) {
	f.calls = append(f.calls, notifyCall{
		playerIDs: playerUserIDs,
		notifType: "email",
		title:     subject,
	})
}

func TestConfirmStaleMatches_Finalizes(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Pair A")
	p2 := makePair(t, app, "Pair B")

	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	comp.Set("quorum_timeout_hours", 1)
	require.NoError(t, app.Save(comp))

	match := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "confirmed")
	match.Set("scores", "6-3 6-4")
	match.Set("submitted_by", p1.GetString("player1"))
	require.NoError(t, app.Save(match))

	// AutodateField ignores Set() — use raw SQL to backdate
	staleTime := time.Now().Add(-2 * time.Hour).UTC().Format("2006-01-02 15:04:05.000Z")
	_, err := app.DB().NewQuery("UPDATE matches SET submitted_at = {:sa} WHERE id = {:id}").
		Bind(map[string]any{"sa": staleTime, "id": match.Id}).Execute()
	require.NoError(t, err)

	notifier := &fakeNotifier{}
	svc := New(app, notifier)
	svc.ConfirmStaleMatches()

	updated, err := app.FindRecordById("matches", match.Id)
	require.NoError(t, err)
	assert.Equal(t, "final", updated.GetString("status"))
	assert.Equal(t, p1.Id, updated.GetString("winner"))
	assert.Contains(t, updated.GetString("dispute_notes"), "Auto-confirmado")
	require.Len(t, notifier.calls, 2)
	assert.Equal(t, "general", notifier.calls[0].notifType)
	assert.Equal(t, "general", notifier.calls[1].notifType)
}

func TestConfirmStaleMatches_NotExpired(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Pair C")
	p2 := makePair(t, app, "Pair D")

	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	comp.Set("quorum_timeout_hours", 24)
	require.NoError(t, app.Save(comp))

	match := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "confirmed")
	match.Set("scores", "6-3 6-4")
	match.Set("submitted_by", p1.GetString("player1"))
	require.NoError(t, app.Save(match))

	recentTime := time.Now().Add(-1 * time.Hour).UTC().Format("2006-01-02 15:04:05.000Z")
	_, err := app.DB().NewQuery("UPDATE matches SET submitted_at = {:sa} WHERE id = {:id}").
		Bind(map[string]any{"sa": recentTime, "id": match.Id}).Execute()
	require.NoError(t, err)

	notifier := &fakeNotifier{}
	svc := New(app, notifier)
	svc.ConfirmStaleMatches()

	updated, err2 := app.FindRecordById("matches", match.Id)
	require.NoError(t, err2)
	assert.Equal(t, "confirmed", updated.GetString("status"))
}

func TestRemindPendingConfirmations_SendsReminder(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Remind A")
	p2 := makePair(t, app, "Remind B")

	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	comp.Set("confirm_reminder_hours", 6)
	comp.Set("quorum_timeout_hours", 24)
	require.NoError(t, app.Save(comp))

	match := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "confirmed")
	match.Set("scores", "6-3 6-4")
	match.Set("submitted_by", p1.GetString("player1"))
	require.NoError(t, app.Save(match))

	pastThreshold := time.Now().Add(-8 * time.Hour).UTC().Format("2006-01-02 15:04:05.000Z")
	_, err := app.DB().NewQuery("UPDATE matches SET submitted_at = {:sa} WHERE id = {:id}").
		Bind(map[string]any{"sa": pastThreshold, "id": match.Id}).Execute()
	require.NoError(t, err)

	notifier := &fakeNotifier{}
	svc := New(app, notifier)
	svc.RemindPendingConfirmations(time.Now())

	// Confirming team (p2) gets in-app + email = 2 calls
	require.Len(t, notifier.calls, 2)
	p2Players := PlayersForPair(app, p2.Id)
	assert.ElementsMatch(t, p2Players, notifier.calls[0].playerIDs)
	assert.Equal(t, "quorum_request", notifier.calls[0].notifType)
	assert.Equal(t, "email", notifier.calls[1].notifType)
	assert.ElementsMatch(t, p2Players, notifier.calls[1].playerIDs)

	// Submitting pair (p1) gets zero
	p1Players := PlayersForPair(app, p1.Id)
	for _, c := range notifier.calls {
		for _, pid := range p1Players {
			assert.NotContains(t, c.playerIDs, pid)
		}
	}

	// Flag set
	updated, err := app.FindRecordById("matches", match.Id)
	require.NoError(t, err)
	assert.True(t, updated.GetBool("confirm_reminded"))

	// Second run → no new notifications
	notifier.calls = nil
	svc.RemindPendingConfirmations(time.Now())
	assert.Empty(t, notifier.calls)
}

func TestRemindPendingConfirmations_NotYetDue(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Young A")
	p2 := makePair(t, app, "Young B")

	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	comp.Set("confirm_reminder_hours", 12)
	require.NoError(t, app.Save(comp))

	match := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "confirmed")
	match.Set("scores", "6-3 6-4")
	match.Set("submitted_by", p1.GetString("player1"))
	require.NoError(t, app.Save(match))

	recent := time.Now().Add(-2 * time.Hour).UTC().Format("2006-01-02 15:04:05.000Z")
	_, err := app.DB().NewQuery("UPDATE matches SET submitted_at = {:sa} WHERE id = {:id}").
		Bind(map[string]any{"sa": recent, "id": match.Id}).Execute()
	require.NoError(t, err)

	notifier := &fakeNotifier{}
	svc := New(app, notifier)
	svc.RemindPendingConfirmations(time.Now())
	assert.Empty(t, notifier.calls)
}

func TestRemindPendingConfirmations_ThresholdGteTimeout(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Gate A")
	p2 := makePair(t, app, "Gate B")

	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	comp.Set("confirm_reminder_hours", 24)
	comp.Set("quorum_timeout_hours", 24)
	require.NoError(t, app.Save(comp))

	match := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "confirmed")
	match.Set("scores", "6-3 6-4")
	match.Set("submitted_by", p1.GetString("player1"))
	require.NoError(t, app.Save(match))

	old := time.Now().Add(-48 * time.Hour).UTC().Format("2006-01-02 15:04:05.000Z")
	_, err := app.DB().NewQuery("UPDATE matches SET submitted_at = {:sa} WHERE id = {:id}").
		Bind(map[string]any{"sa": old, "id": match.Id}).Execute()
	require.NoError(t, err)

	notifier := &fakeNotifier{}
	svc := New(app, notifier)
	svc.RemindPendingConfirmations(time.Now())
	assert.Empty(t, notifier.calls, "threshold >= timeout with timeout>0 must suppress reminder")
}

func TestRemindPendingConfirmations_TimeoutZero_StillReminds(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Zero A")
	p2 := makePair(t, app, "Zero B")

	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	comp.Set("confirm_reminder_hours", 6)
	// quorum_timeout_hours defaults to 0
	require.NoError(t, app.Save(comp))

	match := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "confirmed")
	match.Set("scores", "6-3 6-4")
	match.Set("submitted_by", p1.GetString("player1"))
	require.NoError(t, app.Save(match))

	old := time.Now().Add(-8 * time.Hour).UTC().Format("2006-01-02 15:04:05.000Z")
	_, err := app.DB().NewQuery("UPDATE matches SET submitted_at = {:sa} WHERE id = {:id}").
		Bind(map[string]any{"sa": old, "id": match.Id}).Execute()
	require.NoError(t, err)

	notifier := &fakeNotifier{}
	svc := New(app, notifier)
	svc.RemindPendingConfirmations(time.Now())
	require.Len(t, notifier.calls, 2, "timeout==0 must still remind")
}

func TestRemindPendingConfirmations_RearmAfterCorrection(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Rearm A")
	p2 := makePair(t, app, "Rearm B")

	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	comp.Set("confirm_reminder_hours", 6)
	comp.Set("quorum_timeout_hours", 24)
	require.NoError(t, app.Save(comp))

	match := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "confirmed")
	match.Set("scores", "6-3 6-4")
	match.Set("submitted_by", p1.GetString("player1"))
	require.NoError(t, app.Save(match))

	old := time.Now().Add(-8 * time.Hour).UTC().Format("2006-01-02 15:04:05.000Z")
	_, err := app.DB().NewQuery("UPDATE matches SET submitted_at = {:sa} WHERE id = {:id}").
		Bind(map[string]any{"sa": old, "id": match.Id}).Execute()
	require.NoError(t, err)

	notifier := &fakeNotifier{}
	svc := New(app, notifier)
	svc.RemindPendingConfirmations(time.Now())
	require.Len(t, notifier.calls, 2)

	// Simulate correction: reset flag and submitted_at
	fresh, err := app.FindRecordById("matches", match.Id)
	require.NoError(t, err)
	fresh.Set("confirm_reminded", false)
	require.NoError(t, app.Save(fresh))
	newOld := time.Now().Add(-8 * time.Hour).UTC().Format("2006-01-02 15:04:05.000Z")
	_, err = app.DB().NewQuery("UPDATE matches SET submitted_at = {:sa} WHERE id = {:id}").
		Bind(map[string]any{"sa": newOld, "id": match.Id}).Execute()
	require.NoError(t, err)

	notifier.calls = nil
	svc.RemindPendingConfirmations(time.Now())
	require.Len(t, notifier.calls, 2, "re-armed match must re-remind")
}

func TestRemindPendingConfirmations_AutoFinalizedSkipped(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Final A")
	p2 := makePair(t, app, "Final B")

	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	comp.Set("confirm_reminder_hours", 6)
	comp.Set("quorum_timeout_hours", 24)
	require.NoError(t, app.Save(comp))

	match := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "confirmed")
	match.Set("scores", "6-3 6-4")
	match.Set("submitted_by", p1.GetString("player1"))
	require.NoError(t, app.Save(match))

	old := time.Now().Add(-8 * time.Hour).UTC().Format("2006-01-02 15:04:05.000Z")
	_, err := app.DB().NewQuery("UPDATE matches SET submitted_at = {:sa} WHERE id = {:id}").
		Bind(map[string]any{"sa": old, "id": match.Id}).Execute()
	require.NoError(t, err)

	// Simulate auto-finalize after the initial query but before notify:
	// set status to final so the re-read check skips it.
	_, err = app.DB().NewQuery("UPDATE matches SET status = 'final' WHERE id = {:id}").
		Bind(map[string]any{"id": match.Id}).Execute()
	require.NoError(t, err)

	notifier := &fakeNotifier{}
	svc := New(app, notifier)
	svc.RemindPendingConfirmations(time.Now())
	assert.Empty(t, notifier.calls, "auto-finalized match must be skipped")
}

func TestConfirmReminderHours_Default(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	comp := makeCompetition(t, app, nil)
	assert.Equal(t, 12, ConfirmReminderHours(comp))

	comp.Set("confirm_reminder_hours", 8)
	assert.Equal(t, 8, ConfirmReminderHours(comp))
}

func TestConfirmStaleMatches_NoTimeout(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Pair E")
	p2 := makePair(t, app, "Pair F")

	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	match := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "confirmed")
	match.Set("scores", "6-3 6-4")
	require.NoError(t, app.Save(match))

	staleTime := time.Now().Add(-48 * time.Hour).UTC().Format("2006-01-02 15:04:05.000Z")
	_, err := app.DB().NewQuery("UPDATE matches SET submitted_at = {:sa} WHERE id = {:id}").
		Bind(map[string]any{"sa": staleTime, "id": match.Id}).Execute()
	require.NoError(t, err)

	notifier := &fakeNotifier{}
	svc := New(app, notifier)
	svc.ConfirmStaleMatches()

	updated, err := app.FindRecordById("matches", match.Id)
	require.NoError(t, err)
	assert.Equal(t, "confirmed", updated.GetString("status"))
}
