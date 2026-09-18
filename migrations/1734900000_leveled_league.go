package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// --- competitions: target_matches, open_assignments, seed_pairs ---
		competitions, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return err
		}
		competitions.Fields.Add(
			&core.NumberField{Name: "target_matches", Min: floatPtr(0)},
			&core.NumberField{Name: "open_assignments", Min: floatPtr(0)},
			&core.RelationField{
				Name:          "seed_pairs",
				CollectionId:  mustCollectionID(app, "pairs"),
				MaxSelect:     0,
				CascadeDelete: false,
			},
		)
		if err := app.Save(competitions); err != nil {
			return err
		}

		// --- matches: round_number optional, arrange_by, finalized_at ---
		matches, err := app.FindCollectionByNameOrId("matches")
		if err != nil {
			return err
		}

		roundField := matches.Fields.GetByName("round_number").(*core.NumberField)
		roundField.Required = false

		matches.Fields.Add(
			&core.DateField{Name: "arrange_by"},
			&core.DateField{Name: "finalized_at"},
		)
		if err := app.Save(matches); err != nil {
			return err
		}

		// Backfill finalized_at for existing final matches: use date if set, else created.
		finals, err := app.FindRecordsByFilter("matches", "status = 'final'", "", 0, 0, nil)
		if err != nil {
			return err
		}
		for _, rec := range finals {
			d := rec.GetDateTime("date")
			if d.IsZero() {
				d = rec.GetDateTime("created")
			}
			rec.Set("finalized_at", d)
			if err := app.Save(rec); err != nil {
				return err
			}
		}

		// --- notifications: match_assigned type ---
		notifs, err := app.FindCollectionByNameOrId("notifications")
		if err != nil {
			return err
		}
		typeField := notifs.Fields.GetByName("type").(*core.SelectField)
		typeField.Values = append(typeField.Values, "match_assigned")
		return app.Save(notifs)
	}, func(app core.App) error {
		// Down: remove added fields.
		if competitions, err := app.FindCollectionByNameOrId("competitions"); err == nil {
			competitions.Fields.RemoveByName("target_matches")
			competitions.Fields.RemoveByName("open_assignments")
			competitions.Fields.RemoveByName("seed_pairs")
			_ = app.Save(competitions)
		}

		if matches, err := app.FindCollectionByNameOrId("matches"); err == nil {
			roundField := matches.Fields.GetByName("round_number").(*core.NumberField)
			roundField.Required = true
			matches.Fields.RemoveByName("arrange_by")
			matches.Fields.RemoveByName("finalized_at")
			_ = app.Save(matches)
		}

		if notifs, err := app.FindCollectionByNameOrId("notifications"); err == nil {
			typeField := notifs.Fields.GetByName("type").(*core.SelectField)
			filtered := typeField.Values[:0]
			for _, v := range typeField.Values {
				if v != "match_assigned" {
					filtered = append(filtered, v)
				}
			}
			typeField.Values = filtered
			_ = app.Save(notifs)
		}
		return nil
	})
}

func mustCollectionID(app core.App, name string) string {
	col, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		panic("collection not found: " + name)
	}
	return col.Id
}
