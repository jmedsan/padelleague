package migrations

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocumentAcksUnique_MergesDuplicatesThenRejectsNewOnes(t *testing.T) {
	t.Parallel()
	app := newSlotTestApp(t)

	col, err := app.FindCollectionByNameOrId("document_acks")
	require.NoError(t, err)
	// The test app already ran the migration: drop the index to recreate the
	// pre-migration state, where duplicates can exist.
	col.RemoveIndex("idx_document_acks_user_competition")
	require.NoError(t, app.Save(col))

	pair := makeSlotTestPair(t, app, "Ack")
	comp := core.NewRecord(mustCollection(t, app, "competitions"))
	comp.Set("name", "Acks")
	comp.Set("type", "league")
	require.NoError(t, app.Save(comp))
	user := pair.GetString("player1")
	other := pair.GetString("player2")

	docs := make([]string, 3)
	for i := range docs {
		d := core.NewRecord(mustCollection(t, app, "documents"))
		d.Set("title", "Doc")
		require.NoError(t, app.Save(d))
		docs[i] = d.Id
	}
	newAck := func(userID string, documents ...string) {
		ack := core.NewRecord(col)
		ack.Set("user", userID)
		ack.Set("competition", comp.Id)
		ack.Set("documents", documents)
		require.NoError(t, app.Save(ack))
	}
	newAck(user, docs[0])
	newAck(user, docs[0], docs[1])
	newAck(user, docs[2])
	newAck(other, docs[1])

	require.NoError(t, documentAcksUnique(app))

	mine, err := app.FindRecordsByFilter("document_acks", "user = {:u}", "", 0, 0, map[string]any{"u": user})
	require.NoError(t, err)
	require.Len(t, mine, 1)
	assert.ElementsMatch(t, docs, mine[0].GetStringSlice("documents"))
	theirs, err := app.FindRecordsByFilter("document_acks", "user = {:u}", "", 0, 0, map[string]any{"u": other})
	require.NoError(t, err)
	require.Len(t, theirs, 1)
	assert.Equal(t, []string{docs[1]}, theirs[0].GetStringSlice("documents"))

	dup := core.NewRecord(col)
	dup.Set("user", user)
	dup.Set("competition", comp.Id)
	assert.Error(t, app.Save(dup), "a second ack for the same player and competition must be rejected")
}

func mustCollection(t *testing.T, app core.App, name string) *core.Collection {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(name)
	require.NoError(t, err)
	return col
}
