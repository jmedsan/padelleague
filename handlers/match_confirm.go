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
	if submittedByID == "" {
		return alertError(e, "No se encontró quién envió el resultado")
	}
	submitterTeam, err := league.PlayerTeam(h.app, submittedByID, match)
	if err != nil {
		return alertError(e, "Error al verificar el equipo que envió el resultado")
	}

	if myTeam == submitterTeam {
		return alertError(e, "No puedes confirmar tu propio resultado")
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

	submitterPairID := match.GetString("pair1")
	if submitterTeam == 2 {
		submitterPairID = match.GetString("pair2")
	}
	submitterPlayers := league.PlayersForPair(h.app, submitterPairID)
	h.notifier.NotifyPlayers(submitterPlayers, league.Notification{
		Type: "general", Title: "Resultado confirmado",
		Body: "Tu rival ha confirmado el resultado del partido.", MatchID: match.Id,
	})
	h.notifier.EmailPlayers(submitterPlayers, "Resultado confirmado", "Tu rival ha confirmado el resultado del partido.", "/match/"+id)

	return redirectHX(e, "/match/"+id)
}

// MatchDispute handles the opponent disputing a submitted score.
func (h *MatchHandler) MatchDispute(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := findMatchOr404(h.app, e, id)
	if err != nil {
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
	if disputedScores != "" {
		if _, err := league.ParseScore(disputedScores); err != nil {
			return alertError(e, "El marcador que propones no es válido")
		}
	}
	disputeNotes := e.Request.FormValue("dispute_notes")
	match.Set("disputed_by", userID)
	match.Set("disputed_scores", disputedScores)
	match.Set("dispute_notes", disputeNotes)
	match.Set("status", league.StatusDisputed)

	if err := h.app.Save(match); err != nil {
		return alertError(e, "Error al disputar el partido")
	}

	if err := h.notifier.NotifyAdmins("dispute", "Partido disputado", disputeNotes, match.Id); err != nil {
		slog.Error("notify admins failed", "err", err)
	}

	h.notifyDisputeToSubmitter(match, myTeam)

	return redirectHX(e, "/match/"+id)
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
	h.notifier.NotifyPlayers(submitterPlayers, league.Notification{
		Type: "general", Title: "Resultado disputado",
		Body: "Tu rival ha disputado el resultado que enviaste.", MatchID: match.Id,
	})
	h.notifier.EmailPlayers(submitterPlayers, "Resultado disputado",
		"Tu rival ha disputado el resultado que enviaste.", "/match/"+match.Id)
}

// MatchCorrect allows the submitting team to correct a confirmed score.
func (h *MatchHandler) MatchCorrect(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := findMatchOr404(h.app, e, id)
	if err != nil {
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

	submittedAt := match.GetString("submitted_at")
	if submittedAt == "" {
		return alertError(e, "No se encontró la fecha de envío")
	}
	dt, err := types.ParseDateTime(submittedAt)
	if err != nil || time.Since(dt.Time()) >= 24*time.Hour {
		return alertError(e, "El plazo de 24 horas para corregir ha expirado")
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

	h.notifyCorrectionToRival(match, myTeam)
	return redirectHX(e, "/match/"+id)
}

func (h *MatchHandler) notifyCorrectionToRival(match *core.Record, myTeam int) {
	rivalPairID := match.GetString("pair2")
	if myTeam == 2 {
		rivalPairID = match.GetString("pair1")
	}
	rivalPlayers := league.PlayersForPair(h.app, rivalPairID)
	h.notifier.NotifyPlayers(rivalPlayers, league.Notification{
		Type: "quorum_request", Title: "Resultado corregido",
		Body: "El rival ha corregido el resultado. Confirma o disputa.", MatchID: match.Id,
	})
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
