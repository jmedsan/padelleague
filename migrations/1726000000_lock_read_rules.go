package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		for _, name := range []string{"invitations", "match_messages"} {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			c.ListRule = nil
			c.ViewRule = nil
			if err := app.Save(c); err != nil {
				return err
			}
		}
		return nil
	}, func(app core.App) error {
		authed := "@request.auth.id != ''"
		for _, name := range []string{"invitations", "match_messages"} {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			c.ListRule = &authed
			c.ViewRule = &authed
			if err := app.Save(c); err != nil {
				return err
			}
		}
		return nil
	})
}
