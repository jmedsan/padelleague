package handlers

import (
	"fmt"
	"html"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/seed"
)

// AdminSettingsHandler handles the admin settings/reset page.
type AdminSettingsHandler struct {
	app        core.App
	devTools   bool
	staticFS   fs.FS
	renderPage RenderFunc
}

// NewAdminSettingsHandler creates an AdminSettingsHandler with the given dependencies.
func NewAdminSettingsHandler(app core.App, devTools bool, staticFS fs.FS, renderPage RenderFunc) *AdminSettingsHandler {
	return &AdminSettingsHandler{app: app, devTools: devTools, staticFS: staticFS, renderPage: renderPage}
}

// Settings renders the admin settings page.
func (h *AdminSettingsHandler) Settings(e *core.RequestEvent) error {
	return h.renderPage(e, "admin/settings.html", map[string]any{
		"PageTitle": "Configuración",
		"DevMode":   h.devTools,
	})
}

// Reset restarts the database: it wipes ALL non-admin data (administrators, the
// superuser, and venues are preserved), then loads example data up to the
// selected level. With nothing selected it leaves a clean database of just the
// admins. The delete/create split is inferred from the checkboxes — there is no
// separate mode.
//
// The reset button's own hx-target is #reset-result; alertError/alertSuccess
// would override it via HX-Retarget to the global #flash container, so every
// response here is built directly instead of going through the flash helpers.
func (h *AdminSettingsHandler) Reset(e *core.RequestEvent) error {
	if !h.devTools {
		return resetResult(e, "alert-error", "No disponible en este entorno")
	}

	if e.Request.FormValue("confirm") != "DELETE" {
		return resetResult(e, "alert-error", "Escribe DELETE para confirmar")
	}

	// Always wipe everything (non-admin) to a clean baseline.
	summary, err := seed.WipeSelective(h.app, seed.WipeOptions{
		Players: true, Pairs: true, Competitions: true, Matches: true,
	})
	if err != nil {
		slog.Error("reset: wipe failed", "error", err)
		return resetResult(e, "alert-error", "Error al reiniciar la base de datos")
	}

	// Load example data up to the selected level (each stage requires the prior).
	load := seed.SampleOptions{
		Players:      e.Request.FormValue("players") == "on",
		Pairs:        e.Request.FormValue("pairs") == "on",
		Competitions: e.Request.FormValue("competitions") == "on",
		Matches:      e.Request.FormValue("matches") == "on",
		Playoff:      e.Request.FormValue("playoff") == "on",
		StaticFS:     h.staticFS,
	}
	if err := seed.SampleLeaguePartial(h.app, load); err != nil {
		slog.Error("reset: sample load failed", "error", err)
		return resetResult(e, "alert-error", "Datos eliminados, pero error al cargar los datos de ejemplo")
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
	return resetResult(e, "alert-success", msg)
}

func resetResult(e *core.RequestEvent, class, msg string) error {
	fragment := `<div class="` + class + ` text-sm py-2">` + html.EscapeString(msg) + `</div>`
	return e.HTML(http.StatusOK, fragment)
}
