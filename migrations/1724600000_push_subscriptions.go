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

		subs := core.NewBaseCollection("push_subscriptions")
		subs.Fields.Add(
			&core.RelationField{Name: "user", CollectionId: users.Id, Required: true, MaxSelect: 1},
			&core.TextField{Name: "endpoint", Required: true},
			&core.TextField{Name: "p256dh", Required: true},
			&core.TextField{Name: "auth", Required: true},
		)
		subs.ListRule = strPtr("user = @request.auth.id")
		subs.ViewRule = strPtr("user = @request.auth.id")
		subs.CreateRule = strPtr("@request.auth.id != ''")
		subs.UpdateRule = strPtr("user = @request.auth.id")
		subs.DeleteRule = strPtr("user = @request.auth.id")

		return app.Save(subs)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("push_subscriptions")
		if err == nil {
			return app.Delete(col)
		}
		return nil
	})
}
