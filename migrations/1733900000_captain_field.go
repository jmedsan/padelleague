package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		pairs, err := app.FindCollectionByNameOrId("pairs")
		if err != nil {
			return err
		}
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		pairs.Fields.Add(&core.RelationField{
			Name:         "captain",
			CollectionId: users.Id,
			MaxSelect:    1,
		})
		return app.Save(pairs)
	}, func(app core.App) error {
		pairs, err := app.FindCollectionByNameOrId("pairs")
		if err != nil {
			return nil
		}
		pairs.Fields.RemoveByName("captain")
		return app.Save(pairs)
	})
}
