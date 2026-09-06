package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// L2: Cap thread message and rejection text lengths
		messages, err := app.FindCollectionByNameOrId("match_messages")
		if err != nil {
			return err
		}
		messages.Fields.Add(
			&core.TextField{Name: "content", Max: 2000},
			&core.TextField{Name: "rejection_reason", Max: 500},
			&core.TextField{Name: "rejection_text", Max: 2000},
		)
		if err := app.Save(messages); err != nil {
			return err
		}

		// L1: Protect document file field so only authenticated users
		// with a file token can download reference documents
		docs, err := app.FindCollectionByNameOrId("documents")
		if err != nil {
			return err
		}
		docs.Fields.Add(
			&core.FileField{Name: "file", MaxSelect: 1, MaxSize: 25 << 20, Protected: true},
		)
		return app.Save(docs)
	}, func(app core.App) error {
		if messages, err := app.FindCollectionByNameOrId("match_messages"); err == nil {
			messages.Fields.Add(
				&core.TextField{Name: "content"},
				&core.TextField{Name: "rejection_reason"},
				&core.TextField{Name: "rejection_text"},
			)
			_ = app.Save(messages)
		}
		if docs, err := app.FindCollectionByNameOrId("documents"); err == nil {
			docs.Fields.Add(
				&core.FileField{Name: "file", MaxSelect: 1, MaxSize: 25 << 20},
			)
			_ = app.Save(docs)
		}
		return nil
	})
}
