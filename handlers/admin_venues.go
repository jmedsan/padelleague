package handlers

import (
	"fmt"
	"net/http"

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
		return e.HTML(http.StatusOK, `<div class="alert alert-error">El nombre es obligatorio</div>`)
	}

	col, err := h.app.FindCollectionByNameOrId("venues")
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error interno</div>`)
	}

	record := core.NewRecord(col)
	record.Set("name", name)
	record.Set("address", e.Request.FormValue("address"))
	record.Set("courts", e.Request.FormValue("courts"))

	if err := h.app.Save(record); err != nil {
		return e.HTML(http.StatusOK, fmt.Sprintf(`<div class="alert alert-error">Error: %s</div>`, err.Error()))
	}

	e.Response.Header().Set("HX-Redirect", "/admin/venues")
	return e.NoContent(http.StatusNoContent)
}

func (h *AdminHandler) VenuesUpdate(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("venues", id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Club no encontrado</div>`)
	}

	record.Set("name", e.Request.FormValue("name"))
	record.Set("address", e.Request.FormValue("address"))
	record.Set("courts", e.Request.FormValue("courts"))

	if err := h.app.Save(record); err != nil {
		return e.HTML(http.StatusOK, fmt.Sprintf(`<div class="alert alert-error">Error: %s</div>`, err.Error()))
	}

	e.Response.Header().Set("HX-Redirect", "/admin/venues")
	return e.NoContent(http.StatusNoContent)
}

func (h *AdminHandler) VenuesDelete(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("venues", id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Club no encontrado</div>`)
	}

	if err := h.app.Delete(record); err != nil {
		return e.HTML(http.StatusOK, fmt.Sprintf(`<div class="alert alert-error">Error: %s</div>`, err.Error()))
	}

	e.Response.Header().Set("HX-Redirect", "/admin/venues")
	return e.NoContent(http.StatusNoContent)
}
