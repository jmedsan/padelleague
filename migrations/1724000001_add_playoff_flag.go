package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		jornadas, err := app.FindCollectionByNameOrId("jornadas")
		if err != nil {
			return err
		}

		jornadas.Fields.Add(&core.BoolField{Name: "is_playoff"})
		jornadas.Indexes = append(jornadas.Indexes,
			"CREATE INDEX idx_jornadas_temporada_playoff ON jornadas (temporada, is_playoff)",
		)
		if err := app.Save(jornadas); err != nil {
			return err
		}

		clasificacion, err := app.FindCollectionByNameOrId("clasificacion")
		if err != nil {
			return err
		}
		clasificacion.ViewQuery = `SELECT sub.id, sub.pareja, sub.temporada, sub.wins, sub.losses, sub.sets_won, sub.sets_lost, sub.games_won, sub.games_lost, (sub.wins * 3) AS points FROM (SELECT p.id AS id, p.id AS pareja, t.id AS temporada, COUNT(CASE WHEN m.winner = p.id THEN 1 END) AS wins, COUNT(CASE WHEN m.winner != p.id AND m.winner != '' THEN 1 END) AS losses, 0 AS sets_won, 0 AS sets_lost, 0 AS games_won, 0 AS games_lost FROM parejas p LEFT JOIN partidos m ON (m.pareja1 = p.id OR m.pareja2 = p.id) AND m.status = 'final' JOIN temporadas t ON p.temporada = t.id GROUP BY p.id, t.id) AS sub`
		if err := app.Save(clasificacion); err != nil {
			return err
		}

		return nil
	}, func(app core.App) error {
		clasificacion, err := app.FindCollectionByNameOrId("clasificacion")
		if err == nil {
			clasificacion.ViewQuery = `SELECT sub.id, sub.pareja, sub.temporada, sub.wins, sub.losses, sub.sets_won, sub.sets_lost, sub.games_won, sub.games_lost, (sub.wins * 3) AS points FROM (SELECT p.id AS id, p.id AS pareja, t.id AS temporada, COUNT(CASE WHEN m.winner = p.id THEN 1 END) AS wins, COUNT(CASE WHEN m.winner != p.id AND m.winner != '' THEN 1 END) AS losses, 0 AS sets_won, 0 AS sets_lost, 0 AS games_won, 0 AS games_lost FROM parejas p JOIN partidos m ON (m.pareja1 = p.id OR m.pareja2 = p.id) AND m.status = 'final' JOIN jornadas j ON m.jornada = j.id JOIN temporadas t ON j.temporada = t.id GROUP BY p.id, t.id) AS sub`
			if err := app.Save(clasificacion); err != nil {
				return err
			}
		}

		jornadas, err := app.FindCollectionByNameOrId("jornadas")
		if err == nil {
			jornadas.Fields.RemoveByName("is_playoff")
			if err := app.Save(jornadas); err != nil {
				return err
			}
		}

		return nil
	})
}
