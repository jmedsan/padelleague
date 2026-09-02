package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("notifications")
		if err != nil {
			return err
		}
		col.Fields.Add(&core.TextField{Name: "link"})
		return app.Save(col)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("notifications")
		if err != nil {
			return err
		}
		col.Fields.RemoveByName("link")
		return app.Save(col)
	})
}
