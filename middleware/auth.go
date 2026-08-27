package middleware

import (
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// CookieAuth copies the pb_auth cookie into the Authorization header for PocketBase.
func CookieAuth(e *core.RequestEvent) error {
	if strings.HasPrefix(e.Request.URL.Path, "/_/") || strings.HasPrefix(e.Request.URL.Path, "/api/") {
		return e.Next()
	}
	cookie, err := e.Request.Cookie("pb_auth")
	if err == nil && cookie.Value != "" {
		e.Request.Header.Set("Authorization", cookie.Value)
	}
	return e.Next()
}

// SetAuthCookie writes the pb_auth cookie with the given token.
func SetAuthCookie(e *core.RequestEvent, token string) {
	http.SetCookie(e.Response, &http.Cookie{
		Name:     "pb_auth",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// RequireAuth redirects unauthenticated users to /login and incomplete
// profiles to /profile/complete, handling both regular and HTMX requests.
func RequireAuth(e *core.RequestEvent) error {
	if e.Auth == nil {
		if e.Request.Header.Get("HX-Request") == "true" {
			e.Response.Header().Set("HX-Redirect", "/login")
			return e.NoContent(http.StatusNoContent)
		}
		return e.Redirect(http.StatusFound, "/login")
	}
	if e.Auth.GetString("display_name") == "" &&
		e.Request.URL.Path != "/profile/complete" {
		if e.Request.Header.Get("HX-Request") == "true" {
			e.Response.Header().Set("HX-Redirect", "/profile/complete")
			return e.NoContent(http.StatusNoContent)
		}
		return e.Redirect(http.StatusFound, "/profile/complete")
	}
	return e.Next()
}

// ClearAuthCookie removes the pb_auth cookie.
func ClearAuthCookie(e *core.RequestEvent) {
	http.SetCookie(e.Response, &http.Cookie{
		Name:     "pb_auth",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}
