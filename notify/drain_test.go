package notify

import (
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/league"
)

// WaitPush must be safe while deliver keeps firing pushes: handlers do exactly
// that (the test routes drain after every request, shutdown drains while the
// server still answers). A sync.WaitGroup panics when Add races Wait at zero.
func TestWaitPush_ConcurrentWithDeliver_DoesNotPanic(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	notifier := NewNotifier(app, "", "") // keys set: sendPush queries the DB, so pushes take time
	user := makeUser(t, app, "player")
	notifier.save = func(*core.Record) error { return nil } // keep deliver fast: the race window is tiny

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for range 300 {
				notifier.NotifyPlayers([]string{user.Id}, league.Notification{Type: "general", Title: "t", Body: "b"})
			}
		}()
		go func() {
			defer wg.Done()
			for range 3000 {
				notifier.WaitPush()
			}
		}()
	}
	wg.Wait()
	notifier.WaitPush()
}

// After the shutdown drain starts, deliver must not start new pushes: the app
// is about to close, and a push started now would outlive it.
func TestWaitPushTimeout_StopsNewPushes(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	notifier := NewNotifier(app, "", "")
	user := makeUser(t, app, "player")

	require.Zero(t, notifier.WaitPushTimeout(time.Second))
	notifier.NotifyPlayers([]string{user.Id}, league.Notification{Type: "general", Title: "t", Body: "b"})

	assert.Zero(t, notifier.pendingPushes(), "no push may start after the shutdown drain")
	notifs, err := app.FindRecordsByFilter("notifications", "user = {:u}", "", 0, 0, map[string]any{"u": user.Id})
	require.NoError(t, err)
	assert.Len(t, notifs, 1, "the in-app notification is still saved")
}

// WaitPush returns only once every started push has ended.
func TestWaitPush_BlocksUntilPushEnds(t *testing.T) {
	t.Parallel()
	notifier := NewNotifier(newTestApp(t), "", "")
	require.True(t, notifier.startPush())

	returned := make(chan struct{})
	go func() {
		notifier.WaitPush()
		close(returned)
	}()
	select {
	case <-returned:
		t.Fatal("WaitPush returned while a push was in flight")
	case <-time.After(50 * time.Millisecond):
	}

	notifier.endPush()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("WaitPush did not return after the push ended")
	}
}
