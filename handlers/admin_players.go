package handlers

import (
	"github.com/pocketbase/pocketbase/core"
)

func (h *AdminHandler) Players(e *core.RequestEvent) error {
	players, _ := h.app.FindRecordsByFilter("users",
		"role = 'player'", "display_name", 0, 0, nil)

	return h.renderPage(e, "admin/players.html", map[string]any{
		"Players": players,
	})
}
