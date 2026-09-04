package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		sponsors := core.NewBaseCollection("sponsors")
		sponsors.Fields.Add(
			&core.TextField{Name: "name", Required: true, Max: 100},
			&core.FileField{Name: "logo", Required: true, MaxSelect: 1, MaxSize: 5 << 20},
			&core.URLField{Name: "url"},
		)
		sponsors.ListRule = strPtr("@request.auth.id != ''")
		sponsors.ViewRule = strPtr("@request.auth.id != ''")
		if err := app.Save(sponsors); err != nil {
			return err
		}

		competitions, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return err
		}
		competitions.Fields.Add(
			&core.RelationField{Name: "sponsors", CollectionId: sponsors.Id, MaxSelect: 20},
		)
		return app.Save(competitions)
	}, func(app core.App) error {
		competitions, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return err
		}
		competitions.Fields.RemoveByName("sponsors")
		if err := app.Save(competitions); err != nil {
			return err
		}

		col, err := app.FindCollectionByNameOrId("sponsors")
		if err != nil {
			return err
		}
		return app.Delete(col)
	})
}
