package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds the `announcements` collection: persistent records for admin
// broadcasts to a competition's players. Locked down per the api_lockdown
// pattern — every read/write goes through an admin handler that bypasses
// collection rules, so no player-facing flow needs REST access.
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

		announcements := core.NewBaseCollection("announcements")
		announcements.Fields.Add(
			&core.RelationField{Name: "competition", CollectionId: competitions.Id, Required: true, MaxSelect: 1, CascadeDelete: true},
			&core.TextField{Name: "title", Required: true, Max: 200},
			&core.TextField{Name: "body", Required: true, Max: 2000},
			&core.RelationField{Name: "created_by", CollectionId: users.Id, Required: true, MaxSelect: 1},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		return app.Save(announcements)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("announcements")
		if err != nil {
			return nil
		}
		return app.Delete(c)
	})
}
