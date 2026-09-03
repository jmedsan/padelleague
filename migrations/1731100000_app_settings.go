package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		col := core.NewBaseCollection("app_settings")
		col.Fields.Add(
			&core.NumberField{Name: "quorum_timeout_hours", Min: floatPtr(0)},
			&core.NumberField{Name: "arrange_grace_days", Min: floatPtr(0)},
			&core.TextField{Name: "walkover_score", Max: 20},
			&core.NumberField{Name: "default_penalty", Min: floatPtr(0)},
			&core.NumberField{Name: "recovery_days", Min: floatPtr(0)},
			&core.BoolField{Name: "play_twice"},
			&core.SelectField{Name: "gender_type", Values: []string{"free", "male", "female", "mixed"}, MaxSelect: 1},
			&core.NumberField{Name: "invite_max_uses", Min: floatPtr(1)},
			&core.NumberField{Name: "invite_expiration_days", Min: floatPtr(1)},
		)
		col.ListRule = nil
		col.ViewRule = nil
		col.CreateRule = nil
		col.UpdateRule = nil
		col.DeleteRule = nil
		if err := app.Save(col); err != nil {
			return err
		}

		rec := core.NewRecord(col)
		rec.Set("quorum_timeout_hours", 48)
		rec.Set("arrange_grace_days", 3)
		rec.Set("walkover_score", "6-0 6-0")
		rec.Set("default_penalty", 3)
		rec.Set("recovery_days", 14)
		rec.Set("play_twice", false)
		rec.Set("gender_type", "free")
		rec.Set("invite_max_uses", 10)
		rec.Set("invite_expiration_days", 7)
		return app.Save(rec)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("app_settings")
		if err != nil {
			return nil
		}
		return app.Delete(col)
	})
}

func floatPtr(v float64) *float64 { return &v }
