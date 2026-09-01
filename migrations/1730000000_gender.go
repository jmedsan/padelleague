package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// users.gender (NOT Required — existing rows have no gender)
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		users.Fields.Add(&core.SelectField{
			Name:      "gender",
			Values:    []string{"male", "female"},
			MaxSelect: 1,
		})
		if err := app.Save(users); err != nil {
			return err
		}

		// competitions.gender_type
		comps, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return err
		}
		comps.Fields.Add(&core.SelectField{
			Name:      "gender_type",
			Values:    []string{"free", "male", "female", "mixed"},
			MaxSelect: 1,
		})
		if err := app.Save(comps); err != nil {
			return err
		}

		// Backfill existing competitions to "free"
		records, err := app.FindRecordsByFilter("competitions", "gender_type = ''", "", 0, 0)
		if err != nil {
			return err
		}
		for _, r := range records {
			r.Set("gender_type", "free")
			if err := app.Save(r); err != nil {
				return err
			}
		}

		return nil
	}, func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		users.Fields.RemoveByName("gender")
		if err := app.Save(users); err != nil {
			return err
		}

		comps, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return err
		}
		comps.Fields.RemoveByName("gender_type")
		return app.Save(comps)
	})
}
