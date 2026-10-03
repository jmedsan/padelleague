package migrations

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func emptyProposalFixture(t *testing.T, app core.App, content string) (*core.Record, *core.Record) {
	t.Helper()
	comp := jornadaFixCompetition(t, app)
	pairs := comp.GetStringSlice("pairs")
	match := jornadaFixMatch(t, app, comp.Id, pairs[0], pairs[1], "")

	col, err := app.FindCollectionByNameOrId("match_messages")
	require.NoError(t, err)
	proposal := core.NewRecord(col)
	proposal.Set("match", match.Id)
	pair, err := app.FindRecordById("pairs", pairs[0])
	require.NoError(t, err)
	proposal.Set("author", pair.GetString("player1"))
	proposal.Set("type", "result_submission")
	proposal.Set("content", content)
	proposal.Set("proposal_status", "pending")
	require.NoError(t, app.Save(proposal))

	match.SetRaw("submitted_at", types.NowDateTime())
	require.NoError(t, app.Save(match))
	return match, proposal
}

func TestRemoveEmptyResultProposals_RemovesProposalNotificationsAndSubmission(t *testing.T) {
	t.Parallel()
	app := newSlotTestApp(t)
	match, proposal := emptyProposalFixture(t, app, "")

	notes, err := app.FindCollectionByNameOrId("notifications")
	require.NoError(t, err)
	for _, tc := range []struct{ typ, body string }{
		{"match_progress", "Resultado propuesto: "},
		{"quorum_request", "Player 1 (Pareja 1) ha enviado . Confirma o contrapropón."},
		{"match_progress", "Resultado propuesto: 6-3 6-4"},
	} {
		n := core.NewRecord(notes)
		n.Set("related_match", match.Id)
		n.Set("title", "Aviso")
		n.Set("user", proposal.GetString("author"))
		n.Set("type", tc.typ)
		n.Set("body", tc.body)
		require.NoError(t, app.Save(n))
	}

	require.NoError(t, removeEmptyResultProposals(app))

	_, err = app.FindRecordById("match_messages", proposal.Id)
	assert.Error(t, err, "the empty proposal must be deleted")
	left, err := app.FindRecordsByFilter("notifications", "related_match = {:m}", "", 0, 0,
		map[string]any{"m": match.Id})
	require.NoError(t, err)
	require.Len(t, left, 1, "only the notification of a real score survives")
	assert.Equal(t, "Resultado propuesto: 6-3 6-4", left[0].GetString("body"))
	got, err := app.FindRecordById("matches", match.Id)
	require.NoError(t, err)
	assert.Empty(t, got.GetString("submitted_at"))
	assert.Empty(t, got.GetString("submitted_by"))
}

func TestRemoveEmptyResultProposals_KeepsProposalWithScore(t *testing.T) {
	t.Parallel()
	app := newSlotTestApp(t)
	match, proposal := emptyProposalFixture(t, app, "6-3 6-4")

	require.NoError(t, removeEmptyResultProposals(app))

	_, err := app.FindRecordById("match_messages", proposal.Id)
	require.NoError(t, err)
	got, err := app.FindRecordById("matches", match.Id)
	require.NoError(t, err)
	assert.NotEmpty(t, got.GetString("submitted_at"))
}
