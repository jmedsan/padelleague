package middleware

import (
	"net/http"
	"net/url"
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
		return redirectOrHX(e, loginURL(e.Request))
	}
	if e.Auth.GetString("display_name") == "" &&
		e.Request.URL.Path != "/profile/complete" {
		return redirectOrHX(e, "/profile/complete")
	}
	return e.Next()
}

// loginURL is the login page, carrying the requested page as ?next= so the
// user lands there after signing in. Only a plain GET page load is carried:
// an HTMX fragment or a non-GET request is not a page the user can return to,
// and "/" is where login lands anyway.
func loginURL(r *http.Request) string {
	if r.Method != http.MethodGet || r.URL.Path == "/" || r.Header.Get("HX-Request") == "true" {
		return "/login"
	}
	return "/login?next=" + url.QueryEscape(r.URL.RequestURI())
}

// SafeNext returns next when it is a path on this site, else "/". It rejects
// absolute URLs, "//host" and backslash forms, which would redirect off-site.
func SafeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.Contains(next, "\\") {
		return "/"
	}
	return next
}

// redirectOrHX redirects e to url: a plain 302 for a regular request, or a
// 204 + HX-Redirect for an HTMX one — htmx follows a 302 transparently and
// swaps the target page's HTML into the current fragment slot instead of
// navigating, so every guard that can reject an HTMX request must answer
// this way instead of a bare e.Redirect.
func redirectOrHX(e *core.RequestEvent, url string) error {
	if e.Request.Header.Get("HX-Request") == "true" {
		e.Response.Header().Set("HX-Redirect", url)
		return e.NoContent(http.StatusNoContent)
	}
	return e.Redirect(http.StatusFound, url)
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
