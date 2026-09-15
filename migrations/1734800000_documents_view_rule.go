package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		docs, err := app.FindCollectionByNameOrId("documents")
		if err != nil {
			return err
		}
		// Restore ViewRule so PocketBase's file endpoint accepts file tokens
		// from authenticated users. ListRule stays nil (superuser-only) —
		// the collection can't be enumerated via the API, only accessed
		// server-side via app.Find*. The file field remains Protected: true.
		docs.ViewRule = strPtr("@request.auth.id != ''")
		return app.Save(docs)
	}, func(app core.App) error {
		docs, err := app.FindCollectionByNameOrId("documents")
		if err != nil {
			return err
		}
		docs.ViewRule = nil
		return app.Save(docs)
	})
}
