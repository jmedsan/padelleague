package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		invites, err := app.FindRecordsByFilter("invitations",
			"max_uses = 0", "", 0, 0, nil)
		if err != nil {
			return nil
		}
		for _, inv := range invites {
			inv.Set("max_uses", 1)
			if err := app.Save(inv); err != nil {
				return err
			}
		}
		return nil
	}, nil)
}
