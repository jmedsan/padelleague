package handlers

import (
	"fmt"
	"log/slog"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/seed"
)

// AdminSettingsHandler handles the admin settings/reset page.
type AdminSettingsHandler struct {
	app        core.App
	devTools   bool
	renderPage RenderFunc
}

// NewAdminSettingsHandler creates an AdminSettingsHandler with the given dependencies.
func NewAdminSettingsHandler(app core.App, devTools bool, renderPage RenderFunc) *AdminSettingsHandler {
	return &AdminSettingsHandler{app: app, devTools: devTools, renderPage: renderPage}
}

// Settings renders the admin settings page.
func (h *AdminSettingsHandler) Settings(e *core.RequestEvent) error {
	return h.renderPage(e, "admin/settings.html", map[string]any{
		"DevMode": h.devTools,
	})
}

// Reset wipes selected data categories and optionally seeds a sample league.
func (h *AdminSettingsHandler) Reset(e *core.RequestEvent) error {
	if !h.devTools {
		return alertError(e, "No disponible en este entorno")
	}

	confirm := e.Request.FormValue("confirm")
	if confirm != "DELETE" {
		return alertError(e, "Escribe DELETE para confirmar")
	}

	opts := seed.WipeOptions{
		Players:      e.Request.FormValue("players") == "on",
		Pairs:        e.Request.FormValue("pairs") == "on",
		Competitions: e.Request.FormValue("competitions") == "on",
		Matches:      e.Request.FormValue("matches") == "on",
	}

	if msg := opts.ValidationMessage(); msg != "" {
		return alertError(e, msg)
	}

	summary, err := seed.WipeSelective(h.app, opts)
	if err != nil {
		slog.Error("reset: wipe failed", "error", err)
		return alertError(e, "Error al reiniciar la base de datos")
	}

	slog.Info("reset: wipe complete", "total", summary.Total(),
		"players", summary.Players, "pairs", summary.Pairs,
		"competitions", summary.Competitions, "matches", summary.Matches,
		"messages", summary.Messages, "notifications", summary.Notifications,
		"invitations", summary.Invitations, "subscriptions", summary.Subscriptions,
	)

	mode := e.Request.FormValue("mode")
	if mode == "sample" && opts.AllSelected() {
		if err := seed.SampleLeague(h.app); err != nil {
			slog.Error("reset: sample league failed", "error", err)
			return alertError(e, "Datos eliminados, pero error al crear la liga de ejemplo")
		}
		slog.Info("reset: sample league created")
	}

	total := summary.Total()
	msg := fmt.Sprintf("Base de datos reiniciada: %d registros eliminados.", total)
	if mode == "sample" && opts.AllSelected() {
		msg += " Liga de ejemplo creada."
	}
	return alertSuccess(e, msg)
}
