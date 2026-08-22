package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/middleware"
)

type AuthHandler struct {
	app        core.App
	renderPage func(e *core.RequestEvent, page string, data map[string]any) error
}

func NewAuthHandler(app core.App, renderPage func(e *core.RequestEvent, page string, data map[string]any) error) *AuthHandler {
	return &AuthHandler{app: app, renderPage: renderPage}
}

func (h *AuthHandler) Login(e *core.RequestEvent) error {
	if e.Auth != nil {
		return e.Redirect(http.StatusFound, "/")
	}
	return h.renderPage(e, "login.html", map[string]any{})
}

func (h *AuthHandler) LoginSubmit(e *core.RequestEvent) error {
	email := e.Request.FormValue("email")
	password := e.Request.FormValue("password")

	record, err := h.app.FindAuthRecordByEmail("users", email)
	if err != nil || !record.ValidatePassword(password) {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Email o contraseña incorrectos</div>`)
	}

	token, err := record.NewAuthToken()
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al generar sesión</div>`)
	}

	middleware.SetAuthCookie(e, token)

	if e.Request.Header.Get("HX-Request") == "true" {
		e.Response.Header().Set("HX-Redirect", "/")
		return e.NoContent(http.StatusNoContent)
	}
	return e.Redirect(http.StatusFound, "/")
}

func (h *AuthHandler) Register(e *core.RequestEvent) error {
	token := e.Request.URL.Query().Get("token")
	if token == "" {
		return h.renderPage(e, "register.html", map[string]any{
			"NoInvite": true,
		})
	}

	invites, err := h.app.FindRecordsByFilter("invitations",
		"token = {:token}",
		"", 1, 0,
		map[string]any{"token": token})
	if err != nil || len(invites) == 0 || isInviteExpired(invites[0]) {
		return h.renderPage(e, "register.html", map[string]any{
			"InvalidInvite": true,
		})
	}
	invite := invites[0]

	maxUses := int(invite.GetFloat("max_uses"))
	if maxUses < 1 {
		maxUses = 1
	}
	useCount := int(invite.GetFloat("use_count"))
	if useCount >= maxUses {
		return h.renderPage(e, "register.html", map[string]any{
			"InvalidInvite": true,
		})
	}

	return h.renderPage(e, "register.html", map[string]any{
		"Token":       token,
		"InviteEmail": invite.GetString("email"),
	})
}

func (h *AuthHandler) RegisterSubmit(e *core.RequestEvent) error {
	token := e.Request.FormValue("token")
	displayName := e.Request.FormValue("display_name")
	email := e.Request.FormValue("email")
	password := e.Request.FormValue("password")
	passwordConfirm := e.Request.FormValue("password_confirm")

	if token == "" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Invitación requerida</div>`)
	}

	if password != passwordConfirm {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Las contraseñas no coinciden</div>`)
	}

	invites, err := h.app.FindRecordsByFilter("invitations",
		"token = {:token}",
		"", 1, 0,
		map[string]any{"token": token})
	if err != nil || len(invites) == 0 || isInviteExpired(invites[0]) {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Invitación inválida o expirada</div>`)
	}
	invite := invites[0]

	maxUses := int(invite.GetFloat("max_uses"))
	if maxUses < 1 {
		maxUses = 1
	}
	useCount := int(invite.GetFloat("use_count"))
	if useCount >= maxUses {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Invitación agotada</div>`)
	}

	inviteEmail := invite.GetString("email")
	if inviteEmail != "" && !strings.EqualFold(email, inviteEmail) {
		return e.HTML(http.StatusOK, fmt.Sprintf(`<div class="alert alert-error">Debes usar el email %s</div>`, inviteEmail))
	}

	var userRecord *core.Record
	var authToken string

	err = h.app.RunInTransaction(func(txApp core.App) error {
		collection, err := txApp.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		userRecord = core.NewRecord(collection)
		userRecord.Set("email", email)
		userRecord.Set("display_name", displayName)
		userRecord.Set("role", "player")
		userRecord.SetPassword(password)

		if err := txApp.Save(userRecord); err != nil {
			return err
		}

		newCount := int(invite.GetFloat("use_count")) + 1
		invite.Set("use_count", newCount)
		invite.Set("used_by", userRecord.Id)
		invite.Set("used_at", time.Now().UTC().Format("2006-01-02 15:04:05.000Z"))
		invMaxUses := int(invite.GetFloat("max_uses"))
		if invMaxUses < 1 {
			invMaxUses = 1
		}
		if newCount >= invMaxUses {
			invite.Set("status", "used")
		}
		if err := txApp.Save(invite); err != nil {
			return err
		}

		authToken, err = userRecord.NewAuthToken()
		return err
	})

	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al crear la cuenta. Verifica los datos e intenta de nuevo.</div>`)
	}

	middleware.SetAuthCookie(e, authToken)

	if e.Request.Header.Get("HX-Request") == "true" {
		e.Response.Header().Set("HX-Redirect", "/")
		return e.NoContent(http.StatusNoContent)
	}
	return e.Redirect(http.StatusFound, "/")
}

func (h *AuthHandler) ProfileComplete(e *core.RequestEvent) error {
	return h.renderPage(e, "profile-complete.html", map[string]any{})
}

func (h *AuthHandler) ProfileCompleteSubmit(e *core.RequestEvent) error {
	displayName := strings.TrimSpace(e.Request.FormValue("display_name"))
	if displayName == "" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">El nombre es obligatorio</div>`)
	}

	e.Auth.Set("display_name", displayName)
	if err := h.app.Save(e.Auth); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al guardar el perfil</div>`)
	}

	if e.Request.Header.Get("HX-Request") == "true" {
		e.Response.Header().Set("HX-Redirect", "/")
		return e.NoContent(http.StatusNoContent)
	}
	return e.Redirect(http.StatusFound, "/")
}

func (h *AuthHandler) Logout(e *core.RequestEvent) error {
	middleware.ClearAuthCookie(e)

	if e.Request.Header.Get("HX-Request") == "true" {
		e.Response.Header().Set("HX-Redirect", "/login")
		return e.NoContent(http.StatusNoContent)
	}
	return e.Redirect(http.StatusFound, "/login")
}
