package handlers

import (
	"log/slog"

	"github.com/pocketbase/pocketbase/core"
)

func (h *AdminHandler) Venues(e *core.RequestEvent) error {
	venues, _ := h.app.FindRecordsByFilter("venues",
		"id != ''", "name", 0, 0, nil)

	return h.renderPage(e, "admin/venues.html", map[string]any{
		"Venues": venues,
	})
}

func (h *AdminHandler) VenuesCreate(e *core.RequestEvent) error {
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
	record.Set("courts", e.Request.FormValue("courts"))

	if err := h.app.Save(record); err != nil {
		slog.Error("save venue failed", "err", err)
		return alertError(e, "Error al guardar el club")
	}

	return redirectHX(e, "/admin/venues")
}

func (h *AdminHandler) VenuesUpdate(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("venues", id)
	if err != nil {
		return alertError(e, "Club no encontrado")
	}

	record.Set("name", e.Request.FormValue("name"))
	record.Set("address", e.Request.FormValue("address"))
	record.Set("courts", e.Request.FormValue("courts"))

	if err := h.app.Save(record); err != nil {
		slog.Error("update venue failed", "err", err)
		return alertError(e, "Error al guardar el club")
	}

	return redirectHX(e, "/admin/venues")
}

func (h *AdminHandler) VenuesDelete(e *core.RequestEvent) error {
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
