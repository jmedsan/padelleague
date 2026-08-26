package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		competitions, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return err
		}

		competitions.Fields.Add(
			&core.NumberField{Name: "recovery_days"},
			&core.BoolField{Name: "finalized"},
		)

		return app.Save(competitions)
	}, func(app core.App) error {
		competitions, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return nil
		}
		competitions.Fields.RemoveByName("recovery_days")
		competitions.Fields.RemoveByName("finalized")
		return app.Save(competitions)
	})
}
