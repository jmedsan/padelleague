package handlers

import (
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
		"PageTitle":  "Salud",
		"Categories": categories,
		"AllEmpty":   allEmpty,
	})
}
