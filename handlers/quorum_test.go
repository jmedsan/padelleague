package handlers

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/league"
	"padelleague/notify"
)

func TestCheckQuorumTimeout_Expired(t *testing.T) {
	app := newTestApp(t)
	p1 := makePair(t, app, "Pair A")
	p2 := makePair(t, app, "Pair B")
	user := makeUser(t, app, "Test Player", "")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	comp.Set("quorum_timeout_hours", 1)
	require.NoError(t, app.Save(comp))

	match := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "confirmed")
	match.Set("scores", "6-3 6-4")
	match.Set("submitted_by", user.Id)
	require.NoError(t, app.Save(match))

	pastTime := time.Now().Add(-2 * time.Hour).UTC().Format("2006-01-02 15:04:05.000Z")
	_, err := app.DB().NewQuery("UPDATE matches SET submitted_at = {:t} WHERE id = {:id}").
		Bind(map[string]any{"t": pastTime, "id": match.Id}).Execute()
	require.NoError(t, err)

	notifier := notify.NewNotifier(app, "", "")
	svc := league.New(app, notifier)
	svc.ConfirmStaleMatches()

	fresh, findErr := app.FindRecordById("matches", match.Id)
	require.NoError(t, findErr)
	assert.Equal(t, "final", fresh.GetString("status"))
	assert.Equal(t, p1.Id, fresh.GetString("winner"))
}

func TestCheckQuorumTimeout_NotExpired(t *testing.T) {
	app := newTestApp(t)
	p1 := makePair(t, app, "Pair C")
	p2 := makePair(t, app, "Pair D")
	user := makeUser(t, app, "Test Player 2", "")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	comp.Set("quorum_timeout_hours", 1)
	require.NoError(t, app.Save(comp))

	match := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "confirmed")
	match.Set("scores", "6-3 6-4")
	match.Set("submitted_by", user.Id)
	require.NoError(t, app.Save(match))

	recentTime := time.Now().Add(-5 * time.Minute).UTC().Format("2006-01-02 15:04:05.000Z")
	_, err := app.DB().NewQuery("UPDATE matches SET submitted_at = {:t} WHERE id = {:id}").
		Bind(map[string]any{"t": recentTime, "id": match.Id}).Execute()
	require.NoError(t, err)

	notifier := notify.NewNotifier(app, "", "")
	svc := league.New(app, notifier)
	svc.ConfirmStaleMatches()

	fresh, findErr := app.FindRecordById("matches", match.Id)
	require.NoError(t, findErr)
	assert.Equal(t, "confirmed", fresh.GetString("status"))
}
