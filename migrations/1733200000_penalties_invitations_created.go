package migrations

import (
	"time"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds created/updated to penalties and invitations, both missing since
// their original collection definitions. Existing rows have no other
// timestamp to backfill from, so created is set to the migration run time.
func init() {
	m.Register(func(app core.App) error {
		if err := addCreatedUpdated(app, "penalties"); err != nil {
			return err
		}
		return addCreatedUpdated(app, "invitations")
	}, func(app core.App) error {
		if err := removeCreatedUpdated(app, "penalties"); err != nil {
			return err
		}
		return removeCreatedUpdated(app, "invitations")
	})
}

func addCreatedUpdated(app core.App, collectionName string) error {
	col, err := app.FindCollectionByNameOrId(collectionName)
	if err != nil {
		return err
	}
	col.Fields.Add(
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	)
	if err := app.Save(col); err != nil {
		return err
	}

	records, err := app.FindRecordsByFilter(collectionName, "id != ''", "", 0, 0, nil)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, r := range records {
		r.SetRaw("created", now)
		r.SetRaw("updated", now)
		if err := app.SaveNoValidate(r); err != nil {
			return err
		}
	}
	return nil
}

func removeCreatedUpdated(app core.App, collectionName string) error {
	col, err := app.FindCollectionByNameOrId(collectionName)
	if err != nil {
		return nil
	}
	col.Fields.RemoveByName("created")
	col.Fields.RemoveByName("updated")
	return app.Save(col)
}
