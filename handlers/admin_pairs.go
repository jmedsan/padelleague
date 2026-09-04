package handlers

import (
	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
)

// PairHandler handles admin pair management.
type PairHandler struct {
	app        core.App
	renderPage RenderFunc
}

// NewPairHandler creates a PairHandler with the given dependencies.
func NewPairHandler(app core.App, renderPage RenderFunc) *PairHandler {
	return &PairHandler{app: app, renderPage: renderPage}
}

// PairView holds a pair record with resolved player names for display.
type PairView struct {
	Record        *core.Record
	Player1       string
	Player2       string
	Player1Avatar string
	Player2Avatar string
}

// Pairs renders the admin pairs management page.
func (h *PairHandler) Pairs(e *core.RequestEvent) error {
	pairs, _ := h.app.FindRecordsByFilter("pairs", "id != ''", "name", 0, 0, nil)

	var views []PairView
	for _, p := range pairs {
		views = append(views, PairView{
			Record:        p,
			Player1:       league.PlayerName(h.app, p.GetString("player1")),
			Player2:       league.PlayerName(h.app, p.GetString("player2")),
			Player1Avatar: league.PlayerAvatarURL(h.app, p.GetString("player1")),
			Player2Avatar: league.PlayerAvatarURL(h.app, p.GetString("player2")),
		})
	}

	users, _ := h.app.FindRecordsByFilter("users", "roles ~ 'player'", "display_name", 0, 0, nil)

	return h.renderPage(e, "admin/pairs.html", map[string]any{
		"PageTitle": "Parejas",
		"Pairs":     views,
		"Users":     users,
		"Mode":      AdminSummary,
	})
}

// PairsCreate handles POST to create a new pair from two players.
func (h *PairHandler) PairsCreate(e *core.RequestEvent) error {
	name := e.Request.FormValue("name")
	player1 := e.Request.FormValue("player1")
	player2 := e.Request.FormValue("player2")

	if name == "" {
		return alertError(e, "El nombre es obligatorio")
	}
	if player1 == "" || player2 == "" {
		return alertError(e, "Debes seleccionar ambos jugadores")
	}
	if player1 == player2 {
		return alertError(e, "Los dos jugadores deben ser diferentes")
	}

	col, err := h.app.FindCollectionByNameOrId("pairs")
	if err != nil {
		return alertError(e, "Error interno")
	}

	record := core.NewRecord(col)
	record.Set("name", name)
	record.Set("player1", player1)
	record.Set("player2", player2)

	if err := h.app.Save(record); err != nil {
		return alertError(e, "Error al crear la pareja")
	}

	flash(e, "Pareja creada")
	return redirectHX(e, "/admin/pairs")
}

// PairsUpdate handles POST to change the players in an existing pair.
func (h *PairHandler) PairsUpdate(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	pair, err := h.app.FindRecordById("pairs", id)
	if err != nil {
		return alertError(e, "Pareja no encontrada")
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
		return alertError(e, "Los dos jugadores deben ser diferentes")
	}

	if err := h.app.Save(pair); err != nil {
		return alertError(e, "Error al actualizar la pareja")
	}

	flash(e, "Pareja actualizada")
	return redirectHX(e, "/admin/pairs")
}
