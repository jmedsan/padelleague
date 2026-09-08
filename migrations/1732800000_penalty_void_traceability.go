package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds who/when/why to a voided penalty, so the admin detail page can show
// "Anulado por {name} — {reason}" instead of a silent voided flag.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		penalties, err := app.FindCollectionByNameOrId("penalties")
		if err != nil {
			return err
		}
		penalties.Fields.Add(
			&core.RelationField{Name: "voided_by", CollectionId: users.Id, MaxSelect: 1},
			&core.DateField{Name: "voided_at"},
			&core.TextField{Name: "void_reason", Max: 500},
		)
		return app.Save(penalties)
	}, func(app core.App) error {
		penalties, err := app.FindCollectionByNameOrId("penalties")
		if err != nil {
			return err
		}
		penalties.Fields.RemoveByName("voided_by")
		penalties.Fields.RemoveByName("voided_at")
		penalties.Fields.RemoveByName("void_reason")
		return app.Save(penalties)
	})
}
