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
			Name: "penalty_points",
		})
		col.Fields.Add(&core.NumberField{
			Name: "default_penalty",
		})
		return app.Save(col)
	}, nil)
}
