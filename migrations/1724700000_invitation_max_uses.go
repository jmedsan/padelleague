package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("invitations")
		if err != nil {
			return err
		}

		col.Fields.Add(
			&core.NumberField{Name: "max_uses"},
			&core.NumberField{Name: "use_count"},
		)

		return app.Save(col)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("invitations")
		if err != nil {
			return nil
		}

		col.Fields.RemoveByName("max_uses")
		col.Fields.RemoveByName("use_count")

		return app.Save(col)
	})
}
