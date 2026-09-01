package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		comps, err := app.FindRecordsByFilter("competitions", "id != ''", "", 0, 0, nil)
		if err != nil {
			return err
		}
		for _, c := range comps {
			cat := c.GetString("category")
			if cat != "" {
				name := c.GetString("name")
				c.Set("name", name+" — "+cat)
				if err := app.Save(c); err != nil {
					return err
				}
			}
		}

		col, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return err
		}
		col.Fields.RemoveByName("category")
		return app.Save(col)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return err
		}
		col.Fields.Add(&core.TextField{Name: "category"})
		return app.Save(col)
	})
}
