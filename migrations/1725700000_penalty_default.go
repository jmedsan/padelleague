package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		comps, err := app.FindRecordsByFilter("competitions", "default_penalty = 0", "", 0, 0, nil)
		if err != nil {
			return err
		}
		for _, comp := range comps {
			comp.Set("default_penalty", 3)
			if err := app.Save(comp); err != nil {
				return err
			}
		}
		return nil
	}, nil)
}
