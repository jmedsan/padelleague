package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Drops the walkover score setting from competitions and app_settings. An approved walkover always
// records league.WalkoverScore, so the stored value was never read.
func init() {
	m.Register(func(app core.App) error {
		return removeWalkoverScoreFields(app)
	}, func(app core.App) error {
		return addWalkoverScoreFields(app)
	})
}

func removeWalkoverScoreFields(app core.App) error {
	for _, name := range []string{"competitions", "app_settings"} {
		col, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			return err
		}
		col.Fields.RemoveByName("walkover_score")
		if err := app.Save(col); err != nil {
			return err
		}
	}
	return nil
}

func addWalkoverScoreFields(app core.App) error {
	for _, name := range []string{"competitions", "app_settings"} {
		col, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			return err
		}
		col.Fields.Add(&core.TextField{Name: "walkover_score"})
		if err := app.Save(col); err != nil {
			return err
		}
	}
	return nil
}
