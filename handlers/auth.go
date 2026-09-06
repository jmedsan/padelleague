package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"log/slog"
	"sync"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/mails"

	"padelleague/league"
	"padelleague/middleware"
	"padelleague/notify"
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
	params, validationMsg := h.parseRegistrationForm(e)
	if validationMsg != "" {
		return alertError(e, validationMsg)
	}

	_, authToken, err := h.registerUser(params)
	if err != nil {
		return alertError(e, "Error al crear la cuenta. Verifica los datos e intenta de nuevo.")
	}

	middleware.SetAuthCookie(e, authToken)
	if e.Request.Header.Get("HX-Request") == "true" {
		return redirectHX(e, "/")
	}
	return e.Redirect(http.StatusFound, "/")
}

func (h *AuthHandler) parseRegistrationForm(e *core.RequestEvent) (registerParams, string) {
	token := e.Request.FormValue("token")
	if token == "" {
		return registerParams{}, "Invitación requerida"
	}
	password := e.Request.FormValue("password")
	if password != e.Request.FormValue("password_confirm") {
		return registerParams{}, "Las contraseñas no coinciden"
	}

	invite, msg := h.validateInviteToken(token, e.Request.FormValue("email"))
	if msg != "" {
		return registerParams{}, msg
	}

	gender := e.Request.FormValue("gender")
	if gender != "male" && gender != "female" {
		return registerParams{}, "El género es obligatorio"
	}

	phone, phoneErr := league.NormalizePhone(strings.TrimSpace(e.Request.FormValue("phone")))
	if phoneErr != nil {
		return registerParams{}, phoneErr.Error() //nolint:goerr113 // user-facing Spanish
	}

	return registerParams{
		inviteID:    invite.Id,
		inviteEmail: invite.GetString("email"),
		adminNote:   invite.GetString("admin_note"),
		email:       e.Request.FormValue("email"),
		displayName: e.Request.FormValue("display_name"),
		password:    password,
		gender:      gender,
		phone:       phone,
		note:        strings.TrimSpace(e.Request.FormValue("note")),
	}, ""
}

func (h *AuthHandler) validateInviteToken(token, email string) (*core.Record, string) {
	invites, err := h.app.FindRecordsByFilter("invitations",
		"token = {:token}", "", 1, 0, map[string]any{"token": token})
	if err != nil || len(invites) == 0 || isInviteExpired(invites[0]) {
		return nil, "Invitación inválida o expirada"
	}
	invite := invites[0]

	maxUses := int(invite.GetFloat("max_uses"))
	if maxUses < 1 {
		maxUses = 1
	}
	if int(invite.GetFloat("use_count")) >= maxUses {
		return nil, "Invitación agotada"
	}

	inviteEmail := invite.GetString("email")
	if inviteEmail != "" && !strings.EqualFold(email, inviteEmail) {
		return nil, "Esta invitación no es válida o ya fue usada"
	}
	return invite, ""
}

type registerParams struct {
	inviteID, inviteEmail, adminNote                  string
	email, displayName, password, gender, phone, note string
}

func (h *AuthHandler) registerUser(p registerParams) (*core.Record, string, error) {
	boundInvite := p.inviteEmail != "" && strings.EqualFold(p.email, p.inviteEmail)

	userRecord, authToken, err := h.createUserInTx(p, boundInvite)
	if err != nil {
		return nil, "", err
	}

	if !boundInvite && notify.IsMailerConfigured(h.app) {
		if err := mails.SendRecordVerification(h.app, userRecord); err != nil {
			slog.Error("verification email failed", "err", err)
		}
	}

	return userRecord, authToken, nil
}

func (h *AuthHandler) createUserInTx(p registerParams, verified bool) (*core.Record, string, error) {
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
		userRecord.Set("phone", p.phone)
		userRecord.Set("gender", p.gender)
		userRecord.Set("admin_note", p.adminNote)
		userRecord.Set("registration_note", p.note)
		userRecord.SetPassword(p.password)
		userRecord.SetVerified(verified)

		if err := txApp.Save(userRecord); err != nil {
			return err
		}
		if err := consumeInvite(txApp, p.inviteID, userRecord.Id); err != nil {
			return err
		}

		authToken, err = userRecord.NewAuthToken()
		return err
	})
	return userRecord, authToken, err
}

func consumeInvite(txApp core.App, inviteID, userID string) error {
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
	// Append to multi-relation used_by
	usedBy := freshInvite.GetStringSlice("used_by")
	usedBy = append(usedBy, userID)
	freshInvite.Set("used_by", usedBy)
	freshInvite.Set("used_at", time.Now().UTC().Format("2006-01-02 15:04:05.000Z"))
	if currentCount+1 >= maxUses {
		freshInvite.Set("status", "used")
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

// VerifyEmail confirms a verification token from the email link and marks
// the user as verified.
func (h *AuthHandler) VerifyEmail(e *core.RequestEvent) error {
	token := e.Request.URL.Query().Get("token")
	if token == "" {
		return h.renderPage(e, "verify-email.html", map[string]any{
			"PageTitle": "Verificar email",
			"Error":     true,
		})
	}

	record, err := h.app.FindAuthRecordByToken(token, core.TokenTypeVerification)
	if err != nil {
		return h.renderPage(e, "verify-email.html", map[string]any{
			"PageTitle": "Verificar email",
			"Error":     true,
		})
	}

	record.SetVerified(true)
	if err := h.app.Save(record); err != nil {
		return h.renderPage(e, "verify-email.html", map[string]any{
			"PageTitle": "Verificar email",
			"Error":     true,
		})
	}

	return h.renderPage(e, "verify-email.html", map[string]any{
		"PageTitle": "Email confirmado",
		"Verified":  true,
	})
}

// resendLimiter tracks the last verification resend per user.
var resendLimiter = struct {
	sync.Mutex
	sent map[string]time.Time
}{sent: make(map[string]time.Time)}

// ResendVerification sends a new verification email. Rate-limited to one
// per 10 minutes per user.
func (h *AuthHandler) ResendVerification(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.Redirect(http.StatusFound, "/login")
	}

	if e.Auth.Verified() {
		return alertSuccess(e, "Tu email ya está verificado.")
	}

	resendLimiter.Lock()
	last, exists := resendLimiter.sent[e.Auth.Id]
	if exists && time.Since(last) < 10*time.Minute {
		resendLimiter.Unlock()
		return alertError(e, "Ya enviamos un email hace poco. Inténtalo en unos minutos.")
	}
	resendLimiter.sent[e.Auth.Id] = time.Now()
	resendLimiter.Unlock()

	if err := mails.SendRecordVerification(h.app, e.Auth); err != nil {
		return alertError(e, "No se pudo enviar el email. Inténtalo de nuevo.")
	}
	return alertSuccess(e, "Email de verificación enviado.")
}

// Logout clears the auth cookie and redirects to login.
func (h *AuthHandler) Logout(e *core.RequestEvent) error {
	middleware.ClearAuthCookie(e)

	if e.Request.Header.Get("HX-Request") == "true" {
		return redirectHX(e, "/login")
	}
	return e.Redirect(http.StatusFound, "/login")
}
