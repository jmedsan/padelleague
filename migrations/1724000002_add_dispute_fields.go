package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		partidos, err := app.FindCollectionByNameOrId("partidos")
		if err != nil {
			return err
		}

		jugadores, err := app.FindCollectionByNameOrId("jugadores")
		if err != nil {
			return err
		}

		partidos.Fields.Add(
			&core.RelationField{Name: "disputed_by", CollectionId: jugadores.Id, MaxSelect: 1},
			&core.TextField{Name: "dispute_notes"},
		)

		partidos.UpdateRule = strPtr("")

		return app.Save(partidos)
	}, func(app core.App) error {
		partidos, err := app.FindCollectionByNameOrId("partidos")
		if err != nil {
			return err
		}

		partidos.Fields.RemoveByName("disputed_by")
		partidos.Fields.RemoveByName("dispute_notes")

		partidos.UpdateRule = strPtr(
			"@request.auth.id = pareja1.jugador1.user || " +
				"@request.auth.id = pareja1.jugador2.user || " +
				"@request.auth.id = pareja2.jugador1.user || " +
				"@request.auth.id = pareja2.jugador2.user",
		)

		return app.Save(partidos)
	})
}
