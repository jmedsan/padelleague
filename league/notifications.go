package league

import "fmt"

func NotifResultSubmitted(matchID string) Notification {
	return Notification{
		Type: "quorum_request", Title: "Resultado enviado",
		Body: "Tu rival ha registrado un resultado. Confirma o disputa.", MatchID: matchID,
	}
}

func NotifMatchReportedUnplayed(matchID string) Notification {
	return Notification{
		Type: "general", Title: "Partido reportado como no jugado",
		Body: "Tu rival ha reportado este partido como no jugado. Un administrador lo revisará.", MatchID: matchID,
	}
}

func NotifResultConfirmed(matchID string) Notification {
	return Notification{
		Type: "general", Title: "Resultado confirmado",
		Body: "Tu rival ha confirmado el resultado del partido.", MatchID: matchID,
	}
}

func NotifResultDisputed(matchID string) Notification {
	return Notification{
		Type: "general", Title: "Resultado disputado",
		Body: "Tu rival ha disputado el resultado que enviaste.", MatchID: matchID,
	}
}

func NotifResultCorrected(matchID string) Notification {
	return Notification{
		Type: "quorum_request", Title: "Resultado corregido",
		Body: "El rival ha corregido el resultado. Confirma o disputa.", MatchID: matchID,
	}
}

func NotifNewMessage(matchID, authorName, content string) Notification {
	return Notification{
		Type: "general", Title: "Nuevo mensaje",
		Body: fmt.Sprintf("%s escribió: %s", authorName, Truncate(content, 60)), MatchID: matchID,
	}
}

func NotifProposal(matchID, authorName, date, timeStr, venueName string) Notification {
	return Notification{
		Type: "scheduling", Title: "Propuesta de fecha",
		Body: fmt.Sprintf("%s propone jugar el %s a las %s en %s", authorName, date, timeStr, venueName), MatchID: matchID,
	}
}

func NotifProposalAccepted(matchID, responderName, date, timeStr string) Notification {
	return Notification{
		Type: "scheduling", Title: "Propuesta aceptada",
		Body: fmt.Sprintf("%s aceptó tu propuesta para el %s a las %s", responderName, date, timeStr), MatchID: matchID,
	}
}

func NotifProposalRejected(matchID, responderName, reason string) Notification {
	return Notification{
		Type: "scheduling", Title: "Propuesta rechazada",
		Body: fmt.Sprintf("%s rechazó tu propuesta: %s", responderName, reason), MatchID: matchID,
	}
}

func NotifDecisionChangedToRejected(matchID, responderName string) Notification {
	return Notification{
		Type: "scheduling", Title: "Decisión cambiada",
		Body: fmt.Sprintf("%s cambió su decisión: propuesta ahora rechazada", responderName), MatchID: matchID,
	}
}

func NotifDecisionChangedToAccepted(matchID, responderName, date, timeStr string) Notification {
	return Notification{
		Type: "scheduling", Title: "Decisión cambiada",
		Body: fmt.Sprintf("%s cambió su decisión: propuesta ahora aceptada para el %s a las %s", responderName, date, timeStr), MatchID: matchID,
	}
}

func NotifAvailability(matchID, authorName, content string) Notification {
	return Notification{
		Type: "general", Title: "Disponibilidad",
		Body: fmt.Sprintf("%s: %s", authorName, content), MatchID: matchID,
	}
}

func NotifSchedulingReminder(matchID, levelLabel string) Notification {
	return Notification{
		Type: "scheduling", Title: "Recordatorio: organiza tu partido",
		Body: fmt.Sprintf("Tu partido está %s. Organízalo antes de que venza el plazo.", levelLabel), MatchID: matchID,
	}
}

func NotifWalkoverApproved(matchID string) Notification {
	return Notification{
		Type: "general", Title: "Walkover aprobado",
		Body: "Un administrador ha resuelto el partido como walkover.", MatchID: matchID,
	}
}

func NotifDisputeResolved(matchID string) Notification {
	return Notification{
		Type: "dispute", Title: "Disputa resuelta",
		Body: "Un administrador ha resuelto la disputa de tu partido.", MatchID: matchID,
	}
}
