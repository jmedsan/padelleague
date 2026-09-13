package migrations

import (
	"log/slog"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		venues, err := app.FindCollectionByNameOrId("venues")
		if err != nil {
			return err
		}

		newVenues := []string{"Indoor SC", "CD San Benito", "Magma", "GoFit", "CD La Matanza", "Las Palmeras"}
		for _, name := range newVenues {
			v := core.NewRecord(venues)
			v.Set("name", name)
			if err := app.Save(v); err != nil {
				return err
			}
		}
		return nil
	}, func(app core.App) error {
		venues, err := app.FindCollectionByNameOrId("venues")
		if err != nil {
			return nil
		}

		toRemove := []string{"Indoor SC", "CD San Benito", "Magma", "GoFit", "CD La Matanza", "Las Palmeras"}
		for _, name := range toRemove {
			recs, err := app.FindRecordsByFilter("venues", "name = {:name}", "", 0, 0, map[string]any{"name": name})
			if err != nil || len(recs) == 0 {
				continue
			}
			for _, rec := range recs {
				if err := app.Delete(rec); err != nil {
					slog.Error("migration rollback delete venue", "name", name, "err", err)
				}
			}
		}
		_ = venues
		return nil
	})
}
