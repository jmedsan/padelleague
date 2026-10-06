package handlers

import (
	"encoding/json"
	"errors"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
)

func (h *ThreadHandler) acceptProposal(e *core.RequestEvent, match, msg *core.Record, _ string) error {
	existing := findRecordsLogged(h.app, "acceptProposal: find accepted proposals", RecordQuery{
		Collection: "match_messages", Filter: "match = {:mid} && type = 'scheduling_proposal' && proposal_status = 'accepted'",
		Params: map[string]any{"mid": match.Id},
	})
	if len(existing) > 0 && match.GetString("status") != league.StatusScheduled {
		return alertError(e, "Ya hay una propuesta aceptada para este partido")
	}

	pd := ParseProposalData(msg.Get("proposal_data"))
	if pd == nil {
		return alertError(e, "Error al leer los datos de la propuesta")
	}

	proposerName := pairPlayerLabel(h.app, msg.GetString("author"), match)
	if err := h.app.RunInTransaction(func(txApp core.App) error {
		for _, old := range existing {
			old.Set("proposal_status", "superseded")
			if err := txApp.Save(old); err != nil {
				return err
			}
		}
		match.Set("date", pd.Date)
		match.Set("time", pd.Time)
		match.Set("club", pd.VenueName)
		match.Set("status", league.StatusScheduled)
		league.ClearMatchReminders(txApp, match.Id)
		if err := txApp.Save(match); err != nil {
			return err
		}
		msg.Set("proposal_status", "accepted")
		if err := txApp.Save(msg); err != nil {
			return err
		}
		addTimelineEntry(txApp, timelineEntry{
			MatchID: match.Id, ActorID: e.Auth.Id,
			Kind:     "scheduling_response",
			Detail:   "aceptó la propuesta de " + proposerName + " (" + pd.Date + ", " + pd.Time + ", " + pd.VenueName + ")",
			ParentID: msg.Id,
			Action:   "accept",
			Data:     pd,
		})
		return nil
	}); err != nil {
		return alertError(e, "Error al aceptar la propuesta")
	}

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

	msg.Set("proposal_status", "rejected")
	msg.Set("rejection_reason", reason)
	msg.Set("rejection_text", text)
	if err := h.app.Save(msg); err != nil {
		return alertError(e, "Error al rechazar la propuesta")
	}

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
	addTimelineEntry(h.app, timelineEntry{
		MatchID: match.Id, ActorID: e.Auth.Id,
		Kind: "scheduling_response", Detail: detail,
		ParentID: msg.Id, Action: "reject", Note: note,
		Data: ParseProposalData(msg.Get("proposal_data")),
	})

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
		freshMatch, err := txApp.FindRecordById("matches", match.Id)
		if err != nil {
			return err
		}
		if !league.IsPreScore(freshMatch.GetString("status")) {
			return league.ErrMatchNotPreScore
		}
		freshMsg, err := txApp.FindRecordById("match_messages", msg.Id)
		if err != nil {
			return err
		}
		if freshMsg.GetString("proposal_status") != "pending" {
			return errProposalNotPending
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
		pdJSON, _ := json.Marshal(ProposalData{Scores: counterScores})
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
