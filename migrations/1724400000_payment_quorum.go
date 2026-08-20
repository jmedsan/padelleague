package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		competitions, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return err
		}

		competitions.Fields.Add(
			&core.JSONField{Name: "payment_status"},
			&core.NumberField{Name: "quorum_timeout_hours"},
		)

		return app.Save(competitions)
	}, func(app core.App) error {
		competitions, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return nil
		}
		competitions.Fields.RemoveByName("payment_status")
		competitions.Fields.RemoveByName("quorum_timeout_hours")
		return app.Save(competitions)
	})
}
