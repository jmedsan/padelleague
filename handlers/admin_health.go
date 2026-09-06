package handlers

import (
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/hooks"
	"padelleague/league"
	"padelleague/notify"
)

// AdminHealthHandler serves the consolidated league health dashboard.
type AdminHealthHandler struct {
	app        core.App
	notifier   *notify.Notifier
	renderPage RenderFunc
}

// NewAdminHealthHandler creates an AdminHealthHandler.
func NewAdminHealthHandler(app core.App, notifier *notify.Notifier, renderPage RenderFunc) *AdminHealthHandler {
	return &AdminHealthHandler{app: app, notifier: notifier, renderPage: renderPage}
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
		"BackupEnabled": hooks.BackupEnabled(),
	})
}

// BackupNow triggers an immediate Google Drive backup.
func (h *AdminHealthHandler) BackupNow(e *core.RequestEvent) error {
	if err := hooks.RunBackupNow(); err != nil {
		return alertError(e, "Backup no configurado")
	}
	flash(e, "Backup iniciado")
	return redirectHX(e, "/admin/health")
}

// TestPush sends a test notification through the real pipeline to the current admin.
func (h *AdminHealthHandler) TestPush(e *core.RequestEvent) error {
	h.notifier.NotifyPlayers([]string{e.Auth.Id}, league.Notification{
		Type:  "general",
		Title: "Notificación de prueba",
		Body:  "Si ves esto, las notificaciones funcionan correctamente.",
		Link:  "/admin/health",
	})
	flash(e, "Notificación de prueba enviada")
	return redirectHX(e, "/admin/health")
}

// TestEmail sends a test email to the current admin user.
func (h *AdminHealthHandler) TestEmail(e *core.RequestEvent) error {
	email := e.Auth.Email()
	notify.SendEmail(h.app, email,
		notify.SubjectPrefix()+"Notificación de prueba",
		notify.RenderEmail(h.app, "", "Si ves este email, las notificaciones por correo funcionan correctamente."))
	flash(e, "Email de prueba enviado a "+email)
	return redirectHX(e, "/admin/health")
}
