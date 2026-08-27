package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds disputed_scores to matches: the score the disputing pair claims is
// correct. Together with the submitter's `scores` and `dispute_notes`, this
// gives the admin both claimed results and the reason, so a dispute can be
// resolved on evidence instead of guesswork.
func init() {
	m.Register(func(app core.App) error {
		matches, err := app.FindCollectionByNameOrId("matches")
		if err != nil {
			return err
		}
		matches.Fields.Add(&core.TextField{Name: "disputed_scores"})
		return app.Save(matches)
	}, func(app core.App) error {
		matches, err := app.FindCollectionByNameOrId("matches")
		if err != nil {
			return nil
		}
		matches.Fields.RemoveByName("disputed_scores")
		return app.Save(matches)
	})
}
