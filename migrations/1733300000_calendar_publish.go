package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds the draft/publish calendar workflow: competitions.calendar_status
// gates when players can see a generated fixture list, and two new
// competition_events kinds record the generate/publish actions on the
// admin activity timeline. Competitions that already have matches are
// backfilled to "published" since their calendar is already visible today.
func init() {
	m.Register(func(app core.App) error {
		comps, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return err
		}
		comps.Fields.Add(&core.SelectField{
			Name:      "calendar_status",
			Values:    []string{"none", "draft", "published"},
			MaxSelect: 1,
		})
		if err := app.Save(comps); err != nil {
			return err
		}

		notifs, err := app.FindCollectionByNameOrId("notifications")
		if err != nil {
			return err
		}
		notifType := notifs.Fields.GetByName("type").(*core.SelectField)
		notifType.Values = append(notifType.Values, "calendar_published")
		if err := app.Save(notifs); err != nil {
			return err
		}

		events, err := app.FindCollectionByNameOrId("competition_events")
		if err != nil {
			return err
		}
		eventKind := events.Fields.GetByName("kind").(*core.SelectField)
		eventKind.Values = append(eventKind.Values, "fixtures_generated", "calendar_published")
		if err := app.Save(events); err != nil {
			return err
		}

		records, err := app.FindRecordsByFilter("competitions", "calendar_status = ''", "", 0, 0)
		if err != nil {
			return err
		}
		for _, r := range records {
			matches, err := app.FindRecordsByFilter("matches", "competition = {:cid}", "", 1, 0, map[string]any{"cid": r.Id})
			if err != nil {
				return err
			}
			status := "none"
			if len(matches) > 0 {
				status = "published"
			}
			r.Set("calendar_status", status)
			if err := app.Save(r); err != nil {
				return err
			}
		}

		return nil
	}, func(app core.App) error {
		events, err := app.FindCollectionByNameOrId("competition_events")
		if err != nil {
			return err
		}
		eventKind := events.Fields.GetByName("kind").(*core.SelectField)
		filtered := eventKind.Values[:0]
		for _, v := range eventKind.Values {
			if v != "fixtures_generated" && v != "calendar_published" {
				filtered = append(filtered, v)
			}
		}
		eventKind.Values = filtered
		if err := app.Save(events); err != nil {
			return err
		}

		notifs, err := app.FindCollectionByNameOrId("notifications")
		if err != nil {
			return err
		}
		notifType := notifs.Fields.GetByName("type").(*core.SelectField)
		filteredTypes := notifType.Values[:0]
		for _, v := range notifType.Values {
			if v != "calendar_published" {
				filteredTypes = append(filteredTypes, v)
			}
		}
		notifType.Values = filteredTypes
		if err := app.Save(notifs); err != nil {
			return err
		}

		comps, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return err
		}
		comps.Fields.RemoveByName("calendar_status")
		return app.Save(comps)
	})
}
