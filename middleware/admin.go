// Package middleware provides HTTP middleware for authentication and authorization.
package middleware

import (
	"slices"

	"github.com/pocketbase/pocketbase/core"
)

// RequireAppAdmin redirects non-admin users to the home page, handling both
// regular and HTMX requests (see redirectOrHX).
func RequireAppAdmin(e *core.RequestEvent) error {
	if e.Auth == nil || !slices.Contains(e.Auth.GetStringSlice("roles"), "admin") {
		return redirectOrHX(e, "/")
	}
	return e.Next()
}
