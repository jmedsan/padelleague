package handlers

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// ViewHandler switches the admin/player view for users who hold both roles.
// The view is stored in a cookie and gates authorization too: admin pages and
// admin actions require the admin view (middleware.RequireAppAdmin), so an
// admin in player view is a pure player.
type ViewHandler struct{}

// NewViewHandler creates a ViewHandler.
func NewViewHandler() *ViewHandler {
	return &ViewHandler{}
}

// Switch stores the requested view mode in the view_as cookie and returns to
// the page the user came from, unless that page is an admin page and the new
// view is the player view (the admin pages refuse it).
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
	dest := sameSiteReferer(e)
	if mode == "player" && (dest == "/admin" || strings.HasPrefix(dest, "/admin/")) {
		dest = "/"
	}
	return e.Redirect(http.StatusFound, dest)
}

// sameSiteReferer returns the path and query of the Referer when it points to
// this site, and "/" otherwise. Browsers send an absolute Referer; a bare path
// is accepted too. Any other host is refused (open redirect).
func sameSiteReferer(e *core.RequestEvent) string {
	ref, err := url.Parse(e.Request.Header.Get("Referer"))
	if err != nil {
		return "/"
	}
	if (ref.Scheme != "" || ref.Host != "") && ref.Host != e.Request.Host {
		return "/"
	}
	dest := ref.EscapedPath()
	if !strings.HasPrefix(dest, "/") || strings.HasPrefix(dest, "//") {
		return "/"
	}
	if ref.RawQuery != "" {
		dest += "?" + ref.RawQuery
	}
	return dest
}
