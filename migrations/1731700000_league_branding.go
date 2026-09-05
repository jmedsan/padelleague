package migrations

import (
	"log/slog"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		settingsCol, err := app.FindCollectionByNameOrId("app_settings")
		if err != nil {
			return err
		}
		settingsCol.Fields.Add(
			&core.FileField{Name: "league_logo", MaxSelect: 1, MaxSize: 5 << 20},
			&core.TextField{Name: "league_name"},
			&core.TextField{Name: "league_tagline"},
		)
		if err := app.Save(settingsCol); err != nil {
			return err
		}

		records, err := app.FindRecordsByFilter("app_settings", "", "", 1, 0, nil)
		if err != nil {
			return err
		}
		if len(records) > 0 {
			rec := records[0]
			rec.Set("league_name", "Liga Dale Fuerte")
			rec.Set("league_tagline", "A La Pelota")
			if err := app.Save(rec); err != nil {
				return err
			}
		}

		sponsorsCol, err := app.FindCollectionByNameOrId("sponsors")
		if err != nil {
			return err
		}
		sponsorsCol.Fields.Add(&core.BoolField{Name: "is_global"})
		if err := app.Save(sponsorsCol); err != nil {
			return err
		}

		decathlon, err := app.FindFirstRecordByFilter("sponsors", "name = 'Decathlon'")
		if err != nil {
			slog.Info("league_branding migration: no Decathlon sponsor found, skipping global flag")
			return nil
		}
		decathlon.Set("is_global", true)
		if err := app.Save(decathlon); err != nil {
			return err
		}
		slog.Info("league_branding migration: marked Decathlon sponsor as global")
		return nil
	}, func(app core.App) error {
		sponsorsCol, err := app.FindCollectionByNameOrId("sponsors")
		if err != nil {
			return err
		}
		sponsorsCol.Fields.RemoveByName("is_global")
		if err := app.Save(sponsorsCol); err != nil {
			return err
		}

		settingsCol, err := app.FindCollectionByNameOrId("app_settings")
		if err != nil {
			return err
		}
		settingsCol.Fields.RemoveByName("league_logo")
		settingsCol.Fields.RemoveByName("league_name")
		settingsCol.Fields.RemoveByName("league_tagline")
		return app.Save(settingsCol)
	})
}
