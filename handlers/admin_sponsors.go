package handlers

import (
	"log/slog"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
)

// AdminSponsorHandler handles admin sponsor library management and
// per-competition attach/detach.
type AdminSponsorHandler struct {
	app        core.App
	renderPage RenderFunc
}

// NewAdminSponsorHandler creates an AdminSponsorHandler with the given dependencies.
func NewAdminSponsorHandler(app core.App, renderPage RenderFunc) *AdminSponsorHandler {
	return &AdminSponsorHandler{app: app, renderPage: renderPage}
}

// Sponsors renders the admin sponsors library page.
func (h *AdminSponsorHandler) Sponsors(e *core.RequestEvent) error {
	sponsors := findRecordsLogged(h.app, "Sponsors: find sponsors", RecordQuery{
		Collection: "sponsors", Sort: "name",
	})
	return h.renderPage(e, "admin/sponsors.html", map[string]any{
		"PageTitle": "Patrocinadores",
		"Sponsors":  sponsors,
	})
}

// SponsorsCreate handles POST to add a new sponsor (name + logo + optional URL).
func (h *AdminSponsorHandler) SponsorsCreate(e *core.RequestEvent) error {
	name := strings.TrimSpace(e.Request.FormValue("name"))
	if name == "" {
		return alertError(e, "El nombre es obligatorio")
	}

	fh := fileHeader(e, "logo")
	if fh == nil {
		return alertError(e, "Selecciona un logo")
	}
	if !strings.HasPrefix(fh.Header.Get("Content-Type"), "image/") {
		return alertError(e, "El archivo debe ser una imagen")
	}
	if fh.Size > avatarMaxUploadSize {
		return alertError(e, "La imagen no puede superar los 5 MB")
	}

	col, err := h.app.FindCollectionByNameOrId("sponsors")
	if err != nil {
		return alertError(e, "Error interno")
	}

	record := core.NewRecord(col)
	record.Set("name", name)
	record.Set("url", e.Request.FormValue("url"))
	if err := h.app.Save(record); err != nil {
		return alertError(e, "Error al crear el patrocinador")
	}

	f, errMsg := compressLogo(fh, record.Id+"_logo.jpg")
	if errMsg != "" {
		_ = h.app.Delete(record)
		return alertError(e, errMsg)
	}
	record.Set("logo", f)

	if err := h.app.Save(record); err != nil {
		slog.Error("save sponsor failed", "err", err)
		return alertError(e, "Error al guardar el patrocinador")
	}

	return redirectHX(e, "/admin/sponsors")
}

// SponsorsDelete handles POST to remove a sponsor from the library.
func (h *AdminSponsorHandler) SponsorsDelete(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("sponsors", id)
	if err != nil {
		return alertError(e, "Patrocinador no encontrado")
	}

	if err := h.app.Delete(record); err != nil {
		slog.Error("delete sponsor failed", "err", err)
		return alertError(e, "Error al eliminar el patrocinador")
	}

	return redirectHX(e, "/admin/sponsors")
}

// AttachSponsor adds a sponsor to a competition's sponsors relation.
func (h *AdminSponsorHandler) AttachSponsor(e *core.RequestEvent) error {
	comp, err := h.app.FindRecordById("competitions", e.Request.PathValue("id"))
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}
	sponsorID := e.Request.FormValue("sponsor")
	comp.Set("sponsors", league.AppendUnique(comp.GetStringSlice("sponsors"), sponsorID))
	if err := h.app.Save(comp); err != nil {
		return alertError(e, "Error al adjuntar el patrocinador")
	}
	return redirectHX(e, "/admin/competitions/"+comp.Id)
}

// DetachSponsor removes a sponsor from a competition's sponsors relation.
func (h *AdminSponsorHandler) DetachSponsor(e *core.RequestEvent) error {
	comp, err := h.app.FindRecordById("competitions", e.Request.PathValue("id"))
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}
	sponsorID := e.Request.PathValue("sponsorId")
	comp.Set("sponsors", league.RemoveString(comp.GetStringSlice("sponsors"), sponsorID))
	if err := h.app.Save(comp); err != nil {
		return alertError(e, "Error al quitar el patrocinador")
	}
	return redirectHX(e, "/admin/competitions/"+comp.Id)
}
