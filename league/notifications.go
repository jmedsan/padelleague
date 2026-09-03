package league

import (
	"fmt"
	"time"
)

// NotifResultSubmitted notifies the rival that a score was submitted.
func NotifResultSubmitted(matchID, opponent, compName, score string) Notification {
	return Notification{
		Type: "quorum_request", Title: "Resultado enviado",
		Body:    fmt.Sprintf("%s ha enviado %s · %s. Confirma o contrapropón.", opponent, score, compName),
		MatchID: matchID,
	}
}

// NotifMatchReportedUnplayed notifies the rival that the match was reported unplayed.
func NotifMatchReportedUnplayed(matchID string) Notification {
	return Notification{
		Type: "general", Title: "Partido reportado como no jugado",
		Body: "Tu rival ha reportado este partido como no jugado. Un administrador lo revisará.", MatchID: matchID,
	}
}

// NotifResultConfirmed notifies the submitter that the rival confirmed the score.
func NotifResultConfirmed(matchID, opponent, compName string) Notification {
	return Notification{
		Type: "general", Title: "Resultado confirmado",
		Body:    fmt.Sprintf("%s ha confirmado el resultado · %s.", opponent, compName),
		MatchID: matchID,
	}
}

// NotifResultCorrected notifies the rival that the score was corrected.
func NotifResultCorrected(matchID, opponent, compName string) Notification {
	return Notification{
		Type: "quorum_request", Title: "Resultado corregido",
		Body:    fmt.Sprintf("%s ha corregido el resultado · %s.", opponent, compName),
		MatchID: matchID,
	}
}

// NotifResultCountered notifies the original proposer that their result was countered.
func NotifResultCountered(matchID, opponent, compName string) Notification {
	return Notification{
		Type: "quorum_request", Title: "Contrapropuesta recibida",
		Body:    fmt.Sprintf("%s ha propuesto un resultado alternativo · %s.", opponent, compName),
		MatchID: matchID,
	}
}

// NotifNewMessage notifies the rival of a new thread message.
func NotifNewMessage(matchID, authorName, content string) Notification {
	return Notification{
		Type: "general", Title: "Nuevo mensaje",
		Body: fmt.Sprintf("%s escribió: %s", authorName, Truncate(content, 60)), MatchID: matchID,
	}
}

// ProposalParams holds the dynamic parts for a scheduling proposal notification.
type ProposalParams struct {
	MatchID, AuthorName, Date, Time, VenueName string
}

// NotifProposal notifies the rival of a scheduling proposal.
func NotifProposal(p ProposalParams) Notification {
	return Notification{
		Type: "scheduling", Title: "Propuesta de fecha",
		Body: fmt.Sprintf("%s propone jugar el %s a las %s en %s", p.AuthorName, p.Date, p.Time, p.VenueName), MatchID: p.MatchID,
	}
}

// NotifProposalAccepted notifies the proposer that their proposal was accepted.
func NotifProposalAccepted(matchID, responderName, date, timeStr string) Notification {
	return Notification{
		Type: "scheduling", Title: "Propuesta aceptada",
		Body: fmt.Sprintf("%s aceptó tu propuesta para el %s a las %s", responderName, date, timeStr), MatchID: matchID,
	}
}

// NotifProposalRejected notifies the proposer that their proposal was rejected.
func NotifProposalRejected(matchID, responderName, reason string) Notification {
	return Notification{
		Type: "scheduling", Title: "Propuesta rechazada",
		Body: fmt.Sprintf("%s rechazó tu propuesta: %s", responderName, reason), MatchID: matchID,
	}
}

// NotifDecisionChangedToRejected notifies the proposer of a revoked acceptance.
func NotifDecisionChangedToRejected(matchID, responderName string) Notification {
	return Notification{
		Type: "scheduling", Title: "Decisión cambiada",
		Body: fmt.Sprintf("%s cambió su decisión: propuesta ahora rechazada", responderName), MatchID: matchID,
	}
}

// NotifDecisionChangedToAccepted notifies the proposer of a changed-to-accepted decision.
func NotifDecisionChangedToAccepted(matchID, responderName, date, timeStr string) Notification {
	return Notification{
		Type: "scheduling", Title: "Decisión cambiada",
		Body: fmt.Sprintf("%s cambió su decisión: propuesta ahora aceptada para el %s a las %s", responderName, date, timeStr), MatchID: matchID,
	}
}

// SchedulingReminderParams holds the dynamic parts for a scheduling reminder notification.
type SchedulingReminderParams struct {
	MatchID, Opponent, CompName string
	Deadline                    time.Time
	Level                       Warning
}

// NotifSchedulingReminder reminds players to arrange their match.
func NotifSchedulingReminder(p SchedulingReminderParams) Notification {
	urgency := ""
	switch p.Level {
	case WarnOverdue:
		urgency = " El plazo ha vencido."
	case WarnUrgent:
		urgency = " Quedan pocos días."
	}
	return Notification{
		Type: "scheduling", Title: "Recordatorio: organiza tu partido",
		Body:    fmt.Sprintf("Tu partido vs %s · %s vence el %s.%s", p.Opponent, p.CompName, fmtShortDate(p.Deadline), urgency),
		MatchID: p.MatchID,
	}
}

// NotifWalkoverApproved notifies players that an admin approved a walkover.
func NotifWalkoverApproved(matchID, compName string) Notification {
	return Notification{
		Type: "general", Title: "Incomparecencia aprobada",
		Body:    fmt.Sprintf("Incomparecencia aprobada · %s.", compName),
		MatchID: matchID,
	}
}

// NotifDisputeResolved notifies players that an admin resolved the dispute.
func NotifDisputeResolved(matchID, compName string) Notification {
	return Notification{
		Type: "dispute", Title: "Disputa resuelta",
		Body:    fmt.Sprintf("Disputa resuelta · %s.", compName),
		MatchID: matchID,
	}
}

// NotifAdminMatchProgress alerts admins of match score activity (submit or confirm).
func NotifAdminMatchProgress(matchID, summary string) Notification {
	return Notification{
		Type: "match_progress", Title: "Progreso de partido",
		Body: summary, MatchID: matchID,
	}
}

// NotifAdminMatchUnplayed alerts admins that a player reported a match unplayed.
func NotifAdminMatchUnplayed(matchID string) Notification {
	return Notification{
		Type: "dispute", Title: "Partido no jugado",
		Body: "Un jugador ha reportado un partido como no jugado.", MatchID: matchID,
	}
}

// NotifAdminSupersedeFailed alerts admins that pending proposals could not be superseded.
func NotifAdminSupersedeFailed(matchID, pair1Name, pair2Name string) Notification {
	return Notification{
		Type: "admin_message", Title: "Propuestas pendientes no actualizadas",
		Body:    fmt.Sprintf("El partido %s vs %s tiene propuestas que no se pudieron marcar como superadas. Revisa el hilo.", pair1Name, pair2Name),
		MatchID: matchID,
	}
}

// NotifAdminPlayoffAdvanceFailed alerts admins that automatic playoff advancement failed.
func NotifAdminPlayoffAdvanceFailed(matchID string) Notification {
	return Notification{
		Type: "admin_message", Title: "Error en avance de playoff",
		Body:    "El partido finalizó pero el bracket no avanzó automáticamente. Revisa el panel de administración.",
		MatchID: matchID,
	}
}

// NotifMatchAssigned notifies a player that fixtures were generated for a
// competition they're playing in.
func NotifMatchAssigned(compName string) Notification {
	return Notification{
		Type:  "match_assigned",
		Title: "Calendario disponible",
		Body:  fmt.Sprintf("Ya tienes calendario en %s.", compName),
	}
}

// NotifyFixturesGenerated sends a match_assigned notification to every player
// in the given pairs, once fixtures have been generated for a competition.
func (svc *Service) NotifyFixturesGenerated(compID string, pairIDs []string) {
	compName := CompetitionName(svc.app, compID)
	seen := make(map[string]struct{}, len(pairIDs)*2)
	var players []string
	for _, pid := range pairIDs {
		for _, uid := range PlayersForPair(svc.app, pid) {
			if _, ok := seen[uid]; ok {
				continue
			}
			seen[uid] = struct{}{}
			players = append(players, uid)
		}
	}
	svc.notifier.NotifyPlayers(players, NotifMatchAssigned(compName))
}

// NotifMatchReminder reminds players that their match is scheduled for the
// next day.
func NotifMatchReminder(matchID, timeStr, venueName string) Notification {
	return Notification{
		Type:    "scheduling",
		Title:   "Partido mañana",
		Body:    fmt.Sprintf("Tu partido es mañana a las %s en %s.", timeStr, venueName),
		MatchID: matchID,
	}
}
