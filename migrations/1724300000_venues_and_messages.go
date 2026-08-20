package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// --- venues ---
		venues := core.NewBaseCollection("venues")
		venues.Fields.Add(
			&core.TextField{Name: "name", Required: true, Max: 100},
			&core.TextField{Name: "address", Max: 200},
			&core.NumberField{Name: "courts"},
		)
		setAdminRules(venues)
		if err := app.Save(venues); err != nil {
			return err
		}

		seedVenues := []string{"Padel 360", "Wurko", "Tecnisur", "Padelcan", "Punta del Rey"}
		for _, name := range seedVenues {
			v := core.NewRecord(venues)
			v.Set("name", name)
			if err := app.Save(v); err != nil {
				return err
			}
		}

		// --- match_messages ---
		matches, err := app.FindCollectionByNameOrId("matches")
		if err != nil {
			return err
		}
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		matchMessages := core.NewBaseCollection("match_messages")
		matchMessages.Fields.Add(
			&core.RelationField{Name: "match", CollectionId: matches.Id, Required: true, MaxSelect: 1, CascadeDelete: true},
			&core.RelationField{Name: "author", CollectionId: users.Id, Required: true, MaxSelect: 1},
			&core.SelectField{Name: "type", Values: []string{"chat", "scheduling_proposal", "score_discussion"}, MaxSelect: 1, Required: true},
			&core.TextField{Name: "content"},
			&core.JSONField{Name: "proposal_data"},
			&core.SelectField{Name: "proposal_status", Values: []string{"pending", "accepted", "rejected", "superseded"}, MaxSelect: 1},
			&core.TextField{Name: "rejection_reason"},
			&core.TextField{Name: "rejection_text"},
			&core.AutodateField{Name: "created", OnCreate: true},
		)
		matchMessages.ListRule = strPtr("@request.auth.id != ''")
		matchMessages.ViewRule = strPtr("@request.auth.id != ''")
		matchMessages.CreateRule = strPtr("")
		matchMessages.UpdateRule = strPtr("")
		matchMessages.DeleteRule = strPtr("")
		if err := app.Save(matchMessages); err != nil {
			return err
		}

		// --- add "scheduling" to notifications type field ---
		notifications, err := app.FindCollectionByNameOrId("notifications")
		if err != nil {
			return err
		}
		notifications.Fields.Add(
			&core.SelectField{
				Name:      "type",
				Values:    []string{"quorum_request", "dispute", "match_assigned", "admin_message", "general", "scheduling"},
				MaxSelect: 1,
				Required:  true,
			},
		)
		if err := app.Save(notifications); err != nil {
			return err
		}

		return nil
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("match_messages")
		if err == nil {
			app.Delete(col)
		}
		col, err = app.FindCollectionByNameOrId("venues")
		if err == nil {
			app.Delete(col)
		}
		return nil
	})
}
