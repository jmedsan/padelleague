package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		msgs, err := app.FindCollectionByNameOrId("match_messages")
		if err != nil {
			return err
		}
		typeField := msgs.Fields.GetByName("type").(*core.SelectField)
		typeField.Values = append(typeField.Values, "availability")
		return app.Save(msgs)
	}, func(app core.App) error {
		msgs, err := app.FindCollectionByNameOrId("match_messages")
		if err != nil {
			return err
		}
		typeField := msgs.Fields.GetByName("type").(*core.SelectField)
		filtered := typeField.Values[:0]
		for _, v := range typeField.Values {
			if v != "availability" {
				filtered = append(filtered, v)
			}
		}
		typeField.Values = filtered
		return app.Save(msgs)
	})
}
