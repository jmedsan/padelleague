package handlers

import (
	"fmt"
	"html"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
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

// Settings renders the admin settings page: the global defaults form always,
// the DB-reset card only when dev tools are enabled.
func (h *AdminSettingsHandler) Settings(e *core.RequestEvent) error {
	return h.renderPage(e, "admin/settings.html", map[string]any{
		"PageTitle": "Configuración",
		"DevMode":   h.devTools,
		"Settings":  league.LoadSettings(h.app),
	})
}

// SaveDefaults handles POST to update the global app_settings singleton
// that seeds new competitions' default values.
func (h *AdminSettingsHandler) SaveDefaults(e *core.RequestEvent) error {
	records, err := h.app.FindRecordsByFilter("app_settings", "", "", 1, 0, nil)
	if err != nil || len(records) == 0 {
		return alertError(e, "No se encontró la configuración")
	}
	rec := records[0]

	fields, errMsg := parseSettingsForm(e)
	if errMsg != "" {
		return alertError(e, errMsg)
	}

	rec.Set("quorum_timeout_hours", fields.quorum)
	rec.Set("arrange_grace_days", fields.grace)
	rec.Set("walkover_score", fields.walkover)
	rec.Set("default_penalty", fields.penalty)
	rec.Set("recovery_days", fields.recovery)
	rec.Set("play_twice", e.Request.FormValue("play_twice") == "on")
	rec.Set("gender_type", e.Request.FormValue("gender_type"))
	rec.Set("invite_max_uses", fields.maxUses)
	rec.Set("invite_expiration_days", fields.expDays)

	if err := h.app.Save(rec); err != nil {
		slog.Error("save app settings", "error", err)
		return alertError(e, "Error al guardar la configuración")
	}

	league.InvalidateSettingsCache()

	return alertSuccess(e, "Configuración guardada")
}

// settingsFormFields holds the parsed, validated numeric fields from the
// defaults form.
type settingsFormFields struct {
	quorum, grace, penalty, recovery, maxUses, expDays int
	walkover                                           string
}

// parseSettingsForm parses and validates every numeric field on the defaults
// form, returning the first field-specific error message it hits (empty
// string when everything is valid).
func parseSettingsForm(e *core.RequestEvent) (settingsFormFields, string) {
	var f settingsFormFields
	var err error

	if f.quorum, err = parseBoundedInt(e, "quorum_timeout_hours", 0); err != nil {
		return f, "Tiempo de espera debe ser un número entero mayor o igual que 0"
	}
	if f.grace, err = parseBoundedInt(e, "arrange_grace_days", 0); err != nil {
		return f, "Días de gracia debe ser un número entero mayor o igual que 0"
	}
	if f.penalty, err = parseBoundedInt(e, "default_penalty", 0); err != nil {
		return f, "Penalización debe ser un número entero mayor o igual que 0"
	}
	if f.recovery, err = parseBoundedInt(e, "recovery_days", 0); err != nil {
		return f, "Período extra debe ser un número entero mayor o igual que 0"
	}
	if f.maxUses, err = parseBoundedInt(e, "invite_max_uses", 1); err != nil {
		return f, "Usos máximos de invitación debe ser un número entero mayor que 0"
	}
	if f.expDays, err = parseBoundedInt(e, "invite_expiration_days", 1); err != nil {
		return f, "Días de expiración de invitación debe ser un número entero mayor que 0"
	}

	f.walkover = e.Request.FormValue("walkover_score")
	if _, err := league.ParseScore(f.walkover); err != nil {
		return f, "Marcador de incomparecencia no válido"
	}

	return f, ""
}

// parseBoundedInt parses a form field as an integer no smaller than min,
// rejecting non-numeric values instead of silently coercing them.
func parseBoundedInt(e *core.RequestEvent, field string, min int) (int, error) {
	n, err := strconv.Atoi(e.Request.FormValue(field))
	if err != nil {
		return 0, err
	}
	if n < min {
		return 0, fmt.Errorf("must be at least %d, got %d", min, n)
	}
	return n, nil
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
