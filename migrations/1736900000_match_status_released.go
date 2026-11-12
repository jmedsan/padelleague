package migrations

import (
	"errors"
	"slices"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds the terminal match status "released": a leveled match the admin
// released keeps its record and thread but leaves every count and player view.
func init() {
	m.Register(addReleasedStatus, removeReleasedStatus)
}

func addReleasedStatus(app core.App) error {
	matches, err := app.FindCollectionByNameOrId("matches")
	if err != nil {
		return err
	}
	statusField := matches.Fields.GetByName("status").(*core.SelectField)
	if slices.Contains(statusField.Values, "released") {
		return nil
	}
	statusField.Values = append(statusField.Values, "released")
	return app.Save(matches)
}

// removeReleasedStatus refuses while a released match exists: dropping the
// value would leave records that no longer validate.
func removeReleasedStatus(app core.App) error {
	released, err := app.FindRecordsByFilter("matches", "status = 'released'", "", 1, 0)
	if err != nil {
		return err
	}
	if len(released) > 0 {
		return errors.New("cannot remove the released status while released matches exist")
	}
	matches, err := app.FindCollectionByNameOrId("matches")
	if err != nil {
		return err
	}
	statusField := matches.Fields.GetByName("status").(*core.SelectField)
	statusField.Values = slices.DeleteFunc(slices.Clone(statusField.Values), func(v string) bool { return v == "released" })
	return app.Save(matches)
}
