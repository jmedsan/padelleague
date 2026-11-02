package handlers

import (
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
)

const (
	promotionTitleMax       = 100
	promotionDescriptionMax = 300
)

// AdminPromotionHandler manages the sponsor offers (Promociones) library.
type AdminPromotionHandler struct {
	app           core.App
	renderPage    RenderFunc
	renderPartial RenderFunc
}

// NewAdminPromotionHandler creates an AdminPromotionHandler with the given dependencies.
func NewAdminPromotionHandler(app core.App, renderPage, renderPartial RenderFunc) *AdminPromotionHandler {
	return &AdminPromotionHandler{app: app, renderPage: renderPage, renderPartial: renderPartial}
}

// List renders the admin offers page, newest first, with state and counters.
func (h *AdminPromotionHandler) List(e *core.RequestEvent) error {
	recs := findRecordsLogged(h.app, "Promotions: find promotions", RecordQuery{
		Collection: "promotions", Sort: "-created",
	})
	now := time.Now()
	loc := league.Timezone(h.app)
	views := make([]PromotionView, 0, len(recs))
	for _, r := range recs {
		views = append(views, NewPromotionView(r, AdminRow, now, loc))
	}
	return h.renderPage(e, "admin/promotions.html", map[string]any{
		"PageTitle":  "Promociones",
		"Promotions": views,
	})
}

// EditForm returns the edit form fragment for one offer.
func (h *AdminPromotionHandler) EditForm(e *core.RequestEvent) error {
	rec, err := h.app.FindRecordById("promotions", e.Request.PathValue("id"))
	if err != nil {
		return e.NotFoundError("", nil)
	}
	v := NewPromotionView(rec, AdminRow, time.Now(), league.Timezone(h.app))
	expires := ""
	if exp := rec.GetDateTime("expires_on"); !exp.IsZero() {
		expires = exp.Time().UTC().Format(time.DateOnly)
	}
	return h.renderPartial(e, "admin/promotion-edit-form.html", map[string]any{
		"Promotion": v,
		"ExpiresOn": expires,
	})
}

// Create adds an offer. A new offer is active.
func (h *AdminPromotionHandler) Create(e *core.RequestEvent) error {
	col, err := h.app.FindCollectionByNameOrId("promotions")
	if err != nil {
		slog.Error("Promotions: find collection", "err", err)
		return alertError(e, "Error interno")
	}
	rec := core.NewRecord(col)
	rec.Set("active", true)
	if msg := applyPromotionForm(e, rec); msg != "" {
		return alertError(e, msg)
	}
	if err := h.app.Save(rec); err != nil {
		slog.Error("Promotions: save promotion", "err", err)
		return alertError(e, "Error al guardar la promoción")
	}
	flash(e, "Promoción creada")
	return redirectHX(e, "/admin/promotions")
}

// Update edits an offer. Active, views and clicks are never touched, and the
// load and save share a transaction so a concurrent click is not lost.
func (h *AdminPromotionHandler) Update(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	var msg string
	found := true
	err := h.app.RunInTransaction(func(tx core.App) error {
		rec, err := tx.FindRecordById("promotions", id)
		if err != nil {
			found = false
			return nil
		}
		if msg = applyPromotionForm(e, rec); msg != "" {
			return nil
		}
		return tx.Save(rec)
	})
	switch {
	case !found:
		return alertError(e, "Promoción no encontrada")
	case msg != "":
		return alertError(e, msg)
	case err != nil:
		slog.Error("Promotions: update promotion", "id", id, "err", err)
		return alertError(e, "Error al guardar la promoción")
	}
	flash(e, "Promoción actualizada")
	return redirectHX(e, "/admin/promotions")
}

// Toggle flips only the active flag.
func (h *AdminPromotionHandler) Toggle(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	found := true
	err := h.app.RunInTransaction(func(tx core.App) error {
		rec, err := tx.FindRecordById("promotions", id)
		if err != nil {
			found = false
			return nil
		}
		rec.Set("active", !rec.GetBool("active"))
		return tx.Save(rec)
	})
	if !found {
		return alertError(e, "Promoción no encontrada")
	}
	if err != nil {
		slog.Error("Promotions: toggle promotion", "id", id, "err", err)
		return alertError(e, "Error al guardar la promoción")
	}
	return redirectHX(e, "/admin/promotions")
}

// Delete removes an offer and its image.
func (h *AdminPromotionHandler) Delete(e *core.RequestEvent) error {
	rec, err := h.app.FindRecordById("promotions", e.Request.PathValue("id"))
	if err != nil {
		return alertError(e, "Promoción no encontrada")
	}
	if err := h.app.Delete(rec); err != nil {
		slog.Error("Promotions: delete promotion", "id", rec.Id, "err", err)
		return alertError(e, "Error al eliminar la promoción")
	}
	flash(e, "Promoción eliminada")
	return redirectHX(e, "/admin/promotions")
}

// applyPromotionForm validates the posted form and copies it onto rec. It
// returns a Spanish error message, or "" when rec was updated.
func applyPromotionForm(e *core.RequestEvent, rec *core.Record) string {
	title := strings.TrimSpace(e.Request.FormValue("title"))
	if title == "" {
		return "El título es obligatorio"
	}
	if utf8.RuneCountInString(title) > promotionTitleMax {
		return "El título no puede superar los 100 caracteres"
	}
	description := strings.TrimSpace(e.Request.FormValue("description"))
	if utf8.RuneCountInString(description) > promotionDescriptionMax {
		return "La descripción no puede superar los 300 caracteres"
	}
	link := strings.TrimSpace(e.Request.FormValue("url"))
	if !league.ValidPromotionURL(link) {
		return "El enlace debe empezar por http:// o https://"
	}
	// The collection's URL validator wants a lowercase scheme.
	if i := strings.Index(link, "://"); i > 0 {
		link = strings.ToLower(link[:i]) + link[i:]
	}
	expiresAt, errMsg := promotionExpiry(e.Request.FormValue("expires_on"))
	if errMsg != "" {
		return errMsg
	}
	if errMsg := applyPromotionImage(e, rec); errMsg != "" {
		return errMsg
	}
	rec.Set("title", title)
	rec.Set("description", description)
	rec.Set("url", link)
	rec.Set("expires_on", expiresAt)
	return ""
}

// promotionExpiry parses the picked day into the stored 12:00 UTC value ("" when empty).
func promotionExpiry(raw string) (any, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	day, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return nil, "La fecha de caducidad no es válida"
	}
	return day.Add(12 * time.Hour), ""
}

// applyPromotionImage sets, keeps or clears rec's image from the form.
func applyPromotionImage(e *core.RequestEvent, rec *core.Record) string {
	fh := fileHeader(e, "image")
	if fh == nil {
		if e.Request.FormValue("remove_image") == "on" {
			rec.Set("image", "")
		}
		return ""
	}
	if !strings.HasPrefix(fh.Header.Get("Content-Type"), "image/") {
		return "El archivo debe ser una imagen"
	}
	if fh.Size > avatarMaxUploadSize {
		return "La imagen no puede superar los 5 MB"
	}
	f, errMsg := compressLogo(fh, "promotion_image.jpg")
	if errMsg != "" {
		return errMsg
	}
	rec.Set("image", f)
	return ""
}
