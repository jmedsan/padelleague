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
		col.Fields.Add(&core.TextField{Name: "registration_note", Max: 500})
		return app.Save(col)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("invitations")
		if err != nil {
			return err
		}
		col.Fields.RemoveByName("registration_note")
		return app.Save(col)
	})
}
