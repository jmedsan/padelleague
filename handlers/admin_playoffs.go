package handlers

import (
	"fmt"
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

func (h *AdminHandler) Playoffs(e *core.RequestEvent) error {
	temporadaID := e.Request.URL.Query().Get("temporada")
	if temporadaID == "" {
		return e.Redirect(http.StatusFound, "/admin/temporadas")
	}

	temporada, err := h.app.FindRecordById("temporadas", temporadaID)
	if err != nil {
		return e.Redirect(http.StatusFound, "/admin/temporadas")
	}

	jornadas, _ := h.app.FindRecordsByFilter("jornadas",
		"temporada = {:tid} && is_playoff = true",
		"round_number", 0, 0,
		map[string]any{"tid": temporadaID})

	type matchView struct {
		Partido    *core.Record
		PairName1  string
		PairName2  string
		StatusText string
	}
	type roundView struct {
		Jornada *core.Record
		Matches []matchView
	}

	var rounds []roundView
	for _, j := range jornadas {
		partidos, _ := h.app.FindRecordsByFilter("partidos",
			"jornada = {:jid}", "", 0, 0,
			map[string]any{"jid": j.Id})

		var matches []matchView
		for _, p := range partidos {
			pairIDs := []string{p.GetString("pareja1"), p.GetString("pareja2")}
			names, _ := expandPairNames(h.app, pairIDs)
			matches = append(matches, matchView{
				Partido:    p,
				PairName1:  names[pairIDs[0]],
				PairName2:  names[pairIDs[1]],
				StatusText: statusLabel(p.GetString("status")),
			})
		}
		rounds = append(rounds, roundView{Jornada: j, Matches: matches})
	}

	pairs, _ := h.app.FindRecordsByFilter("parejas",
		"temporada = {:tid}", "", 0, 0,
		map[string]any{"tid": temporadaID})

	type pairOption struct {
		ID   string
		Name string
	}
	var pairOptions []pairOption
	for _, p := range pairs {
		name1 := resolvePlayerName(h.app, p.GetString("jugador1"))
		name2 := resolvePlayerName(h.app, p.GetString("jugador2"))
		pairOptions = append(pairOptions, pairOption{
			ID:   p.Id,
			Name: fmt.Sprintf("%s / %s", name1, name2),
		})
	}

	return h.renderPage(e, "admin/playoffs.html", map[string]any{
		"DisplayName": e.Auth.GetString("display_name"),
		"IsAdmin":     e.Auth.GetString("role") == "admin",
		"Temporada":   temporada,
		"TemporadaID": temporadaID,
		"Rounds":      rounds,
		"Pairs":       pairOptions,
	})
}

func (h *AdminHandler) PlayoffsCreate(e *core.RequestEvent) error {
	temporadaID := e.Request.FormValue("temporada")
	if temporadaID == "" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Temporada no especificada</div>`)
	}

	if err := e.Request.ParseForm(); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al procesar el formulario</div>`)
	}
	pairIDs := e.Request.Form["pairs"]
	if len(pairIDs) < 2 || len(pairIDs)%2 != 0 {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Selecciona un número par de parejas (mínimo 2)</div>`)
	}

	existingPlayoffs, _ := h.app.FindRecordsByFilter("jornadas",
		"temporada = {:tid} && is_playoff = true",
		"-round_number", 1, 0,
		map[string]any{"tid": temporadaID})

	nextRound := 1
	if len(existingPlayoffs) > 0 {
		nextRound = int(existingPlayoffs[0].GetFloat("round_number")) + 1
	}

	err := h.app.RunInTransaction(func(txApp core.App) error {
		jornadaCol, err := txApp.FindCollectionByNameOrId("jornadas")
		if err != nil {
			return err
		}
		partidoCol, err := txApp.FindCollectionByNameOrId("partidos")
		if err != nil {
			return err
		}

		jornada := core.NewRecord(jornadaCol)
		jornada.Set("temporada", temporadaID)
		jornada.Set("round_number", nextRound)
		jornada.Set("is_playoff", true)
		if err := txApp.Save(jornada); err != nil {
			return err
		}

		for i := 0; i < len(pairIDs); i += 2 {
			partido := core.NewRecord(partidoCol)
			partido.Set("jornada", jornada.Id)
			partido.Set("pareja1", pairIDs[i])
			partido.Set("pareja2", pairIDs[i+1])
			partido.Set("status", "pending")
			if err := txApp.Save(partido); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return e.HTML(http.StatusOK, fmt.Sprintf(`<div class="alert alert-error">Error al crear la ronda: %s</div>`, err.Error()))
	}

	return e.Redirect(http.StatusFound, "/admin/playoffs?temporada="+temporadaID)
}

// statusLabel defined in match.go
