package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		settingsCol, err := app.FindCollectionByNameOrId("app_settings")
		if err != nil {
			return err
		}
		settingsCol.Fields.Add(&core.TextField{Name: "league_timezone"})
		if err := app.Save(settingsCol); err != nil {
			return err
		}

		recs, err := app.FindRecordsByFilter("app_settings", "", "", 1, 0, nil)
		if err == nil && len(recs) > 0 {
			recs[0].Set("league_timezone", "Atlantic/Canary")
			if err := app.Save(recs[0]); err != nil {
				return err
			}
		}
		return nil
	}, func(app core.App) error {
		settingsCol, err := app.FindCollectionByNameOrId("app_settings")
		if err != nil {
			return err
		}
		settingsCol.Fields.RemoveByName("league_timezone")
		return app.Save(settingsCol)
	})
}
