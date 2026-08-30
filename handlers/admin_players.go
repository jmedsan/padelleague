package handlers

import (
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/security"

	"padelleague/notify"
)

// AdminPlayerHandler handles admin player management.
type AdminPlayerHandler struct {
	app        core.App
	renderPage RenderFunc
}

// NewAdminPlayerHandler creates an AdminPlayerHandler with the given dependencies.
func NewAdminPlayerHandler(app core.App, renderPage RenderFunc) *AdminPlayerHandler {
	return &AdminPlayerHandler{app: app, renderPage: renderPage}
}

// Players renders the admin players management page.
func (h *AdminPlayerHandler) Players(e *core.RequestEvent) error {
	players, _ := h.app.FindRecordsByFilter("users",
		"id != ''", "display_name", 0, 0, nil)

	pendingInvites, _ := h.app.FindRecordsByFilter("invitations",
		"status = 'pending'", "", 0, 0, nil)

	return h.renderPage(e, "admin/players.html", map[string]any{
		"Players":        players,
		"PendingInvites": len(pendingInvites),
	})
}

// PlayerUpdate handles POST to update a player's display name or role.
func (h *AdminPlayerHandler) PlayerUpdate(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	user, err := h.app.FindRecordById("users", id)
	if err != nil {
		return alertError(e, "Usuario no encontrado")
	}

	displayName := e.Request.FormValue("display_name")
	roles := e.Request.Form["roles"]

	for _, r := range roles {
		if r != "admin" && r != "player" {
			return alertError(e, "Rol inválido")
		}
	}
	if len(roles) == 0 {
		roles = []string{"player"}
	}

	user.Set("display_name", displayName)
	user.Set("roles", roles)

	if err := h.app.Save(user); err != nil {
		slog.Error("save player failed", "err", err)
		return alertError(e, "Error al guardar el jugador")
	}

	return redirectHX(e, "/admin/players")
}

// PlayerPreCreate creates a placeholder user account for a player not yet registered.
func (h *AdminPlayerHandler) PlayerPreCreate(e *core.RequestEvent) error {
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
	user.Set("roles", []string{"player"})
	user.SetPassword(tempPassword)

	if err := h.app.Save(user); err != nil {
		slog.Error("create player failed", "err", err)
		return alertError(e, "Error al crear usuario")
	}

	if err := h.createPlayerInvitation(email, e.Auth.Id); err != nil {
		return alertError(e, "Error al crear invitación")
	}

	resetToken, err := user.NewPasswordResetToken()
	if err != nil {
		slog.Error("generate password reset token failed", "user", user.Id, "err", err)
		return alertWarning(e, "Usuario creado pero no se pudo generar enlace de contraseña")
	}

	resetURL := buildResetURL(e, resetToken)

	notify.SendEmail(h.app, email, "Bienvenido a PadelLeague",
		buildOnboardingEmail(email, resetURL))

	return e.HTML(http.StatusOK, fmt.Sprintf(`<div class="alert alert-success">
	<div class="w-full">
		<p class="font-medium">Usuario creado: %s</p>
		<p class="text-sm mt-1">Enlace para establecer contraseña:</p>
		<div class="flex gap-2 mt-2">
			<input type="text" value="%s" class="input input-bordered input-sm flex-1" readonly id="pwd-link">
			<button onclick="navigator.clipboard.writeText(document.getElementById('pwd-link').value).then(function(){this.textContent='Copiado!';setTimeout(function(){document.getElementById('copy-btn').textContent='Copiar'},2000)}.bind(this))" class="btn btn-sm btn-ghost" id="copy-btn">Copiar</button>
		</div>
	</div>
</div>`, html.EscapeString(email), html.EscapeString(resetURL)))
}

func (h *AdminPlayerHandler) createPlayerInvitation(email, createdBy string) error {
	inviteToken, err := generateInviteToken()
	if err != nil {
		return fmt.Errorf("generate invite: %w", err)
	}
	invCol, err := h.app.FindCollectionByNameOrId("invitations")
	if err != nil {
		return fmt.Errorf("invitations collection: %w", err)
	}
	invite := core.NewRecord(invCol)
	invite.Set("token", inviteToken)
	invite.Set("email", email)
	invite.Set("created_by", createdBy)
	invite.Set("expires_at", time.Now().Add(2*24*time.Hour).UTC().Format("2006-01-02 15:04:05.000Z"))
	invite.Set("status", "pending")
	if err := h.app.Save(invite); err != nil {
		slog.Error("create invitation failed", "err", err)
		return fmt.Errorf("save invitation: %w", err)
	}
	return nil
}

func requestBaseURL(e *core.RequestEvent) string {
	scheme := e.Request.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "https"
		if e.Request.TLS == nil {
			scheme = "http"
		}
	}
	return fmt.Sprintf("%s://%s", scheme, e.Request.Host)
}

func buildResetURL(e *core.RequestEvent, token string) string {
	return requestBaseURL(e) + "/reset-password?token=" + token
}

func buildOnboardingEmail(email, resetURL string) string {
	return fmt.Sprintf(`<h2>Bienvenido a PadelLeague</h2>
<p>Se ha creado una cuenta para <strong>%s</strong>.</p>
<p>Establece tu contraseña para acceder:</p>
<p><a href="%s">Establecer contraseña</a></p>
<p>— PadelLeague</p>`, html.EscapeString(email), html.EscapeString(resetURL))
}

func buildInviteEmail(registerURL string) string {
	return fmt.Sprintf(`<h2>PadelLeague</h2>
<p>Has sido invitado a unirte a PadelLeague.</p>
<p>Regístrate con el siguiente enlace:</p>
<p><a href="%s">Registrarse</a></p>
<p>— PadelLeague</p>`, html.EscapeString(registerURL))
}
