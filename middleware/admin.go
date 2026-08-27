// Package middleware provides HTTP middleware for authentication and authorization.
package middleware

import (
	"net/http"
	"slices"

	"github.com/pocketbase/pocketbase/core"
)

// RequireAppAdmin redirects non-admin users to the home page.
func RequireAppAdmin(e *core.RequestEvent) error {
	if e.Auth == nil || !slices.Contains(e.Auth.GetStringSlice("roles"), "admin") {
		return e.Redirect(http.StatusFound, "/")
	}
	return e.Next()
}
