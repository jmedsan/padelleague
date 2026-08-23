package handlers

import (
	"fmt"
	"log/slog"
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
		return alertError(e, "Usuario no encontrado")
	}

	displayName := e.Request.FormValue("display_name")
	role := e.Request.FormValue("role")

	if role != "admin" && role != "player" {
		return alertError(e, "Rol inválido")
	}

	user.Set("display_name", displayName)
	user.Set("role", role)

	if err := h.app.Save(user); err != nil {
		slog.Error("save player failed", "err", err)
		return alertError(e, "Error al guardar el jugador")
	}

	return redirectHX(e, "/admin/players")
}

func (h *AdminHandler) PlayerPreCreate(e *core.RequestEvent) error {
	email := e.Request.FormValue("email")
	displayName := e.Request.FormValue("display_name")

	if email == "" {
		return alertError(e, "El email es obligatorio")
	}

	collection, err := h.app.FindCollectionByNameOrId("users")
	if err != nil {
		return alertError(e, "Error interno")
	}

	tempPassword := security.RandomString(16)

	user := core.NewRecord(collection)
	user.Set("email", email)
	user.Set("display_name", displayName)
	user.Set("role", "player")
	user.SetPassword(tempPassword)

	if err := h.app.Save(user); err != nil {
		slog.Error("create player failed", "err", err)
		return alertError(e, "Error al crear usuario")
	}

	inviteToken, err := generateInviteToken()
	if err != nil {
		return alertError(e, "Error al generar invitación")
	}

	invCol, err := h.app.FindCollectionByNameOrId("invitations")
	if err != nil {
		return alertError(e, "Error interno")
	}

	invite := core.NewRecord(invCol)
	invite.Set("token", inviteToken)
	invite.Set("email", email)
	invite.Set("created_by", e.Auth.Id)
	invite.Set("expires_at", time.Now().Add(2*24*time.Hour).UTC().Format("2006-01-02 15:04:05.000Z"))
	invite.Set("status", "pending")

	if err := h.app.Save(invite); err != nil {
		slog.Error("create invitation failed", "err", err)
		return alertError(e, "Error al crear invitación")
	}

	resetToken, err := user.NewPasswordResetToken()
	if err != nil {
		slog.Error("generate password reset token failed", "user", user.Id, "err", err)
		return alertWarning(e, "Usuario creado pero no se pudo generar enlace de contraseña")
	}

	scheme := e.Request.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "https"
		if e.Request.TLS == nil {
			scheme = "http"
		}
	}
	resetURL := fmt.Sprintf("%s://%s/reset-password?token=%s", scheme, e.Request.Host, resetToken)

	return e.HTML(http.StatusOK, fmt.Sprintf(`<div class="alert alert-success">
	<div class="w-full">
		<p class="font-medium">Usuario creado: %s</p>
		<p class="text-sm mt-1">Enlace para establecer contraseña:</p>
		<div class="flex gap-2 mt-2">
			<input type="text" value="%s" class="input input-bordered input-sm flex-1" readonly id="pwd-link">
			<button onclick="navigator.clipboard.writeText(document.getElementById('pwd-link').value).then(function(){this.textContent='Copiado!';setTimeout(function(){document.getElementById('copy-btn').textContent='Copiar'},2000)}.bind(this))" class="btn btn-sm btn-ghost" id="copy-btn">Copiar</button>
		</div>
	</div>
</div>`, email, resetURL))
}
