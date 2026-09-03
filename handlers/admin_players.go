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
		"PageTitle":      "Jugadores",
		"Players":        players,
		"PendingInvites": len(pendingInvites),
		"Mode":           AdminSummary,
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

	gender := e.Request.FormValue("gender")
	if gender != "male" && gender != "female" {
		return alertError(e, "El género es obligatorio")
	}

	user.Set("display_name", displayName)
	user.Set("gender", gender)
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

	gender := e.Request.FormValue("gender")
	if gender != "male" && gender != "female" {
		return alertError(e, "El género es obligatorio")
	}

	user := core.NewRecord(collection)
	user.Set("email", email)
	user.Set("display_name", displayName)
	user.Set("gender", gender)
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

	notify.SendEmail(h.app, email, "Bienvenido a Dale Fuerte a la Bola",
		buildOnboardingEmail(email, resetURL))

	name := displayName
	if name == "" {
		name = email
	}
	return renderResetLinkPanel(e, name, resetURL, true)
}

// RegenerateLink reissues a password-reset token for an existing player.
func (h *AdminPlayerHandler) RegenerateLink(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	user, err := h.app.FindRecordById("users", id)
	if err != nil {
		return alertError(e, "Usuario no encontrado")
	}

	resetToken, err := user.NewPasswordResetToken()
	if err != nil {
		slog.Error("generate password reset token failed", "user", user.Id, "err", err)
		return alertError(e, "No se pudo generar enlace de contraseña")
	}

	resetURL := buildResetURL(e, resetToken)
	name := user.GetString("display_name")
	if name == "" {
		name = user.GetString("email")
	}
	return renderResetLinkPanel(e, name, resetURL, false)
}

func renderResetLinkPanel(e *core.RequestEvent, name, resetURL string, isNew bool) error {
	title := "Enlace regenerado"
	if isNew {
		title = "Usuario creado"
	}
	uid := fmt.Sprintf("pwd-link-%s", security.RandomString(6))
	return e.HTML(http.StatusOK, fmt.Sprintf(`<div class="card bg-base-100 shadow-sm border border-base-300 p-4">
	<h3 class="font-bold text-lg mb-2">%s</h3>
	<p class="text-sm">%s</p>
	<p class="text-sm mt-2 opacity-60">Enlace para establecer contraseña:</p>
	<div class="flex gap-2 mt-2">
		<input type="text" value="%s" class="input input-bordered input-sm flex-1" readonly id="%s">
		<button onclick="navigator.clipboard.writeText(document.getElementById('%s').value).then(function(){this.textContent='Copiado!'}.bind(this))" class="btn btn-sm btn-ghost">Copiar</button>
	</div>
	<div class="mt-4">
		<a href="/admin/players" class="link link-hover text-sm">&larr; Volver a jugadores</a>
	</div>
</div>`, html.EscapeString(title), html.EscapeString(name), html.EscapeString(resetURL), uid, uid))
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
	// Token in URL is inherent: the reset link requires it; admin-only copy-paste, not logged.
	return requestBaseURL(e) + "/reset-password?token=" + token
}

func buildOnboardingEmail(email, resetURL string) string {
	return fmt.Sprintf(`<h2>Bienvenido a Dale Fuerte a la Bola</h2>
<p>Se ha creado una cuenta para <strong>%s</strong>.</p>
<p>Establece tu contraseña para acceder:</p>
<p><a href="%s">Establecer contraseña</a></p>
<p>— Dale Fuerte a la Bola</p>`, html.EscapeString(email), html.EscapeString(resetURL))
}

func buildInviteEmail(registerURL string) string {
	return fmt.Sprintf(`<h2>Dale Fuerte a la Bola</h2>
<p>Has sido invitado a unirte a Dale Fuerte a la Bola.</p>
<p>Regístrate con el siguiente enlace:</p>
<p><a href="%s">Registrarse</a></p>
<p>— Dale Fuerte a la Bola</p>`, html.EscapeString(registerURL))
}
