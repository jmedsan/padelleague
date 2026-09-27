package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds matches.arbitration/arbitration_by: a participant's open request for
// admin review, generalizing the existing walkover report (review_type). An
// open request is arbitration != "". The free-text reason reuses the
// existing dispute_notes field.
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
			&core.SelectField{Name: "arbitration", Values: []string{"result", "scheduling", "abandonment", "no_show", "other"}, MaxSelect: 1},
			&core.RelationField{Name: "arbitration_by", CollectionId: users.Id, MaxSelect: 1},
		)
		return app.Save(matches)
	}, func(app core.App) error {
		matches, err := app.FindCollectionByNameOrId("matches")
		if err != nil {
			return err
		}
		matches.Fields.RemoveByName("arbitration")
		matches.Fields.RemoveByName("arbitration_by")
		return app.Save(matches)
	})
}
