package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"sort"
	"strconv"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// InvitationHandler handles admin invitation management and outstanding matches.
type InvitationHandler struct {
	app        core.App
	renderPage RenderFunc
}

// NewInvitationHandler creates an InvitationHandler with the given dependencies.
func NewInvitationHandler(app core.App, renderPage RenderFunc) *InvitationHandler {
	return &InvitationHandler{app: app, renderPage: renderPage}
}

// InvitationsList renders the admin invitations page.
func (h *InvitationHandler) InvitationsList(e *core.RequestEvent) error {
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

// InvitationsCreate generates a new invitation token with the given max uses.
func (h *InvitationHandler) InvitationsCreate(e *core.RequestEvent) error {
	email := e.Request.FormValue("email")
	competition := e.Request.FormValue("competition")

	token, err := generateInviteToken()
	if err != nil {
		return alertError(e, "Error al generar el token")
	}

	col, err := h.app.FindCollectionByNameOrId("invitations")
	if err != nil {
		return alertError(e, "Error interno")
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
		return alertError(e, "Error al crear la invitación")
	}

	return redirectHX(e, "/admin/invitations")
}

// InvitationsRevoke deactivates an invitation so it can no longer be used.
func (h *InvitationHandler) InvitationsRevoke(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	invitation, err := h.app.FindRecordById("invitations", id)
	if err != nil {
		return alertError(e, "Invitación no encontrada")
	}

	if invitation.GetString("status") != "pending" {
		return alertError(e, "Solo se pueden revocar invitaciones pendientes")
	}

	if err := h.app.Delete(invitation); err != nil {
		return alertError(e, "Error al revocar la invitación")
	}

	return redirectHX(e, "/admin/invitations")
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
