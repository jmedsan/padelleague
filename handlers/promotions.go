package handlers

import (
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
	"padelleague/render"
)

// PromotionHandler serves the player side of Promociones: the offer list, the
// random card slot and the counted redirect.
type PromotionHandler struct {
	app           core.App
	renderPage    RenderFunc
	renderPartial RenderFunc
	errorPage     RenderErrorFunc
}

// NewPromotionHandler creates a PromotionHandler with the given dependencies.
func NewPromotionHandler(app core.App, renderPage, renderPartial RenderFunc, errorPage RenderErrorFunc) *PromotionHandler {
	return &PromotionHandler{app: app, renderPage: renderPage, renderPartial: renderPartial, errorPage: errorPage}
}

// List renders every current offer. It counts no view.
func (h *PromotionHandler) List(e *core.RequestEvent) error {
	now := time.Now()
	recs, err := league.CurrentPromotions(h.app, now)
	if err != nil {
		slog.Error("Promotions: list current offers", "err", err)
		return h.errorPage(e, http.StatusInternalServerError, "Error al cargar las promociones")
	}
	loc := league.Timezone(h.app)
	views := make([]PromotionView, 0, len(recs))
	for _, r := range recs {
		views = append(views, NewPromotionView(r, PlayerReadOnly, now, loc))
	}
	return h.renderPage(e, "promotions.html", map[string]any{
		"PageTitle":  "Promociones",
		"Promotions": views,
	})
}

// Card picks a random current offer that the browser has not dismissed, counts
// the view and renders the card. It answers 204 when there is nothing to show
// and in admin view, which never counts.
func (h *PromotionHandler) Card(e *core.RequestEvent) error {
	if render.AdminView(e) {
		return e.NoContent(http.StatusNoContent)
	}
	now := time.Now()
	recs, err := league.CurrentPromotions(h.app, now)
	if err != nil {
		slog.Error("Promotions: card: list current offers", "err", err)
		return e.NoContent(http.StatusNoContent)
	}
	rec := league.PickPromotion(recs, excludedIDs(e.Request.URL.Query().Get("exclude")), rand.IntN)
	if rec == nil {
		return e.NoContent(http.StatusNoContent)
	}
	if err := league.AddPromotionView(h.app, rec.Id); err != nil {
		slog.Error("Promotions: count view", "promotion", rec.Id, "err", err)
	}
	view := NewPromotionView(rec, PlayerSummary, now, league.Timezone(h.app))
	return h.renderPartial(e, "promotion-card-fragment.html", map[string]any{"Promotion": view})
}

// Click redirects to an available offer's URL and counts the click for a
// player. Admin view redirects without counting. Anything else is a 404.
func (h *PromotionHandler) Click(e *core.RequestEvent) error {
	rec, err := h.app.FindRecordById("promotions", e.Request.PathValue("id"))
	if err != nil || !league.PromotionCurrent(rec, time.Now(), league.Timezone(h.app)) ||
		!league.ValidPromotionURL(rec.GetString("url")) {
		return h.errorPage(e, http.StatusNotFound, "Promoción no disponible")
	}
	if !render.AdminView(e) {
		if err := league.AddPromotionClick(h.app, rec.Id); err != nil {
			slog.Error("Promotions: count click", "promotion", rec.Id, "err", err)
		}
	}
	return e.Redirect(http.StatusFound, rec.GetString("url"))
}

// excludedIDs parses the comma-separated exclude query value.
func excludedIDs(raw string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, id := range strings.Split(raw, ",") {
		if id = strings.TrimSpace(id); id != "" {
			out[id] = struct{}{}
		}
	}
	return out
}
