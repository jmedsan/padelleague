package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		notifs, err := app.FindCollectionByNameOrId("notifications")
		if err != nil {
			return err
		}
		typeField := notifs.Fields.GetByName("type").(*core.SelectField)
		typeField.Values = append(typeField.Values, "match_progress")
		return app.Save(notifs)
	}, func(app core.App) error {
		notifs, err := app.FindCollectionByNameOrId("notifications")
		if err != nil {
			return err
		}
		typeField := notifs.Fields.GetByName("type").(*core.SelectField)
		filtered := typeField.Values[:0]
		for _, v := range typeField.Values {
			if v != "match_progress" {
				filtered = append(filtered, v)
			}
		}
		typeField.Values = filtered
		return app.Save(notifs)
	})
}
