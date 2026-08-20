package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// --- App branding ---
		settings := app.Settings()
		settings.Meta.AppName = "PadelLeague"
		if err := app.Save(settings); err != nil {
			return err
		}

		// --- users (extend built-in auth collection) ---
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			users = core.NewAuthCollection("users")
		}
		users.Fields.Add(
			&core.TextField{Name: "display_name", Required: true, Max: 100},
			&core.FileField{Name: "avatar", MaxSelect: 1, MaxSize: 5 << 20},
			&core.SelectField{Name: "role", Values: []string{"admin", "player"}, MaxSelect: 1, Required: true},
			&core.JSONField{Name: "notification_prefs"},
			&core.NumberField{Name: "elo"},
		)
		users.ListRule = strPtr("@request.auth.id != ''")
		users.ViewRule = strPtr("@request.auth.id != ''")
		if err := app.Save(users); err != nil {
			return err
		}

		// --- pairs ---
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

		// --- competitions ---
		competitions := core.NewBaseCollection("competitions")
		competitions.Fields.Add(
			&core.TextField{Name: "name", Required: true},
			&core.SelectField{Name: "type", Values: []string{"league", "playoff"}, MaxSelect: 1, Required: true},
			&core.TextField{Name: "category"},
			&core.BoolField{Name: "active"},
			&core.BoolField{Name: "play_twice"},
			&core.NumberField{Name: "rounds"},
		)
		setAdminRules(competitions)
		if err := app.Save(competitions); err != nil {
			return err
		}

		// --- competition_pairs ---
		competitionPairs := core.NewBaseCollection("competition_pairs")
		competitionPairs.Fields.Add(
			&core.RelationField{Name: "competition", CollectionId: competitions.Id, Required: true, MaxSelect: 1, CascadeDelete: true},
			&core.RelationField{Name: "pair", CollectionId: pairs.Id, Required: true, MaxSelect: 1, CascadeDelete: false},
			&core.NumberField{Name: "seed"},
		)
		competitionPairs.Indexes = append(competitionPairs.Indexes,
			"CREATE UNIQUE INDEX idx_competition_pair ON competition_pairs (competition, pair)")
		setAdminRules(competitionPairs)
		if err := app.Save(competitionPairs); err != nil {
			return err
		}

		// --- matchdays ---
		matchdays := core.NewBaseCollection("matchdays")
		matchdays.Fields.Add(
			&core.RelationField{Name: "competition", CollectionId: competitions.Id, Required: true, MaxSelect: 1, CascadeDelete: true},
			&core.NumberField{Name: "round_number", Required: true},
			&core.NumberField{Name: "matches_to_win"},
		)
		setAdminRules(matchdays)
		if err := app.Save(matchdays); err != nil {
			return err
		}

		// --- matches ---
		matches := core.NewBaseCollection("matches")
		matches.Fields.Add(
			&core.RelationField{Name: "matchday", CollectionId: matchdays.Id, Required: true, MaxSelect: 1, CascadeDelete: true},
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

		// --- elo_history ---
		eloHistory := core.NewBaseCollection("elo_history")
		eloHistory.Fields.Add(
			&core.RelationField{Name: "player", CollectionId: users.Id, Required: true, MaxSelect: 1},
			&core.NumberField{Name: "old_elo", Required: true},
			&core.NumberField{Name: "new_elo", Required: true},
			&core.NumberField{Name: "delta", Required: true},
			&core.RelationField{Name: "match", CollectionId: matches.Id, Required: true, MaxSelect: 1, CascadeDelete: false},
		)
		eloHistory.ListRule = strPtr("@request.auth.id != ''")
		eloHistory.ViewRule = strPtr("@request.auth.id != ''")
		eloHistory.CreateRule = strPtr("")
		eloHistory.UpdateRule = strPtr("")
		eloHistory.DeleteRule = strPtr("")
		if err := app.Save(eloHistory); err != nil {
			return err
		}

		// --- notifications ---
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
			"notifications", "elo_history", "matches", "matchdays",
			"competition_pairs", "competitions", "pairs",
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
			users.Fields.RemoveByName("elo")
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
