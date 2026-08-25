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
	renderPage RenderFunc
}

// NewAdminHandler creates an AdminHandler with the given dependencies.
func NewAdminHandler(app core.App, notifier *notify.Notifier, renderPage RenderFunc) *AdminHandler {
	return &AdminHandler{app: app, notifier: notifier, renderPage: renderPage}
}
