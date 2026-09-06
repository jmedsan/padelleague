package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// 1. Users: add phone, admin_note, registration_note fields
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		users.Fields.Add(
			&core.TextField{Name: "phone", Max: 20},
			&core.TextField{Name: "admin_note", Max: 500},
			&core.TextField{Name: "registration_note", Max: 500},
		)
		// Tighten API rules — server-rendered UI uses app.Find* which
		// bypasses rules; no player feature uses the REST API.
		users.ListRule = ptrStr("id = @request.auth.id")
		users.ViewRule = ptrStr("id = @request.auth.id")
		if err := app.Save(users); err != nil {
			return err
		}

		// 2. Invitations: add admin_note, make competition optional,
		//    change used_by to multi-relation
		invitations, err := app.FindCollectionByNameOrId("invitations")
		if err != nil {
			return err
		}
		invitations.Fields.Add(
			&core.TextField{Name: "admin_note", Max: 500},
		)
		// Make competition optional by re-adding without Required
		competitions, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return err
		}
		invitations.Fields.Add(
			&core.RelationField{
				Name:         "competition",
				CollectionId: competitions.Id,
				MaxSelect:    1,
			},
		)
		// Change used_by to multi-relation (MaxSelect 0 = unlimited)
		invitations.Fields.Add(
			&core.RelationField{
				Name:         "used_by",
				CollectionId: users.Id,
				MaxSelect:    0,
			},
		)
		// Add use_count field if not already present (added in a previous
		// migration via NumberField, keep it consistent)
		if err := app.Save(invitations); err != nil {
			return err
		}

		// 3. New collection: competition_signups
		signups := core.NewBaseCollection("competition_signups")
		signups.Fields.Add(
			&core.RelationField{
				Name:         "competition",
				CollectionId: competitions.Id,
				Required:     true,
				MaxSelect:    1,
			},
			&core.RelationField{
				Name:         "user",
				CollectionId: users.Id,
				Required:     true,
				MaxSelect:    1,
			},
			&core.SelectField{
				Name:      "status",
				Values:    []string{"pending", "paired", "declined"},
				MaxSelect: 1,
				Required:  true,
			},
			&core.SelectField{
				Name:      "source",
				Values:    []string{"invite", "admin", "self"},
				MaxSelect: 1,
				Required:  true,
			},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		signups.ListRule = ptrStr(`@request.auth.id != ""`)
		signups.ViewRule = ptrStr(`@request.auth.id != ""`)

		return app.Save(signups)
	}, func(app core.App) error {
		// Down: remove competition_signups, revert user/invitation changes
		if col, err := app.FindCollectionByNameOrId("competition_signups"); err == nil {
			if err := app.Delete(col); err != nil {
				return err
			}
		}

		if users, err := app.FindCollectionByNameOrId("users"); err == nil {
			users.Fields.RemoveByName("phone")
			users.Fields.RemoveByName("admin_note")
			users.Fields.RemoveByName("registration_note")
			users.ListRule = ptrStr(`@request.auth.id != ""`)
			users.ViewRule = ptrStr(`@request.auth.id != ""`)
			_ = app.Save(users)
		}

		if inv, err := app.FindCollectionByNameOrId("invitations"); err == nil {
			inv.Fields.RemoveByName("admin_note")
			// Revert used_by to single relation
			if users, err := app.FindCollectionByNameOrId("users"); err == nil {
				inv.Fields.Add(&core.RelationField{
					Name:         "used_by",
					CollectionId: users.Id,
					MaxSelect:    1,
				})
			}
			_ = app.Save(inv)
		}

		return nil
	})
}

func ptrStr(s string) *string { return &s }
