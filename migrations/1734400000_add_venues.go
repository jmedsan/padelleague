package migrations

import (
	"fmt"
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

		records, err := app.FindRecordsByFilter("venues", "name = 'Wurko'", "", 1, 0)
		if err != nil {
			return err
		}
		if len(records) > 0 {
			records[0].Set("name", "Wurko Pádel")
			if err := app.Save(records[0]); err != nil {
				return err
			}
		}

		return nil
	}, func(app core.App) error {
		toDelete := []string{"Indoor SC", "CD San Benito", "Magma", "GoFit", "CD La Matanza", "Las Palmeras"}
		for _, name := range toDelete {
			records, err := app.FindRecordsByFilter("venues", fmt.Sprintf("name = '%s'", name), "", 1, 0)
			if err != nil {
				slog.Error("migration rollback find venue", "name", name, "err", err)
				continue
			}
			for _, r := range records {
				if err := app.Delete(r); err != nil {
					slog.Error("migration rollback delete venue", "name", name, "err", err)
				}
			}
		}

		records, err := app.FindRecordsByFilter("venues", "name = 'Wurko Pádel'", "", 1, 0)
		if err == nil && len(records) > 0 {
			records[0].Set("name", "Wurko")
			if err := app.Save(records[0]); err != nil {
				slog.Error("migration rollback rename venue", "err", err)
			}
		}

		return nil
	})
}
