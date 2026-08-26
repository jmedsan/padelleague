package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		comps, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return err
		}

		comps.Fields.Add(
			&core.DateField{Name: "start_date"},
			&core.DateField{Name: "end_date"},
			&core.NumberField{Name: "arrange_grace_days"},
			&core.BoolField{Name: "auto_flag"},
			&core.TextField{Name: "walkover_score"},
		)

		if err := app.Save(comps); err != nil {
			return err
		}

		// Backfill existing competitions with defaults.
		allComps, _ := app.FindRecordsByFilter("competitions", "id != ''", "", 0, 0, nil)
		for _, c := range allComps {
			if c.GetFloat("arrange_grace_days") == 0 {
				c.Set("arrange_grace_days", 3)
			}
			if c.GetString("walkover_score") == "" {
				c.Set("walkover_score", "6-0 6-0")
			}
			c.Set("auto_flag", false)
			if err := app.Save(c); err != nil {
				return err
			}
		}

		matches, err := app.FindCollectionByNameOrId("matches")
		if err != nil {
			return err
		}

		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		matches.Fields.Add(
			&core.NumberField{Name: "last_warn_level"},
			&core.TextField{Name: "review_type"},
			&core.RelationField{
				Name:         "walkover_requested_by",
				CollectionId: users.Id,
				MaxSelect:    1,
			},
		)

		return app.Save(matches)
	}, func(app core.App) error {
		comps, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return nil
		}
		comps.Fields.RemoveByName("start_date")
		comps.Fields.RemoveByName("end_date")
		comps.Fields.RemoveByName("arrange_grace_days")
		comps.Fields.RemoveByName("auto_flag")
		comps.Fields.RemoveByName("walkover_score")
		if err := app.Save(comps); err != nil {
			return err
		}

		matches, err := app.FindCollectionByNameOrId("matches")
		if err != nil {
			return nil
		}
		matches.Fields.RemoveByName("last_warn_level")
		matches.Fields.RemoveByName("review_type")
		matches.Fields.RemoveByName("walkover_requested_by")

		return app.Save(matches)
	})
}
