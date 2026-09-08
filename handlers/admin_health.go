package handlers

import (
	"context"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
)

// AdminHealthHandler serves the consolidated league health dashboard.
type AdminHealthHandler struct {
	app        core.App
	renderPage RenderFunc
}

// NewAdminHealthHandler creates an AdminHealthHandler.
func NewAdminHealthHandler(app core.App, renderPage RenderFunc) *AdminHealthHandler {
	return &AdminHealthHandler{app: app, renderPage: renderPage}
}

// Health renders the admin health dashboard.
func (h *AdminHealthHandler) Health(e *core.RequestEvent) error {
	categories := league.HealthReport(h.app, time.Now())
	allEmpty := true
	for _, cat := range categories {
		if len(cat.Items) > 0 {
			allEmpty = false
			break
		}
	}
	return h.renderPage(e, "admin/health.html", map[string]any{
		"PageTitle":     "Salud",
		"Categories":    categories,
		"AllEmpty":      allEmpty,
		"BackupEnabled": h.app.Settings().Backups.Cron != "",
	})
}

// BackupNow triggers an immediate backup. PocketBase's OnBackupCreate hook
// (registered in hooks.registerBackup) emails a copy once it completes.
func (h *AdminHealthHandler) BackupNow(e *core.RequestEvent) error {
	if h.app.Settings().Backups.Cron == "" {
		return alertError(e, "Backup no configurado")
	}
	if err := h.app.CreateBackup(context.Background(), ""); err != nil {
		return alertError(e, "Error al iniciar el backup")
	}
	flash(e, "Backup iniciado")
	return redirectHX(e, "/admin/health")
}
