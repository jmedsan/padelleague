package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Scope document_acks list/view to the owning user. The original rule
// (@request.auth.id != ”) let any authenticated user read every player's
// acknowledgment records via the auto-API; all app reads go through
// server-side queries that bypass API rules, so owner-scoping is safe.
func init() {
	m.Register(func(app core.App) error {
		acks, err := app.FindCollectionByNameOrId("document_acks")
		if err != nil {
			return err
		}
		acks.ListRule = strPtr("user = @request.auth.id")
		acks.ViewRule = strPtr("user = @request.auth.id")
		return app.Save(acks)
	}, func(app core.App) error {
		acks, err := app.FindCollectionByNameOrId("document_acks")
		if err != nil {
			return err
		}
		acks.ListRule = strPtr("@request.auth.id != ''")
		acks.ViewRule = strPtr("@request.auth.id != ''")
		return app.Save(acks)
	})
}
