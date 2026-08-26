package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Locks the auto-generated record REST API for collections whose writes happen
// exclusively server-side (through app.Save in handlers/hooks). Before this,
// matches and match_messages had create/update/delete rules of "" (public in
// PocketBase), users allowed self-update of the role field plus open
// registration, and notifications allowed public create — so any client, even
// unauthenticated, could rewrite match results, self-escalate to admin, create
// admin accounts, or delete matches via /api/collections/*. A nil rule is
// superuser-only; server-side app.Save bypasses rules, so the in-app flows are
// unaffected.
func init() {
	m.Register(func(app core.App) error {
		lockWrites := func(name string) error {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			c.CreateRule = nil
			c.UpdateRule = nil
			c.DeleteRule = nil
			return app.Save(c)
		}
		for _, name := range []string{"matches", "match_messages", "users"} {
			if err := lockWrites(name); err != nil {
				return err
			}
		}
		notifications, err := app.FindCollectionByNameOrId("notifications")
		if err != nil {
			return err
		}
		notifications.CreateRule = nil
		return app.Save(notifications)
	}, func(app core.App) error {
		restore := func(name string, create, update, del *string) error {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			c.CreateRule = create
			c.UpdateRule = update
			c.DeleteRule = del
			return app.Save(c)
		}
		public := ""
		self := "id = @request.auth.id"
		if err := restore("matches", &public, &public, &public); err != nil {
			return err
		}
		if err := restore("match_messages", &public, &public, &public); err != nil {
			return err
		}
		if err := restore("users", &public, &self, &self); err != nil {
			return err
		}
		notifications, err := app.FindCollectionByNameOrId("notifications")
		if err != nil {
			return err
		}
		notifications.CreateRule = &public
		return app.Save(notifications)
	})
}
