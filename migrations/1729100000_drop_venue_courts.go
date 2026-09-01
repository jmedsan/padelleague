package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("venues")
		if err != nil {
			return err
		}
		col.Fields.RemoveByName("courts")
		return app.Save(col)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("venues")
		if err != nil {
			return err
		}
		col.Fields.Add(&core.NumberField{Name: "courts"})
		return app.Save(col)
	})
}
