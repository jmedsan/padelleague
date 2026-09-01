package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("invitations")
		if err != nil {
			return err
		}

		col.Fields.Add(&core.NumberField{Name: "max_uses", Min: ptrFloat(1)})

		return app.Save(col)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("invitations")
		if err != nil {
			return err
		}

		col.Fields.Add(&core.NumberField{Name: "max_uses"})

		return app.Save(col)
	})
}

func ptrFloat(v float64) *float64 { return &v }
