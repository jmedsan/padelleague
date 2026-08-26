// Package migrations contains PocketBase schema migrations.
package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		settings := app.Settings()
		settings.Meta.AppName = "PadelLeague"
		if err := app.Save(settings); err != nil {
			return err
		}

		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			users = core.NewAuthCollection("users")
		}
		users.Fields.Add(
			&core.TextField{Name: "display_name", Required: true, Max: 100},
			&core.FileField{Name: "avatar", MaxSelect: 1, MaxSize: 5 << 20},
			&core.SelectField{Name: "role", Values: []string{"admin", "player"}, MaxSelect: 1, Required: true},
			&core.JSONField{Name: "notification_prefs"},
		)
		users.ListRule = strPtr("@request.auth.id != ''")
		users.ViewRule = strPtr("@request.auth.id != ''")
		if err := app.Save(users); err != nil {
			return err
		}

		pairs := core.NewBaseCollection("pairs")
		pairs.Fields.Add(
			&core.TextField{Name: "name", Required: true},
			&core.FileField{Name: "avatar", MaxSelect: 1, MaxSize: 5 << 20},
			&core.RelationField{Name: "player1", CollectionId: users.Id, Required: true, MaxSelect: 1},
			&core.RelationField{Name: "player2", CollectionId: users.Id, Required: true, MaxSelect: 1},
		)
		setAdminRules(pairs)
		if err := app.Save(pairs); err != nil {
			return err
		}

		competitions := core.NewBaseCollection("competitions")
		competitions.Fields.Add(
			&core.TextField{Name: "name", Required: true},
			&core.SelectField{Name: "type", Values: []string{"league", "playoff"}, MaxSelect: 1, Required: true},
			&core.TextField{Name: "category"},
			&core.BoolField{Name: "active"},
			&core.BoolField{Name: "play_twice"},
			&core.NumberField{Name: "rounds"},
			&core.RelationField{Name: "pairs", CollectionId: pairs.Id, MaxSelect: 100},
			&core.JSONField{Name: "seeding"},
		)
		setAdminRules(competitions)
		if err := app.Save(competitions); err != nil {
			return err
		}

		matches := core.NewBaseCollection("matches")
		matches.Fields.Add(
			&core.RelationField{Name: "competition", CollectionId: competitions.Id, Required: true, MaxSelect: 1, CascadeDelete: true},
			&core.NumberField{Name: "round_number", Required: true},
			&core.NumberField{Name: "matches_to_win"},
			&core.RelationField{Name: "pair1", CollectionId: pairs.Id, Required: false, MaxSelect: 1},
			&core.RelationField{Name: "pair2", CollectionId: pairs.Id, Required: false, MaxSelect: 1},
			&core.TextField{Name: "scores"},
			&core.RelationField{Name: "winner", CollectionId: pairs.Id, MaxSelect: 1},
			&core.SelectField{Name: "status", Values: []string{"pending", "confirmed", "disputed", "final"}, MaxSelect: 1, Required: true},
			&core.DateField{Name: "date"},
			&core.TextField{Name: "time"},
			&core.TextField{Name: "club"},
			&core.TextField{Name: "court_number"},
			&core.RelationField{Name: "submitted_by", CollectionId: users.Id, MaxSelect: 1},
			&core.RelationField{Name: "confirmed_by", CollectionId: users.Id, MaxSelect: 1},
			&core.AutodateField{Name: "submitted_at", OnCreate: true},
			&core.RelationField{Name: "disputed_by", CollectionId: users.Id, MaxSelect: 1},
			&core.TextField{Name: "dispute_notes"},
		)
		matches.ListRule = strPtr("@request.auth.id != ''")
		matches.ViewRule = strPtr("@request.auth.id != ''")
		matches.CreateRule = strPtr("")
		matches.UpdateRule = strPtr("")
		matches.DeleteRule = strPtr("")
		if err := app.Save(matches); err != nil {
			return err
		}

		notifications := core.NewBaseCollection("notifications")
		notifications.Fields.Add(
			&core.RelationField{Name: "user", CollectionId: users.Id, Required: true, MaxSelect: 1, CascadeDelete: true},
			&core.SelectField{
				Name:      "type",
				Values:    []string{"quorum_request", "dispute", "match_assigned", "admin_message", "general"},
				MaxSelect: 1,
				Required:  true,
			},
			&core.TextField{Name: "title", Required: true},
			&core.TextField{Name: "body"},
			&core.BoolField{Name: "read"},
			&core.RelationField{Name: "related_match", CollectionId: matches.Id, MaxSelect: 1},
		)
		notifications.ListRule = strPtr("user = @request.auth.id")
		notifications.ViewRule = strPtr("user = @request.auth.id")
		notifications.CreateRule = strPtr("")
		notifications.UpdateRule = strPtr("user = @request.auth.id")
		notifications.DeleteRule = strPtr("user = @request.auth.id")
		if err := app.Save(notifications); err != nil {
			return err
		}

		return nil
	}, func(app core.App) error {
		collections := []string{
			"notifications", "matches",
			"competitions", "pairs",
		}
		for _, name := range collections {
			col, err := app.FindCollectionByNameOrId(name)
			if err == nil {
				if err := app.Delete(col); err != nil {
					return err
				}
			}
		}
		users, err := app.FindCollectionByNameOrId("users")
		if err == nil {
			users.Fields.RemoveByName("display_name")
			users.Fields.RemoveByName("avatar")
			users.Fields.RemoveByName("role")
			users.Fields.RemoveByName("notification_prefs")
			if err := app.Save(users); err != nil {
				return err
			}
		}
		return nil
	})
}

func strPtr(s string) *string {
	return &s
}

func setAdminRules(c *core.Collection) {
	c.ListRule = strPtr("@request.auth.id != ''")
	c.ViewRule = strPtr("@request.auth.id != ''")
	c.CreateRule = strPtr("@request.auth.role = 'admin'")
	c.UpdateRule = strPtr("@request.auth.role = 'admin'")
	c.DeleteRule = strPtr("@request.auth.role = 'admin'")
}
