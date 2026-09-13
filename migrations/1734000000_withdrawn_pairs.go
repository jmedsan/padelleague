package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return err
		}
		col.Fields.Add(&core.JSONField{
			Name: "withdrawn_pairs",
		})
		return app.Save(col)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return nil
		}
		col.Fields.RemoveByName("withdrawn_pairs")
		return app.Save(col)
	})
}
