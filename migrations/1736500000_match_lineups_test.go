package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackfillLineups_FreezesMatchesWithAResult(t *testing.T) {
	t.Parallel()
	app := newSlotTestApp(t)
	withResult, p1, p2 := newPairSwapMatch(t, app)
	withResult.Set("status", "final")
	require.NoError(t, app.SaveNoValidate(withResult))
	proposed, _, _ := newPairSwapMatch(t, app)
	msg := newMessage(t, app, proposed.Id, p1.GetString("player1"), "result_submission")
	require.NotEmpty(t, msg.Id)
	idle, _, _ := newPairSwapMatch(t, app)

	require.NoError(t, backfillLineups(app))

	lineup := func(id, field string) []string {
		got, err := app.FindRecordById("matches", id)
		require.NoError(t, err)
		return got.GetStringSlice(field)
	}
	assert.ElementsMatch(t, []string{p1.GetString("player1"), p1.GetString("player2")}, lineup(withResult.Id, "lineup1"))
	assert.ElementsMatch(t, []string{p2.GetString("player1"), p2.GetString("player2")}, lineup(withResult.Id, "lineup2"))
	assert.NotEmpty(t, lineup(proposed.Id, "lineup1"), "a pending result proposal counts as played")
	assert.Empty(t, lineup(idle.Id, "lineup1"), "a match with no result has no lineup")
}
