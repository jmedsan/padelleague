package handlers

import (
	"github.com/pocketbase/pocketbase/core"
)

type AdminHandler struct {
	app        core.App
	renderPage func(e *core.RequestEvent, page string, data map[string]any) error
}

func NewAdminHandler(app core.App, renderPage func(e *core.RequestEvent, page string, data map[string]any) error) *AdminHandler {
	return &AdminHandler{app: app, renderPage: renderPage}
}
