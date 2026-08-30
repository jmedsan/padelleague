package league

import "fmt"

// NotifResultSubmitted notifies the rival that a score was submitted.
func NotifResultSubmitted(matchID string) Notification {
	return Notification{
		Type: "quorum_request", Title: "Resultado enviado",
		Body: "Tu rival ha registrado un resultado. Confirma o disputa.", MatchID: matchID,
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
func NotifResultConfirmed(matchID string) Notification {
	return Notification{
		Type: "general", Title: "Resultado confirmado",
		Body: "Tu rival ha confirmado el resultado del partido.", MatchID: matchID,
	}
}

// NotifResultDisputed notifies the submitter that the rival disputed the score.
func NotifResultDisputed(matchID string) Notification {
	return Notification{
		Type: "general", Title: "Resultado disputado",
		Body: "Tu rival ha disputado el resultado que enviaste.", MatchID: matchID,
	}
}

// NotifResultCorrected notifies the rival that the score was corrected.
func NotifResultCorrected(matchID string) Notification {
	return Notification{
		Type: "quorum_request", Title: "Resultado corregido",
		Body: "El rival ha corregido el resultado. Confirma o disputa.", MatchID: matchID,
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

// NotifAvailability notifies the rival of an availability message.
func NotifAvailability(matchID, authorName, content string) Notification {
	return Notification{
		Type: "general", Title: "Disponibilidad",
		Body: fmt.Sprintf("%s: %s", authorName, content), MatchID: matchID,
	}
}

// NotifSchedulingReminder reminds players to arrange their match.
func NotifSchedulingReminder(matchID, levelLabel string) Notification {
	return Notification{
		Type: "scheduling", Title: "Recordatorio: organiza tu partido",
		Body: fmt.Sprintf("Tu partido está %s. Organízalo antes de que venza el plazo.", levelLabel), MatchID: matchID,
	}
}

// NotifWalkoverApproved notifies players that an admin approved a walkover.
func NotifWalkoverApproved(matchID string) Notification {
	return Notification{
		Type: "general", Title: "Walkover aprobado",
		Body: "Un administrador ha resuelto el partido como walkover.", MatchID: matchID,
	}
}

// NotifDisputeResolved notifies players that an admin resolved the dispute.
func NotifDisputeResolved(matchID string) Notification {
	return Notification{
		Type: "dispute", Title: "Disputa resuelta",
		Body: "Un administrador ha resuelto la disputa de tu partido.", MatchID: matchID,
	}
}
