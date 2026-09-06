package handlers

import (
	"fmt"
	"html"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
	"padelleague/notify"
	"padelleague/seed"
)

// AdminDevToolsHandler serves the dev-only tools page, gated by APP_DEV_TOOLS.
type AdminDevToolsHandler struct {
	app        core.App
	notifier   *notify.Notifier
	staticFS   fs.FS
	renderPage RenderFunc
}

// NewAdminDevToolsHandler creates an AdminDevToolsHandler.
func NewAdminDevToolsHandler(app core.App, notifier *notify.Notifier, staticFS fs.FS, renderPage RenderFunc) *AdminDevToolsHandler {
	return &AdminDevToolsHandler{app: app, notifier: notifier, staticFS: staticFS, renderPage: renderPage}
}

// DevTools renders the dev tools page.
func (h *AdminDevToolsHandler) DevTools(e *core.RequestEvent) error {
	return h.renderPage(e, "admin/dev-tools.html", map[string]any{
		"PageTitle": "Herramientas de desarrollo",
	})
}

// TestPush sends a test notification through the real pipeline to the current admin.
func (h *AdminDevToolsHandler) TestPush(e *core.RequestEvent) error {
	h.notifier.NotifyPlayers([]string{e.Auth.Id}, league.Notification{
		Type:  "general",
		Title: "Notificación de prueba",
		Body:  "Si ves esto, las notificaciones funcionan correctamente.",
		Link:  "/admin/dev-tools",
	})
	flash(e, "Notificación de prueba enviada")
	return redirectHX(e, "/admin/dev-tools")
}

// TestEmail sends a test email to the current admin user.
func (h *AdminDevToolsHandler) TestEmail(e *core.RequestEvent) error {
	email := e.Auth.Email()
	notify.SendEmail(h.app, email,
		notify.SubjectPrefix()+"Notificación de prueba",
		notify.RenderEmail(h.app, "", "Si ves este email, las notificaciones por correo funcionan correctamente."))
	flash(e, "Email de prueba enviado a "+email)
	return redirectHX(e, "/admin/dev-tools")
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
func (h *AdminDevToolsHandler) Reset(e *core.RequestEvent) error {
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
