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

		col.Fields.Add(
			&core.AutodateField{Name: "created", OnCreate: true},
		)

		return app.Save(col)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("notifications")
		if err != nil {
			return nil
		}

		col.Fields.RemoveByName("created")

		return app.Save(col)
	})
}
