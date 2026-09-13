package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// serverOnlyCollections lists collections whose REST list/view access becomes
// superuser-only: handlers read them via app.Find*, which bypasses rules.
func serverOnlyCollections() []string {
	return []string{"documents", "competition_signups"}
}

func init() {
	m.Register(func(app core.App) error {
		for _, name := range serverOnlyCollections() {
			col, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			col.ListRule = nil
			col.ViewRule = nil
			if err := app.Save(col); err != nil {
				return err
			}
		}
		return nil
	}, func(app core.App) error {
		for _, name := range serverOnlyCollections() {
			col, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				continue
			}
			col.ListRule = strPtr("@request.auth.id != ''")
			col.ViewRule = strPtr("@request.auth.id != ''")
			_ = app.Save(col)
		}
		return nil
	})
}
