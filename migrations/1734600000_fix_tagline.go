package migrations

import (
	"github.com/pocketbase/pocketbase/core"
)

func init() {
	core.SystemMigrations.Register(func(app core.App) error {
		rec, err := app.FindFirstRecordByFilter("app_settings", "id != ''")
		if err != nil {
			return nil
		}
		if rec.GetString("league_tagline") == "A La Pelota" {
			rec.Set("league_tagline", "A La Bola")
			return app.Save(rec)
		}
		return nil
	}, func(app core.App) error {
		rec, err := app.FindFirstRecordByFilter("app_settings", "id != ''")
		if err != nil {
			return nil
		}
		if rec.GetString("league_tagline") == "A La Bola" {
			rec.Set("league_tagline", "A La Pelota")
			return app.Save(rec)
		}
		return nil
	})
}
