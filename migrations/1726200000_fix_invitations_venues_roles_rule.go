package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Locks API writes (create/update/delete) to superuser-only for pairs,
// competitions, invitations, and venues. All writes to these collections
// go through server-side app.Save in admin handlers, which bypasses
// collection rules. The prior rules referenced a deleted field (role)
// or used the wrong operator (?=), making them accidentally fail-closed;
// this migration makes the intent explicit.
func init() {
	m.Register(func(app core.App) error {
		for _, name := range []string{"pairs", "competitions", "invitations", "venues"} {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return fmt.Errorf("find %s: %w", name, err)
			}
			c.CreateRule = nil
			c.UpdateRule = nil
			c.DeleteRule = nil
			if err := app.Save(c); err != nil {
				return fmt.Errorf("lock %s writes: %w", name, err)
			}
		}
		return nil
	}, func(app core.App) error {
		rolesRule := `@request.auth.roles ?= 'admin'`
		roleRule := `@request.auth.role = 'admin'`
		restoreRules := map[string]string{
			"pairs":        rolesRule,
			"competitions": rolesRule,
			"invitations":  roleRule,
			"venues":       roleRule,
		}
		for name, rule := range restoreRules {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return fmt.Errorf("find %s: %w", name, err)
			}
			r := rule
			c.CreateRule = &r
			c.UpdateRule = &r
			c.DeleteRule = &r
			if err := app.Save(c); err != nil {
				return fmt.Errorf("restore %s rules: %w", name, err)
			}
		}
		return nil
	})
}
