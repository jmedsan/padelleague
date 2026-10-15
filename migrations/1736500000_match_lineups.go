package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds matches.lineup1 and matches.lineup2, the players of each pair who
// played the match, frozen when a result is first proposed or a walkover is
// recorded. A player's own stats read them, so a pair swap does not move
// played matches to the replacement. Matches that already have a result are
// backfilled from the current membership.
func init() {
	m.Register(func(app core.App) error {
		matches, err := app.FindCollectionByNameOrId("matches")
		if err != nil {
			return err
		}
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		matches.Fields.Add(
			&core.RelationField{Name: "lineup1", CollectionId: users.Id, MaxSelect: 2},
			&core.RelationField{Name: "lineup2", CollectionId: users.Id, MaxSelect: 2},
		)
		if err := app.Save(matches); err != nil {
			return err
		}
		return backfillLineups(app)
	}, func(app core.App) error {
		matches, err := app.FindCollectionByNameOrId("matches")
		if err != nil {
			return err
		}
		matches.Fields.RemoveByName("lineup1")
		matches.Fields.RemoveByName("lineup2")
		return app.Save(matches)
	})
}

// backfillLineups freezes the current players of both pairs on every match
// that has a result: a submission, a final status, a walkover, or a pending
// result proposal in the thread.
func backfillLineups(app core.App) error {
	byResult, err := app.FindRecordsByFilter("matches",
		"submitted_by != '' || status = 'final' || review_type = 'walkover'", "", 0, 0)
	if err != nil {
		return err
	}
	proposals, err := app.FindRecordsByFilter("match_messages", "type = 'result_submission'", "", 0, 0)
	if err != nil {
		return err
	}
	seen := make(map[string]bool, len(byResult))
	matches := byResult
	for _, m := range byResult {
		seen[m.Id] = true
	}
	for _, p := range proposals {
		id := p.GetString("match")
		if seen[id] {
			continue
		}
		seen[id] = true
		if match, err := app.FindRecordById("matches", id); err == nil {
			matches = append(matches, match)
		}
	}
	for _, match := range matches {
		match.Set("lineup1", pairPlayerIDs(app, match.GetString("pair1")))
		match.Set("lineup2", pairPlayerIDs(app, match.GetString("pair2")))
		if err := app.SaveNoValidate(match); err != nil {
			return err
		}
	}
	return nil
}

func pairPlayerIDs(app core.App, pairID string) []string {
	pair, err := app.FindRecordById("pairs", pairID)
	if err != nil {
		return nil
	}
	var ids []string
	for _, f := range []string{"player1", "player2"} {
		if id := pair.GetString(f); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}
