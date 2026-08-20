package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/security"
)

func (h *AdminHandler) Players(e *core.RequestEvent) error {
	players, _ := h.app.FindRecordsByFilter("users",
		"id != ''", "display_name", 0, 0, nil)

	pendingInvites, _ := h.app.FindRecordsByFilter("invitations",
		"status = 'pending'", "", 0, 0, nil)

	return h.renderPage(e, "admin/players.html", map[string]any{
		"Players":        players,
		"PendingInvites": len(pendingInvites),
	})
}

func (h *AdminHandler) PlayerUpdate(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	user, err := h.app.FindRecordById("users", id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Usuario no encontrado</div>`)
	}

	displayName := e.Request.FormValue("display_name")
	role := e.Request.FormValue("role")

	if role != "admin" && role != "player" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Rol inválido</div>`)
	}

	user.Set("display_name", displayName)
	user.Set("role", role)

	if err := h.app.Save(user); err != nil {
		return e.HTML(http.StatusOK, fmt.Sprintf(`<div class="alert alert-error">Error: %s</div>`, err.Error()))
	}

	e.Response.Header().Set("HX-Redirect", "/admin/players")
	return e.NoContent(http.StatusNoContent)
}

func (h *AdminHandler) PlayerPreCreate(e *core.RequestEvent) error {
	email := e.Request.FormValue("email")
	displayName := e.Request.FormValue("display_name")

	if email == "" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">El email es obligatorio</div>`)
	}

	collection, err := h.app.FindCollectionByNameOrId("users")
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error interno</div>`)
	}

	tempPassword := security.RandomString(16)

	user := core.NewRecord(collection)
	user.Set("email", email)
	user.Set("display_name", displayName)
	user.Set("role", "player")
	user.SetPassword(tempPassword)

	if err := h.app.Save(user); err != nil {
		return e.HTML(http.StatusOK, fmt.Sprintf(`<div class="alert alert-error">Error al crear usuario: %s</div>`, err.Error()))
	}

	inviteToken, err := generateInviteToken()
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al generar invitación</div>`)
	}

	invCol, err := h.app.FindCollectionByNameOrId("invitations")
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error interno</div>`)
	}

	invite := core.NewRecord(invCol)
	invite.Set("token", inviteToken)
	invite.Set("email", email)
	invite.Set("created_by", e.Auth.Id)
	invite.Set("expires_at", time.Now().Add(7*24*time.Hour).UTC().Format("2006-01-02 15:04:05.000Z"))
	invite.Set("status", "pending")

	if err := h.app.Save(invite); err != nil {
		return e.HTML(http.StatusOK, fmt.Sprintf(`<div class="alert alert-error">Error al crear invitación: %s</div>`, err.Error()))
	}

	e.Response.Header().Set("HX-Redirect", "/admin/players")
	return e.NoContent(http.StatusNoContent)
}
