package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// hiddenUserNoteFields lists the admin-only users fields that must never
// leave the server through the REST API (same mechanism as "roles").
func hiddenUserNoteFields() []string {
	return []string{"admin_note", "registration_note"}
}

func init() {
	m.Register(func(app core.App) error {
		return setUserNoteFieldsHidden(app, true)
	}, func(app core.App) error {
		return setUserNoteFieldsHidden(app, false)
	})
}

func setUserNoteFieldsHidden(app core.App, hidden bool) error {
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	for _, name := range hiddenUserNoteFields() {
		users.Fields.GetByName(name).SetHidden(hidden)
	}
	return app.Save(users)
}
