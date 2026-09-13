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
			&core.NumberField{Name: "max_pending_matches", Min: floatPtr(0)},
		)
		if err := app.Save(competitions); err != nil {
			return err
		}

		settings, err := app.FindCollectionByNameOrId("app_settings")
		if err != nil {
			return err
		}
		settings.Fields.Add(
			&core.NumberField{Name: "default_max_pending_matches", Min: floatPtr(0)},
		)
		if err := app.Save(settings); err != nil {
			return err
		}

		// Seed existing competition rows with the rulebook default (2).
		comps, err := app.FindRecordsByFilter("competitions", "", "", 0, 0, nil)
		if err != nil {
			return err
		}
		for _, c := range comps {
			c.Set("max_pending_matches", 2)
			if err := app.Save(c); err != nil {
				return err
			}
		}

		// Seed the existing app_settings row with the default value.
		rows, err := app.FindRecordsByFilter("app_settings", "", "", 1, 0, nil)
		if err != nil || len(rows) == 0 {
			return err
		}
		rows[0].Set("default_max_pending_matches", 2)
		return app.Save(rows[0])
	}, func(app core.App) error {
		if competitions, err := app.FindCollectionByNameOrId("competitions"); err == nil {
			competitions.Fields.RemoveByName("max_pending_matches")
			_ = app.Save(competitions)
		}
		if settings, err := app.FindCollectionByNameOrId("app_settings"); err == nil {
			settings.Fields.RemoveByName("default_max_pending_matches")
			_ = app.Save(settings)
		}
		return nil
	})
}
