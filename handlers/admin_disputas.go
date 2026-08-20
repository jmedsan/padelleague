package handlers

import (
	"fmt"
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

type DisputaView struct {
	Partido      *core.Record
	Pareja1Name  string
	Pareja2Name  string
	SubmittedBy  string
	DisputedBy   string
	DisputeNotes string
}

func (h *AdminHandler) Disputas(e *core.RequestEvent) error {
	partidos, _ := h.app.FindRecordsByFilter("partidos",
		"status = 'disputed'", "", 0, 0, nil)

	var views []DisputaView
	for _, p := range partidos {
		pairIDs := []string{p.GetString("pareja1"), p.GetString("pareja2")}
		pairNames, _ := expandPairNames(h.app, pairIDs)

		submittedBy := resolveJugadorName(h.app, p.GetString("submitted_by"))
		disputedBy := resolveJugadorName(h.app, p.GetString("disputed_by"))

		views = append(views, DisputaView{
			Partido:      p,
			Pareja1Name:  pairNames[pairIDs[0]],
			Pareja2Name:  pairNames[pairIDs[1]],
			SubmittedBy:  submittedBy,
			DisputedBy:   disputedBy,
			DisputeNotes: p.GetString("dispute_notes"),
		})
	}

	return h.renderPage(e, "admin/disputas.html", map[string]any{
		"Disputas": views,
	})
}

func (h *AdminHandler) DisputasResolve(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	partido, err := h.app.FindRecordById("partidos", id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Partido no encontrado</div>`)
	}

	if partido.GetString("status") != "disputed" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Este partido no está en disputa</div>`)
	}

	score := e.Request.FormValue("score")
	if score == "" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Debes ingresar un marcador</div>`)
	}

	manualWinner := e.Request.FormValue("winner")
	var winnerID string
	if manualWinner != "" {
		if manualWinner != partido.GetString("pareja1") && manualWinner != partido.GetString("pareja2") {
			return e.HTML(http.StatusOK, `<div class="alert alert-error">El ganador debe ser una de las dos parejas</div>`)
		}
		winnerID = manualWinner
	} else {
		var err error
		winnerID, err = determineWinner(partido, score)
		if err != nil {
			return e.HTML(http.StatusOK, fmt.Sprintf(`<div class="alert alert-error">Marcador inválido: %s. Selecciona el ganador manualmente.</div>`, err.Error()))
		}
	}

	partido.Set("scores", score)
	partido.Set("winner", winnerID)
	partido.Set("status", "final")

	if err := h.app.Save(partido); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al resolver la disputa</div>`)
	}

	allPlayers := append(getPlayersForPair(h.app, partido.GetString("pareja1")),
		getPlayersForPair(h.app, partido.GetString("pareja2"))...)
	notifyPlayers(h.app, allPlayers, "dispute", "Disputa resuelta", "Un administrador ha resuelto la disputa de tu partido.", partido.Id)

	e.Response.Header().Set("HX-Redirect", "/admin/disputas")
	return e.NoContent(http.StatusNoContent)
}

func resolveJugadorName(app core.App, jugadorID string) string {
	if jugadorID == "" {
		return "—"
	}
	return resolvePlayerName(app, jugadorID)
}
