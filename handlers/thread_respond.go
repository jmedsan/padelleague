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

	proposerName := league.PlayerName(h.app, msg.GetString("author"))
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
		MatchID: match.Id, ResponderName: league.PlayerName(h.app, e.Auth.Id), Date: pd.Date, Time: pd.Time, CompName: compName,
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

	proposerName := league.PlayerName(h.app, msg.GetString("author"))
	detail := "rechazó la propuesta de " + proposerName
	if text != "" {
		detail += ": " + text
	} else if reason != "" {
		detail += ": " + reason
	}
	addTimelineEntry(h.app, timelineEntry{
		MatchID: match.Id, ActorID: e.Auth.Id,
		Kind: "scheduling_response", Detail: detail,
		ParentID: msg.Id, Action: "reject",
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
	notif := league.NotifProposalRejected(match.Id, league.PlayerName(h.app, e.Auth.Id), notifReason, compName)
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

func (h *ThreadHandler) rejectResultProposal(e *core.RequestEvent, match, msg *core.Record, proposerPairID string) error {
	counterScores, err := readScoreForm(e, match.GetString("carried_sets"), "counter_scores")
	if err != nil {
		return err
	}

	msg.Set("proposal_status", "superseded")
	if err := h.app.Save(msg); err != nil {
		return alertError(e, "Error al rechazar la propuesta")
	}

	rejectedScores := msg.GetString("content")
	addTimelineEntry(h.app, timelineEntry{
		MatchID: match.Id, ActorID: e.Auth.Id,
		Kind: "result_response", Detail: "Resultado rechazado: " + rejectedScores,
		ParentID: msg.Id, Action: "reject", Scores: rejectedScores,
	})

	col, err := h.app.FindCollectionByNameOrId("match_messages")
	if err != nil {
		return alertError(e, "Error interno")
	}
	pdJSON, _ := json.Marshal(ProposalData{Scores: counterScores})
	counter := core.NewRecord(col)
	counter.Set("match", match.Id)
	counter.Set("author", e.Auth.Id)
	counter.Set("type", "result_submission")
	counter.Set("content", counterScores)
	counter.Set("proposal_status", "pending")
	counter.Set("proposal_data", string(pdJSON))
	if err := h.app.Save(counter); err != nil {
		return alertError(e, "Error al crear la contrapropuesta")
	}

	proposerPlayers := league.PlayersForPair(h.app, proposerPairID)
	counterPairID := match.GetString("pair1")
	if counterPairID == proposerPairID {
		counterPairID = match.GetString("pair2")
	}
	counterPairName := league.PairNames(h.app, []string{counterPairID})[counterPairID]
	compName := league.CompetitionName(h.app, match.GetString("competition"))
	notif := league.NotifResultCountered(match.Id, counterPairName, compName)
	h.notifier.NotifyPlayers(proposerPlayers, notif)
	return nil
}
