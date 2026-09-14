package handlers

import (
	"log/slog"

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
	CaptainID     string
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
			CaptainID:     p.GetString("captain"),
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
	captain := e.Request.FormValue("captain")
	if captain != player1 && captain != player2 {
		return alertError(e, "El capitán debe ser uno de los dos jugadores")
	}

	record.Set("name", name)
	record.Set("player1", player1)
	record.Set("player2", player2)
	record.Set("captain", captain)

	compID := e.Request.FormValue("competition_id")
	if err := h.app.RunInTransaction(func(txApp core.App) error {
		if err := txApp.Save(record); err != nil {
			return err
		}
		if compID == "" {
			return nil
		}
		comp, err := txApp.FindRecordById("competitions", compID)
		if err != nil {
			return err
		}
		pairs := comp.GetStringSlice("pairs")
		pairs = append(pairs, record.Id)
		comp.Set("pairs", pairs)
		return txApp.Save(comp)
	}); err != nil {
		slog.Error("create pair", "err", err)
		return alertError(e, "Error al crear la pareja")
	}

	if compID != "" {
		flash(e, "Pareja creada y añadida")
		return redirectHX(e, "/admin/competitions/"+compID)
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

	captain := e.Request.FormValue("captain")
	p1 := pair.GetString("player1")
	p2 := pair.GetString("player2")
	if captain == "" {
		// Preserve existing captain if still valid for the current players.
		existing := pair.GetString("captain")
		if existing != p1 && existing != p2 {
			return alertError(e, "El capitán debe ser uno de los dos jugadores")
		}
	} else if captain != p1 && captain != p2 {
		return alertError(e, "El capitán debe ser uno de los dos jugadores")
	} else {
		pair.Set("captain", captain)
	}

	comps, _ := h.app.FindRecordsByFilter("competitions",
		"pairs ~ {:pid}", "", 0, 0, map[string]any{"pid": id})
	for _, comp := range comps {
		if err := validatePlayerUniqueness(h.app, comp.GetStringSlice("pairs"), pair, id); err != nil {
			return alertError(e, "Un jugador de esta pareja ya participa en otra pareja de "+comp.GetString("name"))
		}
	}

	if err := h.app.Save(pair); err != nil {
		return alertError(e, "Error al actualizar la pareja")
	}

	flash(e, "Pareja actualizada")
	return redirectHX(e, "/admin/pairs")
}
