package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

func init() {
	m.Register(func(app core.App) error {
		notifs, err := app.FindCollectionByNameOrId("notifications")
		if err != nil {
			return err
		}
		notifType := notifs.Fields.GetByName("type").(*core.SelectField)
		notifType.Values = append(notifType.Values, "match_reminder")
		if err := app.Save(notifs); err != nil {
			return err
		}

		comps, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return err
		}
		comps.Fields.Add(&core.JSONField{Name: "match_reminder_hours", MaxSize: 200})
		if err := app.Save(comps); err != nil {
			return err
		}

		settings, err := app.FindCollectionByNameOrId("app_settings")
		if err != nil {
			return err
		}
		settings.Fields.Add(&core.JSONField{Name: "match_reminder_hours", MaxSize: 200})
		if err := app.Save(settings); err != nil {
			return err
		}

		recs, err := app.FindRecordsByFilter("app_settings", "", "", 1, 0, nil)
		if err == nil && len(recs) > 0 {
			recs[0].Set("match_reminder_hours", []int{26, 1})
			if err := app.Save(recs[0]); err != nil {
				return err
			}
		}

		matches, err := app.FindCollectionByNameOrId("matches")
		if err != nil {
			return err
		}
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		col := core.NewBaseCollection("match_reminders")
		col.Fields.Add(
			&core.RelationField{Name: "match", CollectionId: matches.Id, Required: true, MaxSelect: 1, CascadeDelete: true},
			&core.RelationField{Name: "user", CollectionId: users.Id, Required: true, MaxSelect: 1, CascadeDelete: true},
			&core.NumberField{Name: "hours_before", Required: true, Min: floatPtr(1)},
			&core.AutodateField{Name: "created", OnCreate: true},
		)
		col.Indexes = types.JSONArray[string]{
			"CREATE UNIQUE INDEX idx_match_reminders_key ON match_reminders (match, user, hours_before)",
		}
		col.ListRule = nil
		col.ViewRule = nil
		col.CreateRule = nil
		col.UpdateRule = nil
		col.DeleteRule = nil
		return app.Save(col)
	}, func(app core.App) error {
		if notifs, err := app.FindCollectionByNameOrId("notifications"); err == nil {
			notifType := notifs.Fields.GetByName("type").(*core.SelectField)
			filtered := make([]string, 0, len(notifType.Values))
			for _, v := range notifType.Values {
				if v != "match_reminder" {
					filtered = append(filtered, v)
				}
			}
			notifType.Values = filtered
			if err := app.Save(notifs); err != nil {
				return err
			}
		}

		if c, err := app.FindCollectionByNameOrId("match_reminders"); err == nil {
			if err := app.Delete(c); err != nil {
				return err
			}
		}

		if comps, err := app.FindCollectionByNameOrId("competitions"); err == nil {
			comps.Fields.RemoveByName("match_reminder_hours")
			if err := app.Save(comps); err != nil {
				return err
			}
		}

		if settings, err := app.FindCollectionByNameOrId("app_settings"); err == nil {
			settings.Fields.RemoveByName("match_reminder_hours")
			if err := app.Save(settings); err != nil {
				return err
			}
		}

		return nil
	})
}
