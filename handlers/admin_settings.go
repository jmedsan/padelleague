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

// Reset restarts the database: it wipes ALL non-admin data (administrators, the
// superuser, and venues are preserved), then loads example data up to the
// selected level. With nothing selected it leaves a clean database of just the
// admins. The delete/create split is inferred from the checkboxes — there is no
// separate mode.
func (h *AdminSettingsHandler) Reset(e *core.RequestEvent) error {
	if !h.devTools {
		return alertError(e, "No disponible en este entorno")
	}

	if e.Request.FormValue("confirm") != "DELETE" {
		return alertError(e, "Escribe DELETE para confirmar")
	}

	// Always wipe everything (non-admin) to a clean baseline.
	summary, err := seed.WipeSelective(h.app, seed.WipeOptions{
		Players: true, Pairs: true, Competitions: true, Matches: true,
	})
	if err != nil {
		slog.Error("reset: wipe failed", "error", err)
		return alertError(e, "Error al reiniciar la base de datos")
	}

	// Load example data up to the selected level (each stage requires the prior).
	load := seed.SampleOptions{
		Players:      e.Request.FormValue("players") == "on",
		Pairs:        e.Request.FormValue("pairs") == "on",
		Competitions: e.Request.FormValue("competitions") == "on",
		Matches:      e.Request.FormValue("matches") == "on",
	}
	if err := seed.SampleLeaguePartial(h.app, load); err != nil {
		slog.Error("reset: sample load failed", "error", err)
		return alertError(e, "Datos eliminados, pero error al cargar los datos de ejemplo")
	}

	slog.Info("reset: complete", "wiped", summary.Total(),
		"load_players", load.Players, "load_pairs", load.Pairs,
		"load_competitions", load.Competitions, "load_matches", load.Matches)

	msg := fmt.Sprintf("Base de datos reiniciada: %d registros eliminados.", summary.Total())
	if load.Players {
		msg += " Datos de ejemplo cargados."
	} else {
		msg += " Base de datos vacía."
	}
	return alertSuccess(e, msg)
}
