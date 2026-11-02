package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds the promotions collection (sponsor offers shown to players). All API
// rules stay nil: pages go through handlers, and a write rule would let the
// view and click counters be forged.
func init() {
	m.Register(func(app core.App) error {
		zero := 0.0
		promotions := core.NewBaseCollection("promotions")
		promotions.Fields.Add(
			&core.TextField{Name: "title", Required: true, Max: 100},
			&core.TextField{Name: "description", Max: 300},
			&core.FileField{Name: "image", MaxSelect: 1, MaxSize: 5 << 20},
			&core.URLField{Name: "url", Required: true},
			&core.DateField{Name: "expires_on"},
			&core.BoolField{Name: "active"},
			&core.NumberField{Name: "views", OnlyInt: true, Min: &zero},
			&core.NumberField{Name: "clicks", OnlyInt: true, Min: &zero},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		return app.Save(promotions)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("promotions")
		if err != nil {
			return err
		}
		return app.Delete(col)
	})
}
