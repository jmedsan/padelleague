package handlers

import (
	"github.com/pocketbase/pocketbase/core"

	"padelleague/notify"
)

type AdminHandler struct {
	app        core.App
	notifier   *notify.Notifier
	renderPage func(e *core.RequestEvent, page string, data map[string]any) error
}

func NewAdminHandler(app core.App, notifier *notify.Notifier, renderPage func(e *core.RequestEvent, page string, data map[string]any) error) *AdminHandler {
	return &AdminHandler{app: app, notifier: notifier, renderPage: renderPage}
}
