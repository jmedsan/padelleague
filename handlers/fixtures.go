package handlers

import (
	"fmt"
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

type Round struct {
	Number  int
	Matches []Match
}

type Match struct {
	Home string
	Away string
}

func generateRoundRobin(pairIDs []string, double bool) []Round {
	n := len(pairIDs)
	if n < 2 {
		return nil
	}

	pairs := make([]string, len(pairIDs))
	copy(pairs, pairIDs)

	if n%2 == 1 {
		pairs = append(pairs, "")
		n++
	}

	rounds := make([]Round, 0, n-1)
	for r := 0; r < n-1; r++ {
		var matches []Match
		for i := 0; i < n/2; i++ {
			home := pairs[i]
			away := pairs[n-1-i]
			if home != "" && away != "" {
				matches = append(matches, Match{Home: home, Away: away})
			}
		}
		rounds = append(rounds, Round{Number: r + 1, Matches: matches})
		last := pairs[n-1]
		copy(pairs[2:], pairs[1:n-1])
		pairs[1] = last
	}

	if double {
		half := len(rounds)
		for i := 0; i < half; i++ {
			var swapped []Match
			for _, m := range rounds[i].Matches {
				swapped = append(swapped, Match{Home: m.Away, Away: m.Home})
			}
			rounds = append(rounds, Round{Number: half + i + 1, Matches: swapped})
		}
	}

	return rounds
}

func expandPairNames(app core.App, pairIDs []string) (map[string]string, error) {
	names := make(map[string]string, len(pairIDs))
	for _, id := range pairIDs {
		if id == "" {
			continue
		}
		pair, err := app.FindRecordById("parejas", id)
		if err != nil {
			names[id] = "Pareja desconocida"
			continue
		}

		j1ID := pair.GetString("jugador1")
		j2ID := pair.GetString("jugador2")

		name1 := resolvePlayerName(app, j1ID)
		name2 := resolvePlayerName(app, j2ID)

		names[id] = fmt.Sprintf("%s / %s", name1, name2)
	}
	return names, nil
}

func resolvePlayerName(app core.App, jugadorID string) string {
	if jugadorID == "" {
		return "?"
	}
	jugador, err := app.FindRecordById("jugadores", jugadorID)
	if err != nil {
		return "?"
	}
	userID := jugador.GetString("user")
	if userID == "" {
		return "?"
	}
	user, err := app.FindRecordById("users", userID)
	if err != nil {
		return "?"
	}
	return user.GetString("display_name")
}

type FixtureHandler struct {
	app        core.App
	renderPage func(e *core.RequestEvent, page string, data map[string]any) error
}

func NewFixtureHandler(app core.App, renderPage func(e *core.RequestEvent, page string, data map[string]any) error) *FixtureHandler {
	return &FixtureHandler{app: app, renderPage: renderPage}
}

func (h *FixtureHandler) GenerateFixtures(e *core.RequestEvent) error {
	seasonID := e.Request.PathValue("id")
	confirm := e.Request.URL.Query().Get("confirm") == "true"

	season, err := h.app.FindRecordById("temporadas", seasonID)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Temporada no encontrada</div>`)
	}

	existingJornadas, _ := h.app.FindRecordsByFilter("jornadas",
		"temporada = {:id} && is_playoff = false",
		"-round_number", 0, 0,
		map[string]any{"id": seasonID})

	if len(existingJornadas) > 0 && !confirm {
		return e.HTML(http.StatusOK, fmt.Sprintf(`
			<div class="alert alert-warning">
				<span>Ya existen %d jornadas para esta temporada. ¿Desea regenerar? Esto eliminará los partidos existentes.</span>
				<button hx-post="/admin/temporadas/%s/generate?confirm=true" hx-target="#generate-result" class="btn btn-sm btn-warning">Confirmar</button>
			</div>`, len(existingJornadas), seasonID))
	}

	pairs, _ := h.app.FindRecordsByFilter("parejas",
		"temporada = {:id}",
		"", 0, 0,
		map[string]any{"id": seasonID})

	if len(pairs) < 2 {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Se necesitan al menos 2 parejas para generar el calendario</div>`)
	}

	pairIDs := make([]string, len(pairs))
	for i, p := range pairs {
		pairIDs[i] = p.Id
	}

	double := season.GetBool("play_twice")
	rounds := generateRoundRobin(pairIDs, double)

	err = h.app.RunInTransaction(func(txApp core.App) error {
		for _, j := range existingJornadas {
			partidos, _ := txApp.FindRecordsByFilter("partidos",
				"jornada = {:jid}",
				"", 0, 0,
				map[string]any{"jid": j.Id})
			for _, p := range partidos {
				if err := txApp.Delete(p); err != nil {
					return err
				}
			}
			if err := txApp.Delete(j); err != nil {
				return err
			}
		}

		jornadaCol, err := txApp.FindCollectionByNameOrId("jornadas")
		if err != nil {
			return err
		}
		partidoCol, err := txApp.FindCollectionByNameOrId("partidos")
		if err != nil {
			return err
		}

		for _, round := range rounds {
			jornada := core.NewRecord(jornadaCol)
			jornada.Set("temporada", seasonID)
			jornada.Set("round_number", round.Number)
			jornada.Set("is_playoff", false)
			if err := txApp.Save(jornada); err != nil {
				return err
			}

			for _, match := range round.Matches {
				partido := core.NewRecord(partidoCol)
				partido.Set("jornada", jornada.Id)
				partido.Set("pareja1", match.Home)
				partido.Set("pareja2", match.Away)
				partido.Set("status", "pending")
				if err := txApp.Save(partido); err != nil {
					return err
				}
			}
		}
		return nil
	})

	if err != nil {
		return e.HTML(http.StatusOK, fmt.Sprintf(`<div class="alert alert-error">Error al generar: %s</div>`, err.Error()))
	}

	totalMatches := 0
	for _, r := range rounds {
		totalMatches += len(r.Matches)
	}

	return e.HTML(http.StatusOK, fmt.Sprintf(`<div class="alert alert-success">Calendario generado: %d jornadas, %d partidos</div>`, len(rounds), totalMatches))
}
