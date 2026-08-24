package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("match_messages")
		if err != nil {
			return err
		}

		col.Fields.Add(
			&core.SelectField{
				Name:      "type",
				Values:    []string{"chat", "scheduling_proposal", "score_discussion", "admin_action"},
				MaxSelect: 1,
				Required:  true,
			},
		)

		return app.Save(col)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("match_messages")
		if err != nil {
			return nil
		}

		col.Fields.Add(
			&core.SelectField{
				Name:      "type",
				Values:    []string{"chat", "scheduling_proposal", "score_discussion"},
				MaxSelect: 1,
				Required:  true,
			},
		)

		return app.Save(col)
	})
}
