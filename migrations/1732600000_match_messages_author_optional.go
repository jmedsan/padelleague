package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Allows match_messages.author to be empty, so the quorum cron can write a
// system-authored timeline entry (rendered as "Sistema") when it auto-accepts
// a result with no human actor.
func init() {
	m.Register(func(app core.App) error {
		msgs, err := app.FindCollectionByNameOrId("match_messages")
		if err != nil {
			return err
		}
		authorField := msgs.Fields.GetByName("author").(*core.RelationField)
		authorField.Required = false
		return app.Save(msgs)
	}, func(app core.App) error {
		msgs, err := app.FindCollectionByNameOrId("match_messages")
		if err != nil {
			return err
		}
		authorField := msgs.Fields.GetByName("author").(*core.RelationField)
		authorField.Required = true
		return app.Save(msgs)
	})
}
