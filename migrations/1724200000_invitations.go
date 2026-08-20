package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

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

		invitations := core.NewBaseCollection("invitations")
		invitations.Fields.Add(
			&core.TextField{Name: "token", Required: true, Max: 32},
			&core.TextField{Name: "email"},
			&core.RelationField{Name: "competition", CollectionId: competitions.Id, MaxSelect: 1},
			&core.RelationField{Name: "created_by", CollectionId: users.Id, Required: true, MaxSelect: 1},
			&core.RelationField{Name: "used_by", CollectionId: users.Id, MaxSelect: 1},
			&core.DateField{Name: "used_at"},
			&core.DateField{Name: "expires_at"},
			&core.SelectField{Name: "status", Values: []string{"pending", "used", "expired"}, MaxSelect: 1, Required: true},
		)
		setAdminRules(invitations)

		return app.Save(invitations)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("invitations")
		if err == nil {
			return app.Delete(col)
		}
		return nil
	})
}
