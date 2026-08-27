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

		col := core.NewBaseCollection("search_history")
		col.Fields.Add(
			&core.RelationField{Name: "user", CollectionId: users.Id, Required: true, MaxSelect: 1, CascadeDelete: true},
			&core.TextField{Name: "query", Required: true, Max: 500},
			&core.AutodateField{Name: "created", OnCreate: true},
		)
		col.ListRule = strPtr("user = @request.auth.id")
		col.ViewRule = strPtr("user = @request.auth.id")
		col.CreateRule = strPtr("user = @request.auth.id")
		return app.Save(col)
	}, func(app core.App) error {
		if c, err := app.FindCollectionByNameOrId("search_history"); err == nil {
			return app.Delete(c)
		}
		return nil
	})
}
