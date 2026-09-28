// Package middleware provides HTTP middleware for authentication and authorization.
package middleware

import (
	"github.com/pocketbase/pocketbase/core"

	"padelleague/render"
)

// RequireAppAdmin redirects to the home page every user who is not an admin in
// the admin view: a non-admin, or an admin who switched to the player view (an
// admin in player view is a pure player). Handles both regular and HTMX
// requests (see redirectOrHX).
func RequireAppAdmin(e *core.RequestEvent) error {
	if e.Auth == nil || !render.AdminView(e) {
		return redirectOrHX(e, "/")
	}
	return e.Next()
}
