package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

func (h *AdminHandler) InvitationsList(e *core.RequestEvent) error {
	invitations, _ := h.app.FindRecordsByFilter("invitations",
		"id != ''", "", 0, 0, nil)

	sort.Slice(invitations, func(i, j int) bool {
		return invitations[i].GetDateTime("created").Time().After(
			invitations[j].GetDateTime("created").Time())
	})

	competitions, _ := h.app.FindRecordsByFilter("competitions",
		"active = true", "name", 0, 0, nil)

	return h.renderPage(e, "admin/invitations.html", map[string]any{
		"Invitations":  invitations,
		"Competitions": competitions,
	})
}

func (h *AdminHandler) InvitationsCreate(e *core.RequestEvent) error {
	email := e.Request.FormValue("email")
	competition := e.Request.FormValue("competition")

	token, err := generateInviteToken()
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al generar el token</div>`)
	}

	col, err := h.app.FindCollectionByNameOrId("invitations")
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error interno</div>`)
	}

	maxUses := 1
	if email == "" {
		if v := e.Request.FormValue("max_uses"); v != "" {
			maxUses, _ = strconv.Atoi(v)
		}
		if maxUses < 1 {
			maxUses = 1
		}
	}

	expirationDays := 7
	if d := e.Request.FormValue("expiration_days"); d != "" {
		expirationDays, _ = strconv.Atoi(d)
	}
	if expirationDays < 1 {
		expirationDays = 1
	}

	record := core.NewRecord(col)
	record.Set("token", token)
	record.Set("email", email)
	record.Set("competition", competition)
	record.Set("created_by", e.Auth.Id)
	record.Set("status", "pending")
	record.Set("max_uses", maxUses)
	record.Set("use_count", 0)
	record.Set("expires_at", time.Now().Add(time.Duration(expirationDays)*24*time.Hour).UTC().Format(time.RFC3339))

	if err := h.app.Save(record); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al crear la invitacion</div>`)
	}

	e.Response.Header().Set("HX-Redirect", "/admin/invitations")
	return e.NoContent(http.StatusNoContent)
}

func (h *AdminHandler) InvitationsRevoke(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	invitation, err := h.app.FindRecordById("invitations", id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Invitacion no encontrada</div>`)
	}

	if invitation.GetString("status") != "pending" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Solo se pueden revocar invitaciones pendientes</div>`)
	}

	if err := h.app.Delete(invitation); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al revocar la invitacion</div>`)
	}

	e.Response.Header().Set("HX-Redirect", "/admin/invitations")
	return e.NoContent(http.StatusNoContent)
}

func generateInviteToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func isInviteExpired(invite *core.Record) bool {
	expiresAt := invite.GetDateTime("expires_at")
	return expiresAt.Time().Before(time.Now())
}
