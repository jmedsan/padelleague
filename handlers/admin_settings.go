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
	appEnv     string
	renderPage RenderFunc
}

// NewAdminSettingsHandler creates an AdminSettingsHandler with the given dependencies.
func NewAdminSettingsHandler(app core.App, appEnv string, renderPage RenderFunc) *AdminSettingsHandler {
	return &AdminSettingsHandler{app: app, appEnv: appEnv, renderPage: renderPage}
}

// Settings renders the admin settings page.
func (h *AdminSettingsHandler) Settings(e *core.RequestEvent) error {
	return h.renderPage(e, "admin/settings.html", map[string]any{
		"DevMode": h.appEnv == "dev",
	})
}

// Reset wipes in-scope data and optionally seeds a sample league.
func (h *AdminSettingsHandler) Reset(e *core.RequestEvent) error {
	if h.appEnv != "dev" {
		return alertError(e, "No disponible en este entorno")
	}

	confirm := e.Request.FormValue("confirm")
	if confirm != "DELETE" {
		return alertError(e, "Escribe DELETE para confirmar")
	}

	admins, err := h.app.FindRecordsByFilter("users", "role = 'admin'", "", 0, 0)
	if err != nil {
		slog.Error("reset: finding admins", "error", err)
		return alertError(e, "Error interno al buscar administradores")
	}

	keep := make(map[string]struct{}, len(admins))
	for _, a := range admins {
		keep[a.Id] = struct{}{}
	}

	summary, err := seed.Wipe(h.app, keep)
	if err != nil {
		slog.Error("reset: wipe failed", "error", err)
		return alertError(e, "Error al reiniciar la base de datos")
	}

	slog.Info("reset: wipe complete",
		"competitions", summary.Competitions,
		"pairs", summary.Pairs,
		"players", summary.Players,
		"matches", summary.Matches,
		"messages", summary.Messages,
		"notifications", summary.Notifications,
		"invitations", summary.Invitations,
		"subscriptions", summary.Subscriptions,
	)

	mode := e.Request.FormValue("mode")
	if mode == "sample" {
		if err := seed.SampleLeague(h.app); err != nil {
			slog.Error("reset: sample league failed", "error", err)
			return alertError(e, "Datos eliminados, pero error al crear la liga de ejemplo")
		}
		slog.Info("reset: sample league created")
	}

	total := summary.Competitions + summary.Pairs + summary.Players +
		summary.Matches + summary.Messages + summary.Notifications +
		summary.Invitations + summary.Subscriptions

	msg := fmt.Sprintf("Base de datos reiniciada: %d registros eliminados.", total)
	if mode == "sample" {
		msg += " Liga de ejemplo creada."
	}

	return alertSuccess(e, msg)
}
