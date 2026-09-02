package handlers

import (
	"log/slog"

	"github.com/pocketbase/pocketbase/core"
)

// VenueHandler handles admin venue management.
type VenueHandler struct {
	app        core.App
	renderPage RenderFunc
}

// NewVenueHandler creates a VenueHandler with the given dependencies.
func NewVenueHandler(app core.App, renderPage RenderFunc) *VenueHandler {
	return &VenueHandler{app: app, renderPage: renderPage}
}

// Venues renders the admin venues management page.
func (h *VenueHandler) Venues(e *core.RequestEvent) error {
	venues, _ := h.app.FindRecordsByFilter("venues",
		"id != ''", "name", 0, 0, nil)

	return h.renderPage(e, "admin/venues.html", map[string]any{
		"PageTitle": "Pistas",
		"Venues":    venues,
	})
}

// VenuesCreate handles POST to add a new venue.
func (h *VenueHandler) VenuesCreate(e *core.RequestEvent) error {
	name := e.Request.FormValue("name")
	if name == "" {
		return alertError(e, "El nombre es obligatorio")
	}

	col, err := h.app.FindCollectionByNameOrId("venues")
	if err != nil {
		return alertError(e, "Error interno")
	}

	record := core.NewRecord(col)
	record.Set("name", name)
	record.Set("address", e.Request.FormValue("address"))

	if err := h.app.Save(record); err != nil {
		slog.Error("save venue failed", "err", err)
		return alertError(e, "Error al guardar el club")
	}

	return redirectHX(e, "/admin/venues")
}

// VenuesUpdate handles POST to modify an existing venue.
func (h *VenueHandler) VenuesUpdate(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("venues", id)
	if err != nil {
		return alertError(e, "Club no encontrado")
	}

	record.Set("name", e.Request.FormValue("name"))
	record.Set("address", e.Request.FormValue("address"))

	if err := h.app.Save(record); err != nil {
		slog.Error("update venue failed", "err", err)
		return alertError(e, "Error al guardar el club")
	}

	return redirectHX(e, "/admin/venues")
}

// VenuesDelete handles POST to remove a venue.
func (h *VenueHandler) VenuesDelete(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("venues", id)
	if err != nil {
		return alertError(e, "Club no encontrado")
	}

	if err := h.app.Delete(record); err != nil {
		slog.Error("delete venue failed", "err", err)
		return alertError(e, "Error al eliminar el club")
	}

	return redirectHX(e, "/admin/venues")
}
