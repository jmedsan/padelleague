package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Drops matches.rematch: a pairing is never repeated, so no match is a rematch.
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
