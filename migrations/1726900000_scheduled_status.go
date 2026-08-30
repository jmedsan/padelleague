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
		statusField := matches.Fields.GetByName("status").(*core.SelectField)
		statusField.Values = append(statusField.Values, "scheduled")
		return app.Save(matches)
	}, func(app core.App) error {
		matches, err := app.FindCollectionByNameOrId("matches")
		if err != nil {
			return err
		}
		statusField := matches.Fields.GetByName("status").(*core.SelectField)
		filtered := statusField.Values[:0]
		for _, v := range statusField.Values {
			if v != "scheduled" {
				filtered = append(filtered, v)
			}
		}
		statusField.Values = filtered
		return app.Save(matches)
	})
}
