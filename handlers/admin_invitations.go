package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
	"padelleague/notify"
	"padelleague/render"
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

// InvitationsList renders the global admin invitations page, newest first.
func (h *InvitationHandler) InvitationsList(e *core.RequestEvent) error {
	invitations, err := h.app.FindRecordsByFilter("invitations",
		"id != ''", "", 0, 0, nil)
	if err != nil {
		slog.Error("InvitationsList: find invitations", "err", err)
	}
	sort.Slice(invitations, func(i, j int) bool {
		return invitations[i].GetDateTime("created").Time().After(
			invitations[j].GetDateTime("created").Time())
	})

	competitions, err := h.app.FindRecordsByFilter("competitions",
		"active = true", "name", 0, 0, nil)
	if err != nil {
		slog.Error("InvitationsList: find competitions", "err", err)
	}

	compNames := make(map[string]string, len(invitations))
	for _, inv := range invitations {
		if compID := inv.GetString("competition"); compID != "" {
			if _, ok := compNames[compID]; !ok {
				compNames[compID] = league.CompetitionName(h.app, compID)
			}
		}
	}

	return h.renderPage(e, "admin/invitations.html", map[string]any{
		"PageTitle":    "Invitaciones",
		"Invitations":  invitations,
		"Competitions": competitions,
		"CompNames":    compNames,
		"BaseURL":      render.RequestBaseURL(e),
	})
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
	adminNote := strings.TrimSpace(e.Request.FormValue("admin_note"))
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
	record.Set("admin_note", adminNote)
	record.Set("created_by", e.Auth.Id)
	record.Set("status", "pending")
	record.Set("max_uses", maxUses)
	record.Set("use_count", 0)
	record.Set("expires_at", time.Now().Add(time.Duration(expirationDays)*24*time.Hour).UTC().Format(time.RFC3339))

	if err := h.app.Save(record); err != nil {
		return alertError(e, "Error al crear la invitación")
	}

	if email != "" {
		registerURL := render.RequestBaseURL(e) + "/register?token=" + token
		compName := league.CompetitionName(h.app, competition)
		notify.SendEmail(h.app, email, notify.SubjectPrefix()+"Invitación a Liga Dale Fuerte",
			notify.RenderEmail(h.app, competition, buildInviteEmail(registerURL, compName)))
	}

	flash(e, "Invitación creada")
	return redirectHX(e, "/admin/invitations")
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

// InvitationsResend re-sends the invitation email for an existing invitation.
func (h *InvitationHandler) InvitationsResend(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	invitation, err := h.app.FindRecordById("invitations", id)
	if err != nil {
		return alertError(e, "Invitación no encontrada")
	}
	email := invitation.GetString("email")
	if email == "" {
		return alertError(e, "Esta invitación no tiene email")
	}
	token := invitation.GetString("token")
	compID := invitation.GetString("competition")
	registerURL := render.RequestBaseURL(e) + "/register?token=" + token
	compName := league.CompetitionName(h.app, compID)
	notify.SendEmail(h.app, email, notify.SubjectPrefix()+"Invitación a Liga Dale Fuerte",
		notify.RenderEmail(h.app, compID, buildInviteEmail(registerURL, compName)))
	flash(e, "Invitación reenviada")
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

	flash(e, "Invitación revocada")
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
