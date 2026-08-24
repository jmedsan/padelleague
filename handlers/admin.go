// Package handlers implements HTTP handlers for all application routes.
package handlers

import (
	"github.com/pocketbase/pocketbase/core"

	"padelleague/notify"
)

// AdminHandler handles admin pages for players, pairs, invitations, venues, and disputes.
type AdminHandler struct {
	app        core.App
	notifier   *notify.Notifier
	renderPage func(e *core.RequestEvent, page string, data map[string]any) error
}

// NewAdminHandler creates an AdminHandler with the given dependencies.
func NewAdminHandler(app core.App, notifier *notify.Notifier, renderPage func(e *core.RequestEvent, page string, data map[string]any) error) *AdminHandler {
	return &AdminHandler{app: app, notifier: notifier, renderPage: renderPage}
}
