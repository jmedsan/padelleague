package handlers

import (
	"encoding/json"
	"errors"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
)

var errScheduleAlreadyAccepted = errors.New("scheduling proposal already accepted")

// lockPendingProposal reloads the match and the proposal inside the transaction
// and checks they are still pre-score and pending. Handlers check the same
// things before opening the transaction, but another request may have moved
// either record since; only this read is atomic with the write that follows,
// and the fresh records are the ones to save.
func lockPendingProposal(txApp core.App, matchID, msgID string) (match, msg *core.Record, err error) {
	if match, err = txApp.FindRecordById("matches", matchID); err != nil {
		return nil, nil, err
	}
	if !league.IsPreScore(match.GetString("status")) {
		return nil, nil, league.ErrMatchNotPreScore
	}
	if msg, err = txApp.FindRecordById("match_messages", msgID); err != nil {
		return nil, nil, err
	}
	if msg.GetString("proposal_status") != "pending" {
		return nil, nil, errProposalNotPending
	}
	return match, msg, nil
}

// proposalTxAlert maps the sentinels of a guarded proposal transaction to the
// alert the player sees; any other error gets the fallback text.
func proposalTxAlert(e *core.RequestEvent, err error, fallback string) error {
	switch {
	case errors.Is(err, league.ErrMatchNotPreScore):
		return alertError(e, "Este partido ya no acepta propuestas")
	case errors.Is(err, errProposalNotPending):
		return alertError(e, "Esta propuesta ya fue respondida")
	case errors.Is(err, errScheduleAlreadyAccepted):
		return alertError(e, "Ya hay una propuesta aceptada para este partido")
	}
	return alertError(e, fallback)
}

// scheduleAccept is what acceptScheduleTx needs to accept a date proposal.
type scheduleAccept struct {
	actorID, matchID, msgID, proposerName string
	pd                                    *league.ProposalData
}

// acceptScheduleTx accepts the date proposal on fresh copies of the match and
// the proposal, retiring any earlier accepted date, and returns the saved match.
func acceptScheduleTx(txApp core.App, a scheduleAccept) (*core.Record, error) {
	actorID, matchID, msgID, pd, proposerName := a.actorID, a.matchID, a.msgID, a.pd, a.proposerName
	match, msg, err := lockPendingProposal(txApp, matchID, msgID)
	if err != nil {
		return nil, err
	}
	existing, err := txApp.FindRecordsByFilter("match_messages",
		"match = {:mid} && type = 'scheduling_proposal' && proposal_status = 'accepted'",
		"", 0, 0, map[string]any{"mid": matchID})
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 && match.GetString("status") != league.StatusScheduled {
		return nil, errScheduleAlreadyAccepted
	}
	for _, old := range existing {
		old.Set("proposal_status", "superseded")
		if err := txApp.Save(old); err != nil {
			return nil, err
		}
	}
	match.Set("date", pd.Date)
	match.Set("time", pd.Time)
	match.Set("club", pd.VenueName)
	match.Set("status", league.StatusScheduled)
	league.ClearMatchReminders(txApp, matchID)
	if err := txApp.Save(match); err != nil {
		return nil, err
	}
	msg.Set("proposal_status", "accepted")
	if err := txApp.Save(msg); err != nil {
		return nil, err
	}
	addTimelineEntry(txApp, timelineEntry{
		MatchID: matchID, ActorID: actorID,
		Kind:     "scheduling_response",
		Detail:   "aceptó la propuesta de " + proposerName + " (" + pd.Date + ", " + pd.Time + ", " + pd.VenueName + ")",
		ParentID: msgID,
		Action:   "accept",
		Data:     pd,
	})
	return match, nil
}

func (h *ThreadHandler) acceptProposal(e *core.RequestEvent, match, msg *core.Record, _ string) error {
	pd := league.ParseProposalData(msg.Get("proposal_data"))
	if pd == nil {
		return alertError(e, "Error al leer los datos de la propuesta")
	}

	proposerName := pairPlayerLabel(h.app, msg.GetString("author"), match)
	var fresh *core.Record
	if err := h.app.RunInTransaction(func(txApp core.App) (err error) {
		fresh, err = acceptScheduleTx(txApp, scheduleAccept{
			actorID: e.Auth.Id, matchID: match.Id, msgID: msg.Id, proposerName: proposerName, pd: pd,
		})
		return err
	}); err != nil {
		return proposalTxAlert(e, err, "Error al aceptar la propuesta")
	}
	match = fresh

	h.supersedePendingAndNotify(match, msg.Id)

	compName := league.CompetitionName(h.app, match.GetString("competition"))
	notif := league.NotifProposalAccepted(league.ProposalAcceptedParams{
		MatchID: match.Id, ResponderName: pairPlayerLabel(h.app, e.Auth.Id, match), Date: pd.Date, Time: pd.Time, CompName: compName,
	})
	allPlayers := league.MatchPlayersExcluding(h.app, match, e.Auth.Id)
	h.notifier.NotifyPlayers(allPlayers, notif)
	return nil
}

func (h *ThreadHandler) rejectProposal(e *core.RequestEvent, msg *core.Record, match *core.Record, proposerPairID string) error {
	reason := e.Request.FormValue("rejection_reason")
	text := e.Request.FormValue("rejection_text")

	proposerName := pairPlayerLabel(h.app, msg.GetString("author"), match)
	detail := "rechazó la propuesta de " + proposerName
	if text != "" {
		detail += ": " + text
	} else if reason != "" {
		detail += ": " + reason
	}
	note := text
	if note == "" {
		note = reason
	}
	if err := h.app.RunInTransaction(func(txApp core.App) error {
		_, freshMsg, err := lockPendingProposal(txApp, match.Id, msg.Id)
		if err != nil {
			return err
		}
		freshMsg.Set("proposal_status", "rejected")
		freshMsg.Set("rejection_reason", reason)
		freshMsg.Set("rejection_text", text)
		if err := txApp.Save(freshMsg); err != nil {
			return err
		}
		addTimelineEntry(txApp, timelineEntry{
			MatchID: match.Id, ActorID: e.Auth.Id,
			Kind: "scheduling_response", Detail: detail,
			ParentID: msg.Id, Action: "reject", Note: note,
			Data: league.ParseProposalData(msg.Get("proposal_data")),
		})
		return nil
	}); err != nil {
		return proposalTxAlert(e, err, "Error al rechazar la propuesta")
	}

	proposerPlayers := league.PlayersForPair(h.app, proposerPairID)
	compName := league.CompetitionName(h.app, match.GetString("competition"))
	notifReason := reason
	if reason == "Otro" && text != "" {
		notifReason = text
	} else if reason == "Otro" {
		notifReason = ""
	}
	notif := league.NotifProposalRejected(match.Id, pairPlayerLabel(h.app, e.Auth.Id, match), notifReason, compName)
	h.notifier.NotifyPlayers(proposerPlayers, notif)
	return nil
}

func (h *ThreadHandler) acceptResultProposal(e *core.RequestEvent, match, msg *core.Record, _ string) error {
	_, err := h.svc.ApplyAcceptedResult(match, league.AcceptedResult{
		Proposal: msg,
		ActorID:  e.Auth.Id,
	})
	if errors.Is(err, league.ErrMatchNotPreScore) {
		return alertError(e, "Este partido ya tiene un resultado registrado")
	}
	if err != nil {
		return alertError(e, "Error al finalizar el partido")
	}
	return nil
}

var errProposalNotPending = errors.New("proposal no longer pending")

func (h *ThreadHandler) rejectResultProposal(e *core.RequestEvent, match, msg *core.Record, proposerPairID string) error {
	counterScores, err := readScoreForm(e, match.GetString("carried_sets"), "counter_scores")
	if err != nil || counterScores == "" {
		return err
	}

	rejectedScores := msg.GetString("content")
	// One transaction: the proposal must still be pending (its author may have
	// corrected it meanwhile) and the match still pre-score, else the counter
	// would sit beside a second pending proposal.
	err = h.app.RunInTransaction(func(txApp core.App) error {
		_, freshMsg, err := lockPendingProposal(txApp, match.Id, msg.Id)
		if err != nil {
			return err
		}
		freshMsg.Set("proposal_status", "superseded")
		if err := txApp.Save(freshMsg); err != nil {
			return err
		}
		addTimelineEntry(txApp, timelineEntry{
			MatchID: match.Id, ActorID: e.Auth.Id,
			Kind: "result_response", Detail: "Resultado rechazado: " + rejectedScores,
			ParentID: msg.Id, Action: "reject", Scores: rejectedScores,
		})
		col, err := txApp.FindCollectionByNameOrId("match_messages")
		if err != nil {
			return err
		}
		pdJSON, _ := json.Marshal(league.ProposalData{Scores: counterScores})
		counter := core.NewRecord(col)
		counter.Set("match", match.Id)
		counter.Set("author", e.Auth.Id)
		counter.Set("type", "result_submission")
		counter.Set("content", counterScores)
		counter.Set("proposal_status", "pending")
		counter.Set("proposal_data", string(pdJSON))
		return txApp.Save(counter)
	})
	switch {
	case errors.Is(err, league.ErrMatchNotPreScore):
		return alertError(e, "Este partido ya tiene un resultado registrado")
	case errors.Is(err, errProposalNotPending):
		return alertError(e, "Esta propuesta ya no está pendiente. Revisa el hilo del partido.")
	case err != nil:
		return alertError(e, "Error al rechazar la propuesta")
	}

	proposerPlayers := league.PlayersForPair(h.app, proposerPairID)
	counterLabel := pairPlayerLabel(h.app, e.Auth.Id, match)
	compName := league.CompetitionName(h.app, match.GetString("competition"))
	notif := league.NotifResultCountered(match.Id, counterLabel, compName)
	h.notifier.NotifyPlayers(proposerPlayers, notif)
	return nil
}
