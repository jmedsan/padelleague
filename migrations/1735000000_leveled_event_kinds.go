package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds "assignment_generated" and "assignment_released" to the
// competition_events.kind select field for leveled-league events.
func init() {
	m.Register(func(app core.App) error {
		events, err := app.FindCollectionByNameOrId("competition_events")
		if err != nil {
			return err
		}
		kind := events.Fields.GetByName("kind").(*core.SelectField)
		kind.Values = append(kind.Values, "assignment_generated", "assignment_released")
		return app.Save(events)
	}, func(app core.App) error {
		events, err := app.FindCollectionByNameOrId("competition_events")
		if err != nil {
			return err
		}
		kind := events.Fields.GetByName("kind").(*core.SelectField)
		filtered := kind.Values[:0]
		for _, v := range kind.Values {
			if v != "assignment_generated" && v != "assignment_released" {
				filtered = append(filtered, v)
			}
		}
		kind.Values = filtered
		return app.Save(events)
	})
}
