package league

import (
	"fmt"
	"time"
)

// NotifResultSubmitted notifies the rival that a score was submitted.
func NotifResultSubmitted(matchID, opponent, compName, score string) Notification {
	return Notification{
		Type:     "quorum_request",
		Title:    "Resultado enviado",
		Body:     fmt.Sprintf("%s ha enviado %s. Confirma o contrapropón.", opponent, score),
		MatchID:  matchID,
		CompName: compName,
	}
}

// NotifMatchReportedUnplayed notifies the rival that the match was reported unplayed.
func NotifMatchReportedUnplayed(matchID, compName string) Notification {
	return Notification{
		Type: "general", Title: "Partido reportado como no jugado",
		Body:     "Tu rival ha reportado este partido como no jugado. Un administrador lo revisará.",
		MatchID:  matchID,
		CompName: compName,
	}
}

// NotifResultConfirmed notifies the submitter that the rival confirmed the score.
func NotifResultConfirmed(matchID, opponent, compName string) Notification {
	return Notification{
		Type:     "general",
		Title:    "Resultado confirmado",
		Body:     fmt.Sprintf("%s ha confirmado el resultado", opponent),
		MatchID:  matchID,
		CompName: compName,
	}
}

// NotifResultCorrected notifies the rival that the score was corrected.
func NotifResultCorrected(matchID, opponent, compName string) Notification {
	return Notification{
		Type:     "quorum_request",
		Title:    "Resultado corregido",
		Body:     fmt.Sprintf("%s ha corregido el resultado", opponent),
		MatchID:  matchID,
		CompName: compName,
	}
}

// NotifResultCountered notifies the original proposer that their result was countered.
func NotifResultCountered(matchID, opponent, compName string) Notification {
	return Notification{
		Type:     "quorum_request",
		Title:    "Contrapropuesta recibida",
		Body:     fmt.Sprintf("%s ha propuesto un resultado alternativo", opponent),
		MatchID:  matchID,
		CompName: compName,
	}
}

// NotifNewMessage notifies the rival of a new thread message.
func NotifNewMessage(matchID, authorName, content, compName string) Notification {
	return Notification{
		Type: "message", Title: "Nuevo mensaje",
		Body:     fmt.Sprintf("%s escribió: %s", authorName, Truncate(content, 60)),
		MatchID:  matchID,
		CompName: compName,
	}
}

// ProposalParams holds the dynamic parts for a scheduling proposal notification.
type ProposalParams struct {
	MatchID, AuthorName, Date, Time, VenueName, CompName string
}

// NotifProposal notifies the rival of a scheduling proposal.
func NotifProposal(p ProposalParams) Notification {
	return Notification{
		Type: "scheduling", Title: "Propuesta de fecha",
		Body:     proposalBody(p.AuthorName, fmtNotifDate(p.Date, p.Time), p.VenueName),
		MatchID:  p.MatchID,
		CompName: p.CompName,
	}
}

func proposalBody(author, dateTime, venue string) string {
	if venue == "" {
		return fmt.Sprintf("%s propone jugar %s", author, dateTime)
	}
	return fmt.Sprintf("%s propone jugar %s en %s", author, dateTime, venue)
}

// ProposalAcceptedParams holds the dynamic parts for a proposal-accepted notification.
type ProposalAcceptedParams struct {
	MatchID, ResponderName, Date, Time, CompName string
}

// NotifProposalAccepted notifies the proposer that their proposal was accepted.
func NotifProposalAccepted(p ProposalAcceptedParams) Notification {
	return Notification{
		Type: "scheduling", Title: "Propuesta aceptada",
		Body:     fmt.Sprintf("%s aceptó tu propuesta para %s", p.ResponderName, fmtNotifDate(p.Date, p.Time)),
		MatchID:  p.MatchID,
		CompName: p.CompName,
	}
}

// NotifProposalWithdrawn notifies the rival that a scheduling proposal was withdrawn.
func NotifProposalWithdrawn(matchID, authorName, compName string) Notification {
	return Notification{
		Type: "scheduling", Title: "Propuesta retirada",
		Body:     authorName + " ha retirado su propuesta de fecha",
		MatchID:  matchID,
		CompName: compName,
	}
}

// NotifProposalRejected notifies the proposer that their proposal was rejected.
func NotifProposalRejected(matchID, responderName, reason, compName string) Notification {
	body := responderName + " ha rechazado tu propuesta"
	if reason != "" {
		body += ": " + reason
	}
	return Notification{
		Type: "scheduling", Title: "Propuesta rechazada",
		Body:     body,
		MatchID:  matchID,
		CompName: compName,
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
		Type:     "general",
		Title:    "Incomparecencia aprobada",
		Body:     "El administrador ha aprobado la incomparecencia",
		MatchID:  matchID,
		CompName: compName,
	}
}

// NotifDisputeResolved notifies players that an admin resolved the dispute.
func NotifDisputeResolved(matchID, compName string) Notification {
	return Notification{
		Type:     "dispute",
		Title:    "Disputa resuelta",
		Body:     "El administrador ha resuelto la disputa",
		MatchID:  matchID,
		CompName: compName,
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
func NotifAdminMatchUnplayed(matchID, compName string) Notification {
	return Notification{
		Type: "dispute", Title: "Partido no jugado",
		Body:     "Un jugador ha reportado un partido como no jugado.",
		MatchID:  matchID,
		CompName: compName,
	}
}

// NotifAdminSupersedeFailed alerts admins that pending proposals could not be superseded.
func NotifAdminSupersedeFailed(matchID, pair1Name, pair2Name, compName string) Notification {
	return Notification{
		Type: "admin_message", Title: "Propuestas pendientes no actualizadas",
		Body:     fmt.Sprintf("El partido %s vs %s tiene propuestas que no se pudieron marcar como superadas. Revisa el hilo.", pair1Name, pair2Name),
		MatchID:  matchID,
		CompName: compName,
	}
}

// NotifCalendarPublished notifies a player that a competition's calendar was
// published and its matches are now visible to them.
func NotifCalendarPublished(compID, compName string) Notification {
	return Notification{
		Type:     "calendar_published",
		Title:    "Calendario publicado",
		Body:     "El calendario ha sido publicado.",
		Link:     "/competition/" + compID,
		CompName: compName,
	}
}

// MatchUpcomingParams holds the dynamic parts for an upcoming-match reminder.
type MatchUpcomingParams struct {
	MatchID, Venue, CompName, Opponent string
	Start                              time.Time
	Until                              time.Duration
}

// NotifMatchUpcoming is the time-relative upcoming-match reminder.
func NotifMatchUpcoming(p MatchUpcomingParams) Notification {
	dateStr := p.Start.In(Madrid).Format("02/01")
	timeStr := p.Start.In(Madrid).Format("15:04")

	var title, body string
	if p.Until > 2*time.Hour {
		title = "Próximo partido"
		body = fmt.Sprintf("Tu partido vs %s es el %s a las %s en %s.", p.Opponent, dateStr, timeStr, p.Venue)
	} else {
		title = "Tu partido empieza pronto"
		remaining := formatRemaining(p.Until)
		body = fmt.Sprintf("Tu partido vs %s empieza %s · %s en %s.", p.Opponent, remaining, timeStr, p.Venue)
	}

	return Notification{
		Type:     "match_reminder",
		Title:    title,
		Body:     body,
		MatchID:  p.MatchID,
		CompName: p.CompName,
	}
}

func formatRemaining(d time.Duration) string {
	totalMin := int(d.Minutes())
	hours := totalMin / 60
	mins := totalMin % 60
	if hours >= 2 {
		return fmt.Sprintf("en %d horas", hours)
	}
	if hours == 1 && mins > 0 {
		return fmt.Sprintf("en 1 hora y %d minutos", mins)
	}
	if hours == 1 {
		return "en 1 hora"
	}
	if mins == 0 {
		return "en menos de 1 minuto"
	}
	return fmt.Sprintf("en %d minutos", mins)
}

// NotifOpponentWithdrawn notifies the opponents of a withdrawn pair per match.
func NotifOpponentWithdrawn(matchID, pairName, compName, woScore string) Notification {
	return Notification{
		Type:     "general",
		Title:    "Pareja retirada",
		Body:     fmt.Sprintf("La pareja %s se ha retirado. El partido se registra como %s a tu favor.", pairName, woScore),
		MatchID:  matchID,
		CompName: compName,
	}
}

// NotifPairWithdrawn notifies the players of the pair that was withdrawn.
func NotifPairWithdrawn(compName string) Notification {
	return Notification{
		Type:     "general",
		Title:    "Retirada de la competición",
		Body:     fmt.Sprintf("Tu pareja ha sido retirada de %s.", compName),
		CompName: compName,
	}
}

// NotifAdminUserJoined alerts admins that a new user registered.
func NotifAdminUserJoined(displayName string) Notification {
	return Notification{
		Type:  "user_joined",
		Title: "Nuevo jugador registrado",
		Body:  fmt.Sprintf("%s se ha registrado en la liga.", displayName),
		Link:  "/admin/players",
	}
}

// fmtNotifDate formats a scheduling proposal's raw date/time (from an HTML
// date input "2006-01-02" and time input "15:04") into Spanish notification
// text: "el DD/MM a las HH:MM", or "el DD/MM/YYYY" when timeStr is empty.
// Falls back to the raw dateStr if it doesn't parse.
func fmtNotifDate(dateStr, timeStr string) string {
	d, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}
	if timeStr == "" {
		return "el " + d.Format("02/01/2006")
	}
	return fmt.Sprintf("el %s a las %s", d.Format("02/01"), timeStr)
}
