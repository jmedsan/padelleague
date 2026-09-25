package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// pairLevelValues are the pairs.level select field options, matching
// league.Levels in league/rating.go.
var pairLevelValues = []string{
	"beginner", "beginner_high", "intermediate_low", "intermediate",
	"intermediate_high", "advanced", "advanced_high", "unranked",
}

func init() {
	m.Register(func(app core.App) error {
		pairs, err := app.FindCollectionByNameOrId("pairs")
		if err != nil {
			return err
		}
		pairs.Fields.Add(&core.SelectField{Name: "level", Values: pairLevelValues, MaxSelect: 1})
		if err := app.Save(pairs); err != nil {
			return err
		}

		// Backfill: every existing pair starts unranked.
		existing, err := app.FindRecordsByFilter("pairs", "id != ''", "", 0, 0, nil)
		if err != nil {
			return err
		}
		for _, rec := range existing {
			rec.Set("level", "unranked")
			if err := app.Save(rec); err != nil {
				return err
			}
		}

		competitions, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return err
		}
		competitions.Fields.Add(
			&core.JSONField{Name: "balls_status"},
			&core.JSONField{Name: "balls_delivered_at"},
			&core.JSONField{Name: "balls_delivered_by"},
		)
		competitions.Fields.RemoveByName("seed_pairs")
		return app.Save(competitions)
	}, func(app core.App) error {
		if pairs, err := app.FindCollectionByNameOrId("pairs"); err == nil {
			pairs.Fields.RemoveByName("level")
			_ = app.Save(pairs)
		}

		if competitions, err := app.FindCollectionByNameOrId("competitions"); err == nil {
			competitions.Fields.RemoveByName("balls_status")
			competitions.Fields.RemoveByName("balls_delivered_at")
			competitions.Fields.RemoveByName("balls_delivered_by")
			competitions.Fields.Add(&core.RelationField{
				Name:          "seed_pairs",
				CollectionId:  mustCollectionID(app, "pairs"),
				MaxSelect:     100,
				CascadeDelete: false,
			})
			_ = app.Save(competitions)
		}
		return nil
	})
}
