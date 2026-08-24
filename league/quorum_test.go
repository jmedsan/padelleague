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

func TestConfirmStaleMatches_Finalizes(t *testing.T) {
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
	assert.NotEmpty(t, notifier.calls)
}

func TestConfirmStaleMatches_NotExpired(t *testing.T) {
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

func TestConfirmStaleMatches_NoTimeout(t *testing.T) {
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
