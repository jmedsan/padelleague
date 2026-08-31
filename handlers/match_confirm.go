package handlers

import (
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"padelleague/league"
)

// MatchCorrect allows the submitting team to correct a pending result proposal.
func (h *MatchHandler) MatchCorrect(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := findMatchOr404(h.app, e, id)
	if err != nil {
		return err
	}

	if err := checkDocGate(h.app, e, match); err != nil {
		return err
	}

	if err := checkCompModifiable(h.app, e, match); err != nil {
		return err
	}

	myTeam, err := h.validateCorrectionAccess(e, match)
	if err != nil {
		return err
	}

	scores, err := h.validateCorrectionInput(e, match)
	if err != nil {
		return err
	}

	h.supersedeMyPendingResults(match.Id, e.Auth.Id)

	col, err2 := h.app.FindCollectionByNameOrId("match_messages")
	if err2 != nil {
		return alertError(e, "Error interno")
	}
	pdJSON, _ := json.Marshal(ProposalData{Scores: scores})
	proposal := core.NewRecord(col)
	proposal.Set("match", match.Id)
	proposal.Set("author", e.Auth.Id)
	proposal.Set("type", "result_submission")
	proposal.Set("content", scores)
	proposal.Set("proposal_status", "pending")
	proposal.Set("proposal_data", string(pdJSON))
	if err := h.app.Save(proposal); err != nil {
		return alertError(e, "Error al crear la propuesta corregida")
	}

	match.Set("submitted_at", time.Now().UTC().Format(time.RFC3339))
	match.Set("confirm_reminded", false)
	if err := h.app.Save(match); err != nil {
		slog.Error("save match after correction", "match", match.Id, "err", err)
	}

	h.notifyCorrectionToRival(match, myTeam)
	return redirectHX(e, "/match/"+id)
}

func (h *MatchHandler) validateCorrectionAccess(e *core.RequestEvent, match *core.Record) (int, error) {
	if !league.IsPreScore(match.GetString("status")) {
		return 0, alertError(e, "Este partido ya tiene un resultado final")
	}

	pending, _ := h.app.FindRecordsByFilter("match_messages",
		"match = {:mid} && type = 'result_submission' && proposal_status = 'pending'",
		"-created", 1, 0,
		map[string]any{"mid": match.Id})
	if len(pending) == 0 {
		return 0, alertError(e, "No hay propuesta de resultado pendiente para corregir")
	}

	isAdmin := isEffectiveAdmin(e)
	submittedByID := pending[0].GetString("author")
	myTeam, err := league.PlayerTeam(h.app, e.Auth.Id, match)
	if err != nil && !isAdmin {
		return 0, alertError(e, "No eres participante de este partido")
	}
	if msg := h.validateCorrectionPermission(isAdmin, myTeam, submittedByID, match); msg != "" {
		return 0, alertError(e, msg)
	}
	return myTeam, nil
}

func (h *MatchHandler) validateCorrectionInput(e *core.RequestEvent, match *core.Record) (string, error) {
	if err := h.validateCorrectionWindow(e, match); err != nil {
		return "", err
	}
	scores := e.Request.FormValue("scores")
	if scores == "" {
		return "", alertError(e, "Debes indicar el marcador corregido")
	}
	if strings.EqualFold(strings.TrimSpace(scores), "WO") {
		return "", alertError(e, "Usa el botón de incomparecencia para reportar un WO")
	}
	if _, err := league.ParseScore(scores); err != nil {
		return "", alertError(e, "Marcador no válido")
	}
	return scores, nil
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
