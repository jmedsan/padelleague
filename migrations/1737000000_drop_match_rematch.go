package migrations

import (
	"log/slog"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Drops matches.rematch: a pairing is never repeated, so no match is a rematch.
// An existing rematch not yet played is released first, as an admin "Liberar"
// would (status released + an authorless admin_action, shown as "Sistema");
// a final rematch was really played and is kept as a normal match. The status
// flip skips validation so an unrelated stale field cannot block startup.
func init() {
	m.Register(dropMatchRematch, restoreMatchRematch)
}

func dropMatchRematch(app core.App) error {
	matches, err := app.FindCollectionByNameOrId("matches")
	if err != nil {
		return err
	}
	if matches.Fields.GetByName("rematch") == nil {
		return nil
	}
	if err := releaseOpenRematches(app); err != nil {
		return err
	}
	matches.Fields.RemoveByName("rematch")
	return app.Save(matches)
}

func restoreMatchRematch(app core.App) error {
	matches, err := app.FindCollectionByNameOrId("matches")
	if err != nil {
		return err
	}
	if matches.Fields.GetByName("rematch") != nil {
		return nil
	}
	matches.Fields.Add(&core.BoolField{Name: "rematch"})
	return app.Save(matches)
}

func releaseOpenRematches(app core.App) error {
	open, err := app.FindRecordsByFilter("matches", "rematch = true && status != 'final' && status != 'released'", "", 0, 0)
	if err != nil {
		return err
	}
	msgs, err := app.FindCollectionByNameOrId("match_messages")
	if err != nil {
		return err
	}
	for _, m := range open {
		m.Set("status", "released")
		if err := app.SaveNoValidate(m); err != nil {
			return err
		}
		entry := core.NewRecord(msgs)
		entry.Set("match", m.Id)
		entry.Set("type", "admin_action")
		entry.Set("content", "liberó el partido")
		if err := app.Save(entry); err != nil {
			return err
		}
	}
	slog.Info("migration drop_match_rematch", "released_open_rematches", len(open))
	return nil
}
