package handlers

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

type PairView struct {
	Record  *core.Record
	Player1 string
	Player2 string
}

func (h *AdminHandler) Pairs(e *core.RequestEvent) error {
	pairs, _ := h.app.FindAllRecords("pairs")

	var views []PairView
	for _, p := range pairs {
		views = append(views, PairView{
			Record:  p,
			Player1: resolvePlayerName(h.app, p.GetString("player1")),
			Player2: resolvePlayerName(h.app, p.GetString("player2")),
		})
	}

	users, _ := h.app.FindRecordsByFilter("users", "role = 'player'", "display_name", 0, 0, nil)

	return h.renderPage(e, "admin/pairs.html", map[string]any{
		"Pairs": views,
		"Users": users,
	})
}

func (h *AdminHandler) PairsCreate(e *core.RequestEvent) error {
	name := e.Request.FormValue("name")
	player1 := e.Request.FormValue("player1")
	player2 := e.Request.FormValue("player2")

	if name == "" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">El nombre es obligatorio</div>`)
	}
	if player1 == "" || player2 == "" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Debes seleccionar ambos jugadores</div>`)
	}
	if player1 == player2 {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Los dos jugadores deben ser diferentes</div>`)
	}

	col, err := h.app.FindCollectionByNameOrId("pairs")
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error interno</div>`)
	}

	record := core.NewRecord(col)
	record.Set("name", name)
	record.Set("player1", player1)
	record.Set("player2", player2)

	if err := h.app.Save(record); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al crear la pareja</div>`)
	}

	e.Response.Header().Set("HX-Redirect", "/admin/pairs")
	return e.NoContent(http.StatusNoContent)
}

func (h *AdminHandler) PairsUpdate(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	pair, err := h.app.FindRecordById("pairs", id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Pareja no encontrada</div>`)
	}

	if name := e.Request.FormValue("name"); name != "" {
		pair.Set("name", name)
	}
	if p1 := e.Request.FormValue("player1"); p1 != "" {
		pair.Set("player1", p1)
	}
	if p2 := e.Request.FormValue("player2"); p2 != "" {
		pair.Set("player2", p2)
	}

	if pair.GetString("player1") == pair.GetString("player2") {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Los dos jugadores deben ser diferentes</div>`)
	}

	if err := h.app.Save(pair); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al actualizar la pareja</div>`)
	}

	e.Response.Header().Set("HX-Redirect", "/admin/pairs")
	return e.NoContent(http.StatusNoContent)
}
