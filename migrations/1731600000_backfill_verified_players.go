package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Registration has always been invite-only — the admin vets a player before
// sending the invite — but registerUser and PlayerPreCreate never called
// SetVerified, so every player registered before that fix shows the
// "email not verified" banner forever. Backfill them.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindRecordsByFilter("users", "verified = false && roles ~ 'player'", "", 0, 0, nil)
		if err != nil {
			return err
		}
		for _, u := range users {
			u.SetVerified(true)
			if err := app.SaveNoValidate(u); err != nil {
				return err
			}
		}
		return nil
	}, func(_ core.App) error {
		return nil
	})
}
