package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds matches.submitted_by_pair and matches.disputed_by_pair, the pair the
// result submitter and the disputer belonged to when they acted, so the result
// panel, the correction window and the reminders keep naming the same pair
// after a pair's players change. Existing matches are backfilled from the
// current membership. It also drops author_pair from admin_action messages:
// an admin acting as admin has no pair, even when the admin also plays.
func init() {
	m.Register(func(app core.App) error {
		matches, err := app.FindCollectionByNameOrId("matches")
		if err != nil {
			return err
		}
		pairs, err := app.FindCollectionByNameOrId("pairs")
		if err != nil {
			return err
		}
		matches.Fields.Add(
			&core.RelationField{Name: "submitted_by_pair", CollectionId: pairs.Id, MaxSelect: 1},
			&core.RelationField{Name: "disputed_by_pair", CollectionId: pairs.Id, MaxSelect: 1},
		)
		if err := app.Save(matches); err != nil {
			return err
		}
		if err := backfillActorPairs(app); err != nil {
			return err
		}
		return clearAdminActionPairs(app)
	}, func(app core.App) error {
		matches, err := app.FindCollectionByNameOrId("matches")
		if err != nil {
			return err
		}
		matches.Fields.RemoveByName("submitted_by_pair")
		matches.Fields.RemoveByName("disputed_by_pair")
		return app.Save(matches)
	})
}

func backfillActorPairs(app core.App) error {
	matches, err := app.FindRecordsByFilter("matches", "submitted_by != '' || disputed_by != ''", "", 0, 0)
	if err != nil {
		return err
	}
	for _, match := range matches {
		match.Set("submitted_by_pair", authorPairID(app, match.GetString("submitted_by"), match))
		match.Set("disputed_by_pair", authorPairID(app, match.GetString("disputed_by"), match))
		if err := app.SaveNoValidate(match); err != nil {
			return err
		}
	}
	return nil
}

func clearAdminActionPairs(app core.App) error {
	msgs, err := app.FindRecordsByFilter("match_messages", "type = 'admin_action' && author_pair != ''", "", 0, 0)
	if err != nil {
		return err
	}
	for _, msg := range msgs {
		msg.Set("author_pair", "")
		if err := app.SaveNoValidate(msg); err != nil {
			return err
		}
	}
	return nil
}
