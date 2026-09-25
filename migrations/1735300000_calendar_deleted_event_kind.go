package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds "calendar_deleted" to the competition_events.kind select field, for
// the admin action that deletes a draft calendar's matches.
func init() {
	m.Register(func(app core.App) error {
		events, err := app.FindCollectionByNameOrId("competition_events")
		if err != nil {
			return err
		}
		kind := events.Fields.GetByName("kind").(*core.SelectField)
		kind.Values = append(kind.Values, "calendar_deleted")
		return app.Save(events)
	}, func(app core.App) error {
		events, err := app.FindCollectionByNameOrId("competition_events")
		if err != nil {
			return err
		}
		kind := events.Fields.GetByName("kind").(*core.SelectField)
		filtered := kind.Values[:0]
		for _, v := range kind.Values {
			if v != "calendar_deleted" {
				filtered = append(filtered, v)
			}
		}
		kind.Values = filtered
		return app.Save(events)
	})
}
