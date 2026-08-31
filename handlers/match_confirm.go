package handlers

import (
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"padelleague/league"
)

// MatchConfirm handles the opponent's confirmation of a submitted score.
func (h *MatchHandler) MatchConfirm(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := findMatchOr404(h.app, e, id)
	if err != nil {
		return err
	}

	if err := checkCompModifiable(h.app, e, match); err != nil {
		return err
	}

	if match.GetString("status") != league.StatusConfirmed {
		return alertError(e, "Este partido no está pendiente de confirmación")
	}

	userID := e.Auth.Id
	submitterTeam, err := h.validateConfirmParticipant(e, match, userID)
	if err != nil {
		return err
	}

	score := match.GetString("scores")
	winnerID, err := league.DetermineWinner(match, score)
	if err != nil {
		slog.Error("determine winner failed", "match", match.Id, "err", err)
		return alertError(e, "Error al determinar el ganador")
	}

	match.Set("confirmed_by", userID)
	match.Set("winner", winnerID)
	match.Set("status", league.StatusFinal)

	if err := h.app.Save(match); err != nil {
		return alertError(e, "Error al confirmar el partido")
	}

	addTimelineEntry(h.app, timelineEntry{
		MatchID: match.Id, ActorID: userID,
		Kind: "result_event", Detail: pairPlayerLabel(h.app, userID, match) + " confirmó el resultado",
	})

	h.notifyConfirmToSubmitter(match, submitterTeam, id)

	participants := matchParticipantUserIDs(h.app, match)
	if err := h.notifier.NotifyAdmins(league.NotifAdminMatchProgress(match.Id, "Resultado confirmado: "+score), participants...); err != nil {
		slog.Error("notify admins match progress failed", "match", match.Id, "err", err)
	}

	return redirectHX(e, "/match/"+id)
}

func (h *MatchHandler) notifyConfirmToSubmitter(match *core.Record, submitterTeam int, matchID string) {
	submitterPairID := match.GetString("pair1")
	if submitterTeam == 2 {
		submitterPairID = match.GetString("pair2")
	}
	submitterPlayers := league.PlayersForPair(h.app, submitterPairID)
	n := league.NotifResultConfirmed(match.Id)
	h.notifier.NotifyPlayers(submitterPlayers, n)
	h.notifier.EmailPlayers(submitterPlayers, n.Title, n.Body, "/match/"+matchID)
}

func (h *MatchHandler) validateConfirmParticipant(e *core.RequestEvent, match *core.Record, userID string) (int, error) {
	isAdmin := slices.Contains(e.Auth.GetStringSlice("roles"), "admin")
	myTeam, err := league.PlayerTeam(h.app, userID, match)
	if err != nil && !isAdmin {
		return 0, alertError(e, "No eres participante de este partido")
	}
	submittedByID := match.GetString("submitted_by")
	if submittedByID == "" {
		return 0, alertError(e, "No se encontró quién envió el resultado")
	}
	submitterTeam, err := league.PlayerTeam(h.app, submittedByID, match)
	if err != nil {
		return 0, alertError(e, "Error al verificar el equipo que envió el resultado")
	}
	if myTeam == submitterTeam {
		return 0, alertError(e, "No puedes confirmar tu propio resultado")
	}
	return submitterTeam, nil
}

// MatchDispute handles the opponent disputing a submitted score.
func (h *MatchHandler) MatchDispute(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := findMatchOr404(h.app, e, id)
	if err != nil {
		return err
	}

	if err := checkCompModifiable(h.app, e, match); err != nil {
		return err
	}

	if match.GetString("status") != league.StatusConfirmed {
		return alertError(e, "Este partido no está pendiente de confirmación")
	}

	userID := e.Auth.Id
	isAdmin := slices.Contains(e.Auth.GetStringSlice("roles"), "admin")
	myTeam, err := league.PlayerTeam(h.app, userID, match)
	if err != nil && !isAdmin {
		return alertError(e, "No eres participante de este partido")
	}

	submittedByID := match.GetString("submitted_by")
	if submittedByID != "" {
		submitterTeam, err := league.PlayerTeam(h.app, submittedByID, match)
		if err == nil && myTeam == submitterTeam {
			return alertError(e, "No puedes disputar tu propio resultado")
		}
	}

	disputedScores := e.Request.FormValue("disputed_scores")
	if disputedScores == "" {
		return alertError(e, "Debes indicar el marcador correcto según tú")
	}
	if _, err := league.ParseScore(disputedScores); err != nil {
		return alertError(e, "El marcador que propones no es válido")
	}
	disputeNotes := e.Request.FormValue("dispute_notes")
	match.Set("disputed_by", userID)
	match.Set("disputed_scores", disputedScores)
	match.Set("dispute_notes", disputeNotes)
	match.Set("status", league.StatusDisputed)

	if err := h.app.Save(match); err != nil {
		return alertError(e, "Error al disputar el partido")
	}

	h.recordDispute(match, userID, disputedScores, disputeNotes)
	h.notifyDisputeToSubmitter(match, myTeam)
	return redirectHX(e, "/match/"+id)
}

func (h *MatchHandler) recordDispute(match *core.Record, userID, scores, notes string) {
	label := pairPlayerLabel(h.app, userID, match)
	addTimelineEntry(h.app, timelineEntry{
		MatchID: match.Id, ActorID: userID,
		Kind: "result_event", Detail: label + " disputó el resultado",
	})
	addTimelineEntry(h.app, timelineEntry{
		MatchID: match.Id, ActorID: userID,
		Kind: "result_submission", Detail: scores,
	})
	n := league.NotifAdminDisputed(match.Id, notes)
	if err := h.notifier.NotifyAdmins(n); err != nil {
		slog.Error("notify admins failed", "err", err)
	}
}

// notifyDisputeToSubmitter tells the pair whose score was disputed — they'd
// otherwise only find out by noticing the status change.
func (h *MatchHandler) notifyDisputeToSubmitter(match *core.Record, disputerTeam int) {
	if disputerTeam <= 0 {
		return
	}
	submitterPairID := match.GetString("pair1")
	if disputerTeam == 1 {
		submitterPairID = match.GetString("pair2")
	}
	submitterPlayers := league.PlayersForPair(h.app, submitterPairID)
	n := league.NotifResultDisputed(match.Id)
	h.notifier.NotifyPlayers(submitterPlayers, n)
	h.notifier.EmailPlayers(submitterPlayers, n.Title, n.Body, "/match/"+match.Id)
}

// MatchCorrect allows the submitting team to correct a confirmed score.
func (h *MatchHandler) MatchCorrect(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := findMatchOr404(h.app, e, id)
	if err != nil {
		return err
	}

	if err := checkCompModifiable(h.app, e, match); err != nil {
		return err
	}

	if match.GetString("status") != league.StatusConfirmed {
		return alertError(e, "Solo se puede corregir un partido en estado confirmado")
	}

	userID := e.Auth.Id
	isAdmin := slices.Contains(e.Auth.GetStringSlice("roles"), "admin")
	submittedByID := match.GetString("submitted_by")
	if submittedByID == "" {
		return alertError(e, "No se encontró quién envió el resultado")
	}

	myTeam, err := league.PlayerTeam(h.app, userID, match)
	if err != nil && !isAdmin {
		return alertError(e, "No eres participante de este partido")
	}
	if msg := h.validateCorrectionPermission(isAdmin, myTeam, submittedByID, match); msg != "" {
		return alertError(e, msg)
	}

	if err := h.validateCorrectionWindow(e, match); err != nil {
		return err
	}
	scores := e.Request.FormValue("scores")
	if scores == "" {
		return alertError(e, "Debes indicar el marcador corregido")
	}

	if strings.EqualFold(strings.TrimSpace(scores), "WO") {
		return alertError(e, "Usa el botón de incomparecencia para reportar un WO")
	}

	if _, err := league.ParseScore(scores); err != nil {
		return alertError(e, "Marcador no válido")
	}

	match.Set("scores", scores)
	match.Set("confirmed_by", "")
	match.Set("submitted_at", time.Now().UTC().Format(time.RFC3339))
	match.Set("confirm_reminded", false)

	if err := h.app.Save(match); err != nil {
		return alertError(e, "Error al corregir el resultado")
	}

	addTimelineEntry(h.app, timelineEntry{
		MatchID: match.Id, ActorID: userID,
		Kind: "result_submission", Detail: scores,
	})
	h.notifyCorrectionToRival(match, myTeam)
	return redirectHX(e, "/match/"+id)
}

func (h *MatchHandler) notifyCorrectionToRival(match *core.Record, myTeam int) {
	rivalPairID := match.GetString("pair2")
	if myTeam == 2 {
		rivalPairID = match.GetString("pair1")
	}
	rivalPlayers := league.PlayersForPair(h.app, rivalPairID)
	h.notifier.NotifyPlayers(rivalPlayers, league.NotifResultCorrected(match.Id))
}

func (h *MatchHandler) validateCorrectionWindow(e *core.RequestEvent, match *core.Record) error {
	submittedAt := match.GetString("submitted_at")
	if submittedAt == "" {
		return alertError(e, "No se encontró la fecha de envío")
	}
	dt, err := types.ParseDateTime(submittedAt)
	if err != nil || time.Since(dt.Time()) >= 24*time.Hour {
		return alertError(e, "El plazo de 24 horas para corregir ha expirado")
	}
	return nil
}

func (h *MatchHandler) validateCorrectionPermission(isAdmin bool, myTeam int, submittedByID string, match *core.Record) string {
	if isAdmin {
		return ""
	}
	submitterTeam, err := league.PlayerTeam(h.app, submittedByID, match)
	if err != nil || myTeam != submitterTeam {
		return "Solo el equipo que envió el resultado puede corregirlo"
	}
	return ""
}
