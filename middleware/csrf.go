package middleware

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// RejectCrossSitePOST blocks POST requests that originate from a
// different site. Uses Sec-Fetch-Site (sent by modern browsers) with
// Origin/Referer as fallback. This is a defense-in-depth layer on top
// of SameSite=Lax cookies.
func RejectCrossSitePOST(e *core.RequestEvent) error {
	if e.Request.Method != http.MethodPost {
		return e.Next()
	}

	// PocketBase API and dashboard handle their own CSRF
	path := e.Request.URL.Path
	if strings.HasPrefix(path, "/_/") || strings.HasPrefix(path, "/api/") {
		return e.Next()
	}

	fetchSite := e.Request.Header.Get("Sec-Fetch-Site")
	if fetchSite != "" {
		if fetchSite == "cross-site" {
			return e.JSON(http.StatusForbidden, map[string]string{"error": "cross-site request blocked"})
		}
		return e.Next()
	}

	// Fallback for browsers that don't send Sec-Fetch-Site
	origin := e.Request.Header.Get("Origin")
	if origin == "" {
		origin = e.Request.Header.Get("Referer")
	}
	if origin == "" {
		return e.Next()
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return e.Next()
	}
	if parsed.Host != "" && parsed.Host != e.Request.Host {
		return e.JSON(http.StatusForbidden, map[string]string{"error": "cross-site request blocked"})
	}
	return e.Next()
}
