package handlers

import (
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
)

// Outstanding renders every non-final match across active competitions,
// ordered most-urgent first.
func (h *InvitationHandler) Outstanding(e *core.RequestEvent) error {
	matches := league.OutstandingMatches(h.app, time.Now())

	return h.renderPage(e, "admin/outstanding.html", map[string]any{
		"Matches": matches,
	})
}
