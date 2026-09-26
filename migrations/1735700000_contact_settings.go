package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds app_settings.contact_whatsapp/contact_email: the global "contact the
// admin" numbers, both optional and empty by default.
func init() {
	m.Register(func(app core.App) error {
		settings, err := app.FindCollectionByNameOrId("app_settings")
		if err != nil {
			return err
		}
		settings.Fields.Add(
			&core.TextField{Name: "contact_whatsapp", Max: 20},
			&core.TextField{Name: "contact_email", Max: 255},
		)
		return app.Save(settings)
	}, func(app core.App) error {
		settings, err := app.FindCollectionByNameOrId("app_settings")
		if err != nil {
			return err
		}
		settings.Fields.RemoveByName("contact_whatsapp")
		settings.Fields.RemoveByName("contact_email")
		return app.Save(settings)
	})
}
