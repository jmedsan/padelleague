package handlers

import (
	"net/http"

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
	return h.renderPage(e, "register.html", map[string]any{})
}

func (h *AuthHandler) RegisterSubmit(e *core.RequestEvent) error {
	displayName := e.Request.FormValue("display_name")
	email := e.Request.FormValue("email")
	password := e.Request.FormValue("password")
	passwordConfirm := e.Request.FormValue("password_confirm")

	if password != passwordConfirm {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Las contraseñas no coinciden</div>`)
	}

	collection, err := h.app.FindCollectionByNameOrId("users")
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error interno</div>`)
	}

	record := core.NewRecord(collection)
	record.Set("email", email)
	record.Set("display_name", displayName)
	record.Set("role", "user")
	record.SetPassword(password)

	if err := h.app.Save(record); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al crear la cuenta. Verifica los datos e intenta de nuevo.</div>`)
	}

	token, err := record.NewAuthToken()
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Cuenta creada, pero error al iniciar sesión. Intenta iniciar sesión manualmente.</div>`)
	}

	middleware.SetAuthCookie(e, token)

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
