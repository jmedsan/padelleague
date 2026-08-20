package middleware

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

func RequireAppAdmin(e *core.RequestEvent) error {
	if e.Auth == nil || e.Auth.GetString("role") != "admin" {
		return e.Redirect(http.StatusFound, "/")
	}
	return e.Next()
}
