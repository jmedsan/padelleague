package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		matches, err := app.FindCollectionByNameOrId("matches")
		if err != nil {
			return err
		}
		matches.Fields.Add(&core.TextField{
			Name: "carried_sets",
			Max:  20,
		})
		return app.Save(matches)
	}, func(app core.App) error {
		matches, err := app.FindCollectionByNameOrId("matches")
		if err != nil {
			return err
		}
		matches.Fields.RemoveByName("carried_sets")
		return app.Save(matches)
	})
}
