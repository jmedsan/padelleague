package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		parejas, err := app.FindCollectionByNameOrId("parejas")
		if err != nil {
			return err
		}

		parejas.Fields.Add(&core.NumberField{
			Name:     "elo",
			Required: false,
		})
		if err := app.Save(parejas); err != nil {
			return err
		}

		// Set default ELO for existing pairs
		existingPairs, _ := app.FindAllRecords("parejas")
		for _, p := range existingPairs {
			if p.GetFloat("elo") == 0 {
				p.Set("elo", 1500)
				app.Save(p)
			}
		}

		eloHistory := core.NewBaseCollection("elo_history")
		eloHistory.Fields.Add(
			&core.RelationField{
				Name:          "pareja",
				CollectionId:  parejas.Id,
				Required:      true,
				MaxSelect:     1,
				CascadeDelete: false,
			},
			&core.NumberField{
				Name:     "old_elo",
				Required: true,
			},
			&core.NumberField{
				Name:     "new_elo",
				Required: true,
			},
			&core.NumberField{
				Name:     "delta",
				Required: true,
			},
		)

		partidosCol, err := app.FindCollectionByNameOrId("partidos")
		if err != nil {
			return err
		}
		eloHistory.Fields.Add(&core.RelationField{
			Name:          "partido",
			CollectionId:  partidosCol.Id,
			Required:      true,
			MaxSelect:     1,
			CascadeDelete: false,
		})

		eloHistory.ListRule = strPtr("@request.auth.id != ''")
		eloHistory.ViewRule = strPtr("@request.auth.id != ''")
		eloHistory.CreateRule = strPtr("")
		eloHistory.UpdateRule = strPtr("")
		eloHistory.DeleteRule = strPtr("")

		if err := app.Save(eloHistory); err != nil {
			return err
		}

		clasificacion, err := app.FindCollectionByNameOrId("clasificacion")
		if err == nil {
			if err := app.Delete(clasificacion); err != nil {
				return err
			}
		}

		return nil
	}, func(app core.App) error {
		eloHistory, err := app.FindCollectionByNameOrId("elo_history")
		if err == nil {
			app.Delete(eloHistory)
		}

		parejas, err := app.FindCollectionByNameOrId("parejas")
		if err == nil {
			parejas.Fields.RemoveByName("elo")
			app.Save(parejas)
		}

		return nil
	})
}
