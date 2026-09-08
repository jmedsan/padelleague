package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds two JSON fields parallel to payment_status (map[pairID]bool),
// recording when and by whom each pair was last marked paid:
// payment_paid_at (map[pairID]RFC3339 string), payment_paid_by
// (map[pairID]userID string). Kept separate from payment_status rather than
// reshaping it into a struct map, since that map is read in several
// unrelated packages.
func init() {
	m.Register(func(app core.App) error {
		competitions, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return err
		}
		competitions.Fields.Add(
			&core.JSONField{Name: "payment_paid_at"},
			&core.JSONField{Name: "payment_paid_by"},
		)
		return app.Save(competitions)
	}, func(app core.App) error {
		competitions, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return nil
		}
		competitions.Fields.RemoveByName("payment_paid_at")
		competitions.Fields.RemoveByName("payment_paid_by")
		return app.Save(competitions)
	})
}
