package handlers

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"padelleague/league"
	"padelleague/render"
)

const resultCorrectionWindow = 24 * time.Hour

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

	if err := h.validateCorrectionAccess(e, match); err != nil {
		return err
	}

	if render.AdminView(e) {
		return h.adminCorrect(e, match)
	}

	scores, err := h.validateCorrectionInput(e, match)
	if err != nil {
		return err
	}

	if err := h.correctResultProposal(match, e.Auth.Id, scores); err != nil {
		switch {
		case errors.Is(err, errOtherSidePending):
			return alertError(e, "El rival ya respondió con otra propuesta. Revisa el hilo del partido.")
		case errors.Is(err, league.ErrMatchNotPreScore):
			return alertError(e, "Este partido ya tiene un resultado final")
		default:
			return alertError(e, "Error al crear la propuesta corregida")
		}
	}

	h.notifyCorrectionToRival(match, e.Auth.Id)
	return redirectHX(e, "/match/"+id)
}

// adminCorrect finalizes the match with the admin's score, exactly like
// AdminOverride: the pending proposals are rejected by the admin and the
// timeline gets an admin_action entry. The admin's write is never a proposal.
func (h *MatchHandler) adminCorrect(e *core.RequestEvent, match *core.Record) error {
	scores, err := readScoreForm(e, match.GetString("carried_sets"), "scores")
	if err != nil || scores == "" {
		return err
	}
	return h.adminOverrideWrite(e, match.Id, overrideForm{scores: scores})
}

// correctResultProposal supersedes the author's pending proposals and creates the
// corrected one, re-checking inside the transaction that the match is still
// pre-score and the rival has not answered meanwhile (a counter-proposal).
func (h *MatchHandler) correctResultProposal(match *core.Record, userID, scores string) error {
	return h.app.RunInTransaction(func(txApp core.App) error {
		fresh, err := txApp.FindRecordById("matches", match.Id)
		if err != nil {
			return err
		}
		if !league.IsPreScore(fresh.GetString("status")) {
			return league.ErrMatchNotPreScore
		}
		if err := ensureNoOtherSidePending(txApp, fresh, userID); err != nil {
			return err
		}
		h.supersedePendingResultsTx(txApp, fresh.Id, userID)
		col, err := txApp.FindCollectionByNameOrId("match_messages")
		if err != nil {
			return err
		}
		pdJSON, _ := json.Marshal(league.ProposalData{Scores: scores})
		proposal := core.NewRecord(col)
		proposal.Set("match", fresh.Id)
		proposal.Set("author", userID)
		proposal.Set("type", "result_submission")
		proposal.Set("content", scores)
		proposal.Set("proposal_status", "pending")
		proposal.Set("proposal_data", string(pdJSON))
		if err := txApp.Save(proposal); err != nil {
			return err
		}
		fresh.SetRaw("submitted_at", types.NowDateTime()) // autodate: Set is a no-op
		fresh.Set("confirm_reminded", false)
		return txApp.Save(fresh)
	})
}

func (h *MatchHandler) validateCorrectionAccess(e *core.RequestEvent, match *core.Record) error {
	if !league.IsPreScore(match.GetString("status")) {
		return alertError(e, "Este partido ya tiene un resultado final")
	}

	pending, _ := h.app.FindRecordsByFilter("match_messages",
		"match = {:mid} && type = 'result_submission' && proposal_status = 'pending'",
		"-created", 1, 0,
		map[string]any{"mid": match.Id})
	if len(pending) == 0 {
		return alertError(e, "No hay propuesta de resultado pendiente para corregir")
	}

	isAdmin := render.AdminView(e)
	submittedByID := pending[0].GetString("author")
	var myTeam int
	if !isAdmin {
		team, err := playerActionGate(h.app, e.Auth.Id, match)
		if err != nil {
			return mapActionGateError(e, err)
		}
		myTeam = team
	}
	if msg := h.validateCorrectionPermission(isAdmin, myTeam, submittedByID, match); msg != "" {
		return alertError(e, msg)
	}
	return nil
}

func (h *MatchHandler) validateCorrectionInput(e *core.RequestEvent, match *core.Record) (string, error) {
	if err := h.validateCorrectionWindow(e, match); err != nil {
		return "", err
	}
	scores, err := readScoreForm(e, match.GetString("carried_sets"), "scores")
	if err != nil {
		return "", err
	}
	return scores, nil
}

func (h *MatchHandler) notifyCorrectionToRival(match *core.Record, correctorID string) {
	rivalPlayers := playersOtherThanAuthorSide(h.app, match, correctorID)
	correctorLabel := pairPlayerLabel(h.app, correctorID, match)
	compName := league.CompetitionName(h.app, match.GetString("competition"))
	h.notifier.NotifyPlayers(rivalPlayers, league.NotifResultCorrected(match.Id, correctorLabel, compName))
}

func (h *MatchHandler) validateCorrectionWindow(e *core.RequestEvent, match *core.Record) error {
	submittedAt := match.GetString("submitted_at")
	if submittedAt == "" {
		return alertError(e, "No se encontró la fecha de envío")
	}
	dt, err := types.ParseDateTime(submittedAt)
	if err != nil || time.Since(dt.Time()) >= resultCorrectionWindow {
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
