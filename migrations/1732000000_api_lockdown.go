package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// lockedDownCollections lists the collections whose REST list/view access is
// superadmin-only: the server-rendered UI reads them via app.Find* (which
// bypasses collection rules), so no player-facing flow needs the REST API.
func lockedDownCollections() []string {
	return []string{"competitions", "matches", "pairs", "sponsors", "venues"}
}

func init() {
	m.Register(func(app core.App) error {
		for _, name := range lockedDownCollections() {
			col, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			col.ListRule = nil
			col.ViewRule = nil
			if err := app.Save(col); err != nil {
				return err
			}
		}

		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		users.AuthToken.Duration = 172800 // 2 days, down from the 5-day default
		return app.Save(users)
	}, func(app core.App) error {
		for _, name := range lockedDownCollections() {
			col, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				continue
			}
			col.ListRule = strPtr("@request.auth.id != ''")
			col.ViewRule = strPtr("@request.auth.id != ''")
			_ = app.Save(col)
		}

		if users, err := app.FindCollectionByNameOrId("users"); err == nil {
			users.AuthToken.Duration = 432000
			_ = app.Save(users)
		}
		return nil
	})
}
