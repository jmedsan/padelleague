package migrations

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPairSwapMatch(t *testing.T, app core.App) (match *core.Record, p1, p2 *core.Record) {
	t.Helper()
	p1 = makeSlotTestPair(t, app, "Swap A")
	p2 = makeSlotTestPair(t, app, "Swap B")
	comp := core.NewRecord(mustCollection(t, app, "competitions"))
	comp.Set("name", "Swap comp")
	comp.Set("type", "league")
	require.NoError(t, app.Save(comp))
	match = core.NewRecord(mustCollection(t, app, "matches"))
	match.Set("competition", comp.Id)
	match.Set("pair1", p1.Id)
	match.Set("pair2", p2.Id)
	match.Set("status", "pending")
	match.Set("round_number", 1)
	require.NoError(t, app.Save(match))
	return match, p1, p2
}

func newMessage(t *testing.T, app core.App, matchID, author, typ string) *core.Record {
	t.Helper()
	msg := core.NewRecord(mustCollection(t, app, "match_messages"))
	msg.Set("match", matchID)
	msg.Set("author", author)
	msg.Set("type", typ)
	msg.Set("content", "x")
	require.NoError(t, app.Save(msg))
	return msg
}

func TestBackfillAuthorPair_StampsPlayersAndSkipsOthers(t *testing.T) {
	t.Parallel()
	app := newSlotTestApp(t)
	match, p1, p2 := newPairSwapMatch(t, app)
	outsider := makeSlotTestPair(t, app, "Swap Out").GetString("player1")

	byP1 := newMessage(t, app, match.Id, p1.GetString("player1"), "chat")
	byP2 := newMessage(t, app, match.Id, p2.GetString("player2"), "chat")
	byOutsider := newMessage(t, app, match.Id, outsider, "chat")
	adminAction := newMessage(t, app, match.Id, p1.GetString("player2"), "admin_action")
	system := newMessage(t, app, match.Id, "", "result_event")

	require.NoError(t, backfillAuthorPair(app))

	pairOf := func(m *core.Record) string {
		got, err := app.FindRecordById("match_messages", m.Id)
		require.NoError(t, err)
		return got.GetString("author_pair")
	}
	assert.Equal(t, p1.Id, pairOf(byP1))
	assert.Equal(t, p2.Id, pairOf(byP2))
	assert.Empty(t, pairOf(byOutsider))
	assert.Empty(t, pairOf(adminAction), "an admin_action carries no pair")
	assert.Empty(t, pairOf(system))
}

func TestMatchActorPairs_BackfillAndClearAdminActions(t *testing.T) {
	t.Parallel()
	app := newSlotTestApp(t)
	match, p1, p2 := newPairSwapMatch(t, app)
	match.Set("submitted_by", p1.GetString("player1"))
	match.Set("disputed_by", p2.GetString("player2"))
	match.Set("submitted_by_pair", "")
	match.Set("disputed_by_pair", "")
	require.NoError(t, app.SaveNoValidate(match))

	stale := newMessage(t, app, match.Id, p1.GetString("player1"), "admin_action")
	stale.Set("author_pair", p1.Id)
	require.NoError(t, app.SaveNoValidate(stale))

	require.NoError(t, backfillActorPairs(app))
	require.NoError(t, clearAdminActionPairs(app))

	got, err := app.FindRecordById("matches", match.Id)
	require.NoError(t, err)
	assert.Equal(t, p1.Id, got.GetString("submitted_by_pair"))
	assert.Equal(t, p2.Id, got.GetString("disputed_by_pair"))
	msg, err := app.FindRecordById("match_messages", stale.Id)
	require.NoError(t, err)
	assert.Empty(t, msg.GetString("author_pair"))
}
