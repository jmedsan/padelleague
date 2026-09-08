package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds the `competition_events` collection: the admin activity timeline for
// a competition (activated/deactivated/finalized/settings changes). Locked
// down per the api_lockdown pattern — every write goes through an admin
// handler that bypasses collection rules.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		competitions, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return err
		}

		events := core.NewBaseCollection("competition_events")
		events.Fields.Add(
			&core.RelationField{Name: "competition", CollectionId: competitions.Id, Required: true, MaxSelect: 1, CascadeDelete: true},
			&core.RelationField{Name: "actor", CollectionId: users.Id, MaxSelect: 1},
			&core.SelectField{Name: "kind", Required: true, MaxSelect: 1,
				Values: []string{"activated", "deactivated", "finalized", "settings_changed"}},
			&core.TextField{Name: "detail", Max: 2000},
			&core.AutodateField{Name: "created", OnCreate: true},
		)
		return app.Save(events)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("competition_events")
		if err != nil {
			return nil
		}
		return app.Delete(c)
	})
}
