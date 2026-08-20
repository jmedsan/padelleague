package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// --- users (extend built-in auth collection) ---
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			users = core.NewAuthCollection("users")
		}
		users.Fields.Add(
			&core.TextField{Name: "display_name", Required: true, Max: 100},
			&core.FileField{Name: "avatar", MaxSelect: 1, MaxSize: 5 << 20},
			&core.SelectField{Name: "role", Values: []string{"admin", "user"}, MaxSelect: 1, Required: true},
		)
		users.ListRule = strPtr("@request.auth.id != ''")
		users.ViewRule = strPtr("@request.auth.id != ''")
		if err := app.Save(users); err != nil {
			return err
		}

		// --- categorias ---
		categorias := core.NewBaseCollection("categorias")
		categorias.Fields.Add(
			&core.TextField{Name: "name", Required: true},
			&core.TextField{Name: "description"},
			&core.SelectField{Name: "sport_type", Values: []string{"padel", "tennis"}, MaxSelect: 1},
			&core.FileField{Name: "logo", MaxSelect: 1, MaxSize: 5 << 20},
		)
		categorias.Indexes = append(categorias.Indexes, "CREATE UNIQUE INDEX idx_categorias_name ON categorias (name)")
		setAdminRules(categorias)
		if err := app.Save(categorias); err != nil {
			return err
		}

		// --- temporadas ---
		temporadas := core.NewBaseCollection("temporadas")
		temporadas.Fields.Add(
			&core.TextField{Name: "name", Required: true},
			&core.RelationField{Name: "categoria", CollectionId: categorias.Id, Required: true, MaxSelect: 1},
			&core.DateField{Name: "start_date", Required: true},
			&core.DateField{Name: "end_date", Required: true},
			&core.BoolField{Name: "active"},
			&core.BoolField{Name: "play_twice"},
		)
		setAdminRules(temporadas)
		if err := app.Save(temporadas); err != nil {
			return err
		}

		// --- jugadores ---
		jugadores := core.NewBaseCollection("jugadores")
		jugadores.Fields.Add(
			&core.RelationField{Name: "user", CollectionId: users.Id, Required: true, MaxSelect: 1},
			&core.RelationField{Name: "categorias", CollectionId: categorias.Id, MaxSelect: 100},
		)
		jugadores.Indexes = append(jugadores.Indexes, "CREATE UNIQUE INDEX idx_jugadores_user ON jugadores (user)")
		setAdminRules(jugadores)
		if err := app.Save(jugadores); err != nil {
			return err
		}

		// --- parejas ---
		parejas := core.NewBaseCollection("parejas")
		parejas.Fields.Add(
			&core.RelationField{Name: "jugador1", CollectionId: jugadores.Id, Required: true, MaxSelect: 1},
			&core.RelationField{Name: "jugador2", CollectionId: jugadores.Id, Required: true, MaxSelect: 1},
			&core.RelationField{Name: "categoria", CollectionId: categorias.Id, Required: true, MaxSelect: 1},
			&core.RelationField{Name: "temporada", CollectionId: temporadas.Id, Required: true, MaxSelect: 1},
		)
		setAdminRules(parejas)
		if err := app.Save(parejas); err != nil {
			return err
		}

		// --- jornadas ---
		jornadas := core.NewBaseCollection("jornadas")
		jornadas.Fields.Add(
			&core.RelationField{Name: "temporada", CollectionId: temporadas.Id, Required: true, MaxSelect: 1},
			&core.NumberField{Name: "round_number", Required: true},
			&core.DateField{Name: "date"},
		)
		setAdminRules(jornadas)
		if err := app.Save(jornadas); err != nil {
			return err
		}

		// --- partidos ---
		partidos := core.NewBaseCollection("partidos")
		partidos.Fields.Add(
			&core.RelationField{Name: "jornada", CollectionId: jornadas.Id, Required: true, MaxSelect: 1},
			&core.RelationField{Name: "pareja1", CollectionId: parejas.Id, Required: true, MaxSelect: 1},
			&core.RelationField{Name: "pareja2", CollectionId: parejas.Id, Required: true, MaxSelect: 1},
			&core.TextField{Name: "scores"},
			&core.RelationField{Name: "winner", CollectionId: parejas.Id, MaxSelect: 1},
			&core.SelectField{Name: "status", Values: []string{"pending", "confirmed", "disputed", "final"}, MaxSelect: 1, Required: true},
			&core.DateField{Name: "date"},
			&core.TextField{Name: "time"},
			&core.TextField{Name: "club"},
			&core.TextField{Name: "court_number"},
			&core.RelationField{Name: "submitted_by", CollectionId: jugadores.Id, MaxSelect: 1},
			&core.RelationField{Name: "confirmed_by", CollectionId: jugadores.Id, MaxSelect: 1},
			&core.AutodateField{Name: "submitted_at", OnCreate: true},
		)
		partidos.ListRule = strPtr("@request.auth.id != ''")
		partidos.ViewRule = strPtr("@request.auth.id != ''")
		partidos.CreateRule = strPtr("@request.auth.role = 'admin'")
		partidos.UpdateRule = strPtr(
			"@request.auth.id = pareja1.jugador1.user || " +
				"@request.auth.id = pareja1.jugador2.user || " +
				"@request.auth.id = pareja2.jugador1.user || " +
				"@request.auth.id = pareja2.jugador2.user",
		)
		partidos.DeleteRule = strPtr("@request.auth.role = 'admin'")
		if err := app.Save(partidos); err != nil {
			return err
		}

		// --- clasificacion (view collection) ---
		clasificacion := core.NewViewCollection("clasificacion")
		clasificacion.ViewQuery = `SELECT sub.id, sub.pareja, sub.temporada, sub.wins, sub.losses, sub.sets_won, sub.sets_lost, sub.games_won, sub.games_lost, (sub.wins * 3) AS points FROM (SELECT p.id AS id, p.id AS pareja, t.id AS temporada, COUNT(CASE WHEN m.winner = p.id THEN 1 END) AS wins, COUNT(CASE WHEN m.winner != p.id AND m.winner != '' THEN 1 END) AS losses, 0 AS sets_won, 0 AS sets_lost, 0 AS games_won, 0 AS games_lost FROM parejas p JOIN partidos m ON (m.pareja1 = p.id OR m.pareja2 = p.id) AND m.status = 'final' JOIN jornadas j ON m.jornada = j.id JOIN temporadas t ON j.temporada = t.id GROUP BY p.id, t.id) AS sub`
		clasificacion.ListRule = strPtr("@request.auth.id != ''")
		clasificacion.ViewRule = strPtr("@request.auth.id != ''")
		if err := app.Save(clasificacion); err != nil {
			return err
		}

		// --- notificaciones ---
		notificaciones := core.NewBaseCollection("notificaciones")
		notificaciones.Fields.Add(
			&core.RelationField{Name: "user", CollectionId: users.Id, Required: true, MaxSelect: 1},
			&core.SelectField{
				Name:      "type",
				Values:    []string{"quorum_request", "dispute", "match_assigned", "admin_message", "general"},
				MaxSelect: 1,
				Required:  true,
			},
			&core.TextField{Name: "title", Required: true},
			&core.TextField{Name: "body"},
			&core.BoolField{Name: "read"},
			&core.RelationField{Name: "related_partido", CollectionId: partidos.Id, MaxSelect: 1},
		)
		notificaciones.ListRule = strPtr("user = @request.auth.id")
		notificaciones.ViewRule = strPtr("user = @request.auth.id")
		notificaciones.CreateRule = nil
		notificaciones.UpdateRule = strPtr("user = @request.auth.id")
		notificaciones.DeleteRule = strPtr("user = @request.auth.id")
		if err := app.Save(notificaciones); err != nil {
			return err
		}

		return nil
	}, func(app core.App) error {
		collections := []string{
			"notificaciones", "clasificacion", "partidos", "jornadas",
			"parejas", "jugadores", "temporadas", "categorias",
		}
		for _, name := range collections {
			col, err := app.FindCollectionByNameOrId(name)
			if err == nil {
				if err := app.Delete(col); err != nil {
					return err
				}
			}
		}
		// Restore users to default (remove custom fields)
		users, err := app.FindCollectionByNameOrId("users")
		if err == nil {
			users.Fields.RemoveByName("display_name")
			users.Fields.RemoveByName("avatar")
			users.Fields.RemoveByName("role")
			if err := app.Save(users); err != nil {
				return err
			}
		}
		return nil
	})
}

func strPtr(s string) *string {
	return &s
}

func setAdminRules(c *core.Collection) {
	c.ListRule = strPtr("@request.auth.id != ''")
	c.ViewRule = strPtr("@request.auth.id != ''")
	c.CreateRule = strPtr("@request.auth.role = 'admin'")
	c.UpdateRule = strPtr("@request.auth.role = 'admin'")
	c.DeleteRule = strPtr("@request.auth.role = 'admin'")
}
