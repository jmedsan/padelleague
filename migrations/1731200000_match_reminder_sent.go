package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		matches, err := app.FindCollectionByNameOrId("matches")
		if err != nil {
			return err
		}

		matches.Fields.Add(
			&core.BoolField{Name: "reminder_sent"},
		)

		return app.Save(matches)
	}, func(app core.App) error {
		matches, err := app.FindCollectionByNameOrId("matches")
		if err != nil {
			return nil
		}
		matches.Fields.RemoveByName("reminder_sent")
		return app.Save(matches)
	})
}
