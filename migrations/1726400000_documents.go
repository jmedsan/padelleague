package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds the reference-document library: a global `documents` collection (a file
// or a link, plus default/mandatory flags), a `documents` relation on
// competitions (attached reference items), and `document_acks` recording that a
// player has read a competition's mandatory items. Reads are open to any
// authenticated user (players view attached docs); writes stay superuser-only
// because every write goes through an admin handler that bypasses API rules.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		competitions, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return err
		}

		documents := core.NewBaseCollection("documents")
		documents.Fields.Add(
			&core.TextField{Name: "title", Required: true, Max: 200},
			&core.TextField{Name: "description", Max: 2000},
			&core.FileField{Name: "file", MaxSelect: 1, MaxSize: 20 << 20},
			&core.URLField{Name: "url"},
			&core.BoolField{Name: "is_default"},
			&core.BoolField{Name: "is_mandatory"},
			&core.RelationField{Name: "created_by", CollectionId: users.Id, MaxSelect: 1},
		)
		documents.ListRule = strPtr("@request.auth.id != ''")
		documents.ViewRule = strPtr("@request.auth.id != ''")
		if err := app.Save(documents); err != nil {
			return err
		}

		competitions.Fields.Add(
			&core.RelationField{Name: "documents", CollectionId: documents.Id, MaxSelect: 100},
		)
		if err := app.Save(competitions); err != nil {
			return err
		}

		acks := core.NewBaseCollection("document_acks")
		acks.Fields.Add(
			&core.RelationField{Name: "user", CollectionId: users.Id, Required: true, MaxSelect: 1, CascadeDelete: true},
			&core.RelationField{Name: "competition", CollectionId: competitions.Id, Required: true, MaxSelect: 1, CascadeDelete: true},
			&core.RelationField{Name: "documents", CollectionId: documents.Id, MaxSelect: 100},
		)
		acks.ListRule = strPtr("@request.auth.id != ''")
		acks.ViewRule = strPtr("@request.auth.id != ''")
		if err := app.Save(acks); err != nil {
			return err
		}

		return nil
	}, func(app core.App) error {
		for _, name := range []string{"document_acks", "documents"} {
			if c, err := app.FindCollectionByNameOrId(name); err == nil {
				if err := app.Delete(c); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
