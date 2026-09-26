package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		comps, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return err
		}
		comps.Fields.Add(&core.JSONField{Name: "released_pairings", MaxSize: 20000})
		return app.Save(comps)
	}, func(app core.App) error {
		comps, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return err
		}
		comps.Fields.RemoveByName("released_pairings")
		return app.Save(comps)
	})
}
