package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("match_messages")
		if err != nil {
			return err
		}
		f := col.Fields.GetByName("proposal_status")
		if sf, ok := f.(*core.SelectField); ok {
			sf.Values = []string{"pending", "accepted", "rejected", "superseded", "withdrawn"}
		}
		return app.Save(col)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("match_messages")
		if err != nil {
			return err
		}
		f := col.Fields.GetByName("proposal_status")
		if sf, ok := f.(*core.SelectField); ok {
			sf.Values = []string{"pending", "accepted", "rejected", "superseded"}
		}
		return app.Save(col)
	})
}
