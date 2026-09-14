package migrations

import (
	"log/slog"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		pairs, err := app.FindRecordsByFilter("pairs", "captain = ''", "", 0, 0, nil)
		if err != nil {
			return err
		}
		for _, p := range pairs {
			p.Set("captain", p.GetString("player1"))
			if err := app.Save(p); err != nil {
				slog.Error("backfill captain", "pair", p.Id, "err", err)
			}
		}
		return nil
	}, func(app core.App) error {
		return nil
	})
}
