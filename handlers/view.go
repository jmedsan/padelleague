package handlers

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

// ViewHandler switches the admin/player view for users who hold both roles.
// The view is a cosmetic preference (which nav and banner to show) stored in a
// cookie; it does not change authorization.
type ViewHandler struct{}

// NewViewHandler creates a ViewHandler.
func NewViewHandler() *ViewHandler {
	return &ViewHandler{}
}

// Switch stores the requested view mode in the view_as cookie and returns to
// the page the user came from.
func (h *ViewHandler) Switch(e *core.RequestEvent) error {
	mode := e.Request.PathValue("mode")
	if mode != "admin" && mode != "player" {
		mode = "admin"
	}
	http.SetCookie(e.Response, &http.Cookie{
		Name:     "view_as",
		Value:    mode,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	dest := e.Request.Header.Get("Referer")
	if dest == "" {
		dest = "/"
	}
	return e.Redirect(http.StatusFound, dest)
}
