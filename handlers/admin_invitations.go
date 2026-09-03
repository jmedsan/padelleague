package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/notify"
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

// InvitationsList redirects to /admin/competitions — invitation management
// lives per-competition (inline in competition-detail.html, like Documentos)
// now, not on a standalone global list.
func (h *InvitationHandler) InvitationsList(e *core.RequestEvent) error {
	return e.Redirect(http.StatusFound, "/admin/competitions")
}

// CompetitionInvitations returns a competition's invitations, newest first —
// shared by CompetitionHandler.Detail (renders the Invitaciones section
// inline, like Documentos) so invitation management lives per-competition.
func CompetitionInvitations(app core.App, compID string) []*core.Record {
	invitations, _ := app.FindRecordsByFilter("invitations",
		"competition = {:cid}", "", 0, 0,
		map[string]any{"cid": compID})
	sort.Slice(invitations, func(i, j int) bool {
		return invitations[i].GetDateTime("created").Time().After(
			invitations[j].GetDateTime("created").Time())
	})
	return invitations
}

// InvitationsCreate generates a new invitation token with the given max uses.
func (h *InvitationHandler) InvitationsCreate(e *core.RequestEvent) error {
	email := strings.TrimSpace(e.Request.FormValue("email"))
	competition := e.Request.FormValue("competition")
	if competition == "" {
		return alertError(e, "La competición es obligatoria")
	}
	if email != "" && !strings.Contains(email, "@") {
		return alertError(e, "El email no es válido")
	}

	maxUses, err := parsePositiveInt(e.Request.FormValue("max_uses"), 1)
	if err != nil {
		return alertError(e, "Los usos máximos deben ser un número entero mayor que 0")
	}

	expirationDays, err := parsePositiveInt(e.Request.FormValue("expiration_days"), 7)
	if err != nil {
		return alertError(e, "Los días hasta expirar deben ser un número entero mayor que 0")
	}

	token, err := generateInviteToken()
	if err != nil {
		return alertError(e, "Error al generar el token")
	}

	col, err := h.app.FindCollectionByNameOrId("invitations")
	if err != nil {
		return alertError(e, "Error interno")
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

	if email != "" {
		// Token in URL is inherent: the invite link requires it; admin-only copy-paste, not logged.
		registerURL := requestBaseURL(e) + "/register?token=" + token
		notify.SendEmail(h.app, email, "Invitación a Padel League",
			buildInviteEmail(registerURL))
	}

	return redirectHX(e, "/admin/competitions/"+competition)
}

// parsePositiveInt parses a form value as a positive integer, returning def
// when the value is blank. Any non-numeric value or a value less than 1 is
// rejected rather than silently coerced.
func parsePositiveInt(v string, def int) (int, error) {
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, err
	}
	if n < 1 {
		return 0, fmt.Errorf("must be at least 1, got %d", n)
	}
	return n, nil
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

	compID := invitation.GetString("competition")
	if err := h.app.Delete(invitation); err != nil {
		return alertError(e, "Error al revocar la invitación")
	}

	return redirectHX(e, "/admin/competitions/"+compID)
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
