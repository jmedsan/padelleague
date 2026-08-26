package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		matches, err := app.FindCollectionByNameOrId("matches")
		if err != nil {
			return err
		}
		matches.Fields.Add(&core.BoolField{Name: "confirm_reminded"})
		if err := app.Save(matches); err != nil {
			return err
		}

		comps, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return err
		}
		comps.Fields.Add(&core.NumberField{Name: "confirm_reminder_hours"})
		return app.Save(comps)
	}, func(app core.App) error {
		matches, err := app.FindCollectionByNameOrId("matches")
		if err != nil {
			return err
		}
		matches.Fields.RemoveByName("confirm_reminded")
		if err := app.Save(matches); err != nil {
			return err
		}

		comps, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return err
		}
		comps.Fields.RemoveByName("confirm_reminder_hours")
		return app.Save(comps)
	})
}
