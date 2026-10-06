package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

const documentAcksUniqueIndex = "CREATE UNIQUE INDEX idx_document_acks_user_competition ON document_acks (user, competition)"

// Makes (user, competition) unique on `document_acks`: two concurrent
// acknowledgments could each create a row. Existing duplicates are merged
// first (the lowest id keeps the union of every row's documents), because the
// index cannot be created while duplicates exist.
func init() {
	m.Register(documentAcksUnique, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("document_acks")
		if err != nil {
			return nil
		}
		col.RemoveIndex("idx_document_acks_user_competition")
		return app.Save(col)
	})
}

func documentAcksUnique(app core.App) error {
	if err := mergeDuplicateAcks(app); err != nil {
		return err
	}
	col, err := app.FindCollectionByNameOrId("document_acks")
	if err != nil {
		return err
	}
	col.Indexes = append(col.Indexes, documentAcksUniqueIndex)
	return app.Save(col)
}

func mergeDuplicateAcks(app core.App) error {
	acks, err := app.FindRecordsByFilter("document_acks", "", "id", 0, 0)
	if err != nil {
		return err
	}
	keepers := map[string]*core.Record{}
	for _, ack := range acks {
		key := ack.GetString("user") + "|" + ack.GetString("competition")
		keeper, seen := keepers[key]
		if !seen {
			keepers[key] = ack
			continue
		}
		merged := append(keeper.GetStringSlice("documents"), ack.GetStringSlice("documents")...)
		keeper.Set("documents", types.JSONArray[string](merged))
		if err := app.Save(keeper); err != nil {
			return err
		}
		if err := app.Delete(ack); err != nil {
			return err
		}
	}
	return nil
}
