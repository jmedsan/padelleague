package migrations

import (
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
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

	// Record.Save skips the update for AutodateField values that already
	// equal the record's last-known value, so a plain SetRaw+Save backfill
	// silently no-ops. A direct SQL update bypasses that entirely.
	now := types.NowDateTime()
	query := "UPDATE " + collectionName + " SET created = {:now}, updated = {:now} WHERE created = '' OR created IS NULL"
	_, err = app.DB().NewQuery(query).Bind(dbx.Params{"now": now}).Execute()
	return err
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
