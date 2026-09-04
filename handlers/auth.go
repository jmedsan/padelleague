package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
	"padelleague/middleware"
)

// AuthHandler handles login, registration, and profile completion.
type AuthHandler struct {
	app        core.App
	renderPage RenderFunc
}

// NewAuthHandler creates an AuthHandler with the given dependencies.
func NewAuthHandler(app core.App, renderPage RenderFunc) *AuthHandler {
	return &AuthHandler{app: app, renderPage: renderPage}
}

// Login renders the login page, redirecting authenticated users to home.
func (h *AuthHandler) Login(e *core.RequestEvent) error {
	if e.Auth != nil {
		return e.Redirect(http.StatusFound, "/")
	}
	return h.renderPage(e, "login.html", map[string]any{"PageTitle": "Iniciar sesión"})
}

// LoginSubmit processes the login form and sets the auth cookie on success.
func (h *AuthHandler) LoginSubmit(e *core.RequestEvent) error {
	email := e.Request.FormValue("email")
	password := e.Request.FormValue("password")

	record, err := h.app.FindAuthRecordByEmail("users", email)
	if err != nil || !record.ValidatePassword(password) {
		return alertError(e, "Email o contraseña incorrectos")
	}

	token, err := record.NewAuthToken()
	if err != nil {
		return alertError(e, "Error al generar sesión")
	}

	middleware.SetAuthCookie(e, token)

	if e.Request.Header.Get("HX-Request") == "true" {
		return redirectHX(e, "/")
	}
	return e.Redirect(http.StatusFound, "/")
}

// Register renders the registration form after validating the invitation token.
func (h *AuthHandler) Register(e *core.RequestEvent) error {
	token := e.Request.URL.Query().Get("token")
	if token == "" {
		return h.renderPage(e, "register.html", map[string]any{
			"PageTitle": "Registro",
			"NoInvite":  true,
		})
	}

	invites, err := h.app.FindRecordsByFilter("invitations",
		"token = {:token}",
		"", 1, 0,
		map[string]any{"token": token})
	if err != nil || len(invites) == 0 || isInviteExpired(invites[0]) {
		return h.renderPage(e, "register.html", map[string]any{
			"PageTitle":     "Registro",
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

	data := map[string]any{
		"PageTitle":   "Registro",
		"Token":       token,
		"InviteEmail": invite.GetString("email"),
	}
	if compID := invite.GetString("competition"); compID != "" {
		if comp, err := h.app.FindRecordById("competitions", compID); err == nil {
			data["CompetitionName"] = comp.GetString("name")
			data["CompetitionLogo"] = league.CompetitionLogoURL(comp.Id, comp.GetString("logo"))
		}
	}
	return h.renderPage(e, "register.html", data)
}

// RegisterSubmit processes the registration form and creates the user account.
func (h *AuthHandler) RegisterSubmit(e *core.RequestEvent) error {
	token := e.Request.FormValue("token")
	displayName := e.Request.FormValue("display_name")
	email := e.Request.FormValue("email")
	password := e.Request.FormValue("password")
	passwordConfirm := e.Request.FormValue("password_confirm")

	if token == "" {
		return alertError(e, "Invitación requerida")
	}

	if password != passwordConfirm {
		return alertError(e, "Las contraseñas no coinciden")
	}

	invites, err := h.app.FindRecordsByFilter("invitations",
		"token = {:token}",
		"", 1, 0,
		map[string]any{"token": token})
	if err != nil || len(invites) == 0 || isInviteExpired(invites[0]) {
		return alertError(e, "Invitación inválida o expirada")
	}
	invite := invites[0]

	maxUses := int(invite.GetFloat("max_uses"))
	if maxUses < 1 {
		maxUses = 1
	}
	useCount := int(invite.GetFloat("use_count"))
	if useCount >= maxUses {
		return alertError(e, "Invitación agotada")
	}

	inviteEmail := invite.GetString("email")
	if inviteEmail != "" && !strings.EqualFold(email, inviteEmail) {
		return alertError(e, "Esta invitación no es válida o ya fue usada")
	}

	gender := e.Request.FormValue("gender")
	if gender != "male" && gender != "female" {
		return alertError(e, "El género es obligatorio")
	}

	note := strings.TrimSpace(e.Request.FormValue("note"))

	_, authToken, err := h.registerUser(registerParams{
		inviteID: invite.Id, email: email, displayName: displayName,
		password: password, gender: gender, note: note,
	})

	if err != nil {
		return alertError(e, "Error al crear la cuenta. Verifica los datos e intenta de nuevo.")
	}

	middleware.SetAuthCookie(e, authToken)

	if e.Request.Header.Get("HX-Request") == "true" {
		return redirectHX(e, "/")
	}
	return e.Redirect(http.StatusFound, "/")
}

type registerParams struct {
	inviteID, email, displayName, password, gender, note string
}

func (h *AuthHandler) registerUser(p registerParams) (*core.Record, string, error) {
	var userRecord *core.Record
	var authToken string

	err := h.app.RunInTransaction(func(txApp core.App) error {
		collection, err := txApp.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		userRecord = core.NewRecord(collection)
		userRecord.Set("email", p.email)
		userRecord.Set("display_name", p.displayName)
		userRecord.Set("roles", []string{"player"})
		if p.gender != "" {
			userRecord.Set("gender", p.gender)
		}
		userRecord.SetPassword(p.password)

		if err := txApp.Save(userRecord); err != nil {
			return err
		}

		if err := consumeInvite(txApp, p.inviteID, userRecord.Id, p.note); err != nil {
			return err
		}

		authToken, err = userRecord.NewAuthToken()
		return err
	})
	return userRecord, authToken, err
}

func consumeInvite(txApp core.App, inviteID, userID, note string) error {
	freshInvite, err := txApp.FindRecordById("invitations", inviteID)
	if err != nil {
		return fmt.Errorf("invitation not found")
	}
	maxUses := int(freshInvite.GetFloat("max_uses"))
	if maxUses < 1 {
		maxUses = 1
	}
	currentCount := int(freshInvite.GetFloat("use_count"))
	if currentCount >= maxUses {
		return fmt.Errorf("invitation exhausted")
	}
	freshInvite.Set("use_count", currentCount+1)
	freshInvite.Set("used_by", userID)
	freshInvite.Set("used_at", time.Now().UTC().Format("2006-01-02 15:04:05.000Z"))
	if currentCount+1 >= maxUses {
		freshInvite.Set("status", "used")
	}
	if note != "" {
		freshInvite.Set("registration_note", note)
	}
	return txApp.Save(freshInvite)
}

// ProfileComplete renders the display-name form for new users.
func (h *AuthHandler) ProfileComplete(e *core.RequestEvent) error {
	return h.renderPage(e, "profile-complete.html", map[string]any{
		"PageTitle": "Completa tu perfil",
	})
}

// ProfileCompleteSubmit saves the display name and redirects to home.
func (h *AuthHandler) ProfileCompleteSubmit(e *core.RequestEvent) error {
	displayName := strings.TrimSpace(e.Request.FormValue("display_name"))
	if displayName == "" {
		return alertError(e, "El nombre es obligatorio")
	}

	gender := e.Request.FormValue("gender")
	if gender != "male" && gender != "female" {
		return alertError(e, "El género es obligatorio")
	}

	e.Auth.Set("display_name", displayName)
	e.Auth.Set("gender", gender)
	if err := h.app.Save(e.Auth); err != nil {
		return alertError(e, "Error al guardar el perfil")
	}

	if e.Request.Header.Get("HX-Request") == "true" {
		return redirectHX(e, "/")
	}
	return e.Redirect(http.StatusFound, "/")
}

// Logout clears the auth cookie and redirects to login.
func (h *AuthHandler) Logout(e *core.RequestEvent) error {
	middleware.ClearAuthCookie(e)

	if e.Request.Header.Get("HX-Request") == "true" {
		return redirectHX(e, "/login")
	}
	return e.Redirect(http.StatusFound, "/login")
}
