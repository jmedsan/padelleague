package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return err
		}
		col.Fields.Add(&core.FileField{Name: "logo", MaxSelect: 1, MaxSize: 5 << 20})
		return app.Save(col)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return err
		}
		col.Fields.RemoveByName("logo")
		return app.Save(col)
	})
}
