package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
	"padelleague/notify"
)

// ThreadHandler handles the match thread: messages, scheduling proposals, and responses.
type ThreadHandler struct {
	app           core.App
	notifier      *notify.Notifier
	svc           *league.Service
	renderPage    RenderFunc
	renderPartial RenderFunc
}

// ThreadDeps holds the dependencies for a ThreadHandler.
type ThreadDeps struct {
	App           core.App
	Notifier      *notify.Notifier
	Svc           *league.Service
	RenderPage    RenderFunc
	RenderPartial RenderFunc
}

// NewThreadHandler creates a ThreadHandler with the given dependencies.
func NewThreadHandler(d ThreadDeps) *ThreadHandler {
	return &ThreadHandler{app: d.App, notifier: d.Notifier, svc: d.Svc, renderPage: d.RenderPage, renderPartial: d.RenderPartial}
}

// ProposalData holds parsed scheduling proposal details from a thread message.
type ProposalData struct {
	Date      string `json:"date"`
	Time      string `json:"time"`
	VenueID   string `json:"venue_id"`
	VenueName string `json:"venue_name"`
	VenueText string `json:"venue_text"`
	Scores    string `json:"scores,omitempty"`
	// Action is set on scheduling_response/result_response entries only:
	// "accept" or "reject", the decision that produced this entry.
	Action string `json:"action,omitempty"`
}

// ParseProposalData decodes a proposal from a raw JSON field value.
func ParseProposalData(raw any) *ProposalData {
	if raw == nil {
		return nil
	}
	var pd ProposalData
	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil
		}
		if err := json.Unmarshal([]byte(v), &pd); err != nil {
			return nil
		}
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		if err := json.Unmarshal(b, &pd); err != nil {
			return nil
		}
	}
	return &pd
}

// Thread renders the full match thread page with messages and proposals.
func (h *ThreadHandler) Thread(e *core.RequestEvent) error {
	matchID := e.Request.PathValue("id")
	match, err := findMatchOr404(h.app, e, matchID)
	if err != nil {
		return err
	}

	pair1ID := match.GetString("pair1")
	pair2ID := match.GetString("pair2")
	if pair1ID == "" || pair2ID == "" {
		return e.HTML(http.StatusOK, `<div class="text-center py-6 opacity-60">Parejas pendientes de asignación</div>`)
	}

	isAdmin := isEffectiveAdmin(e)
	myTeam, _ := league.PlayerTeam(h.app, e.Auth.Id, match)

	if err := checkDocGate(h.app, e, match); err != nil {
		return err
	}

	isParticipant := myTeam != 0
	isPlayoff, compModifiable := false, true
	if comp, err := h.app.FindRecordById("competitions", match.GetString("competition")); err == nil {
		isPlayoff = league.IsPlayoff(comp)
		compModifiable = isAdmin || league.PlayerCanModify(comp, time.Now())
	}
	td := h.buildThreadData(match, matchID, e.Auth.Id, myTeam, compModifiable)
	venues := findRecordsLogged(h.app, "Thread: find venues", RecordQuery{
		Collection: "venues", Filter: "id != ''", Sort: "name",
	})
	canPropose := isParticipant && league.IsPreScore(match.GetString("status")) && !isPlayoff && compModifiable

	resultCard := h.buildResultCard(match, e.Auth.Id, isAdmin, compModifiable)
	resultCard.Venues = venues

	var unpaidWarning string
	if canPropose {
		unpaidWarning = h.checkUnpaid(match, pair1ID, pair2ID)
	}

	return h.renderPartial(e, "thread.html", map[string]any{
		"MatchID":              matchID,
		"Timeline":             td.Timeline,
		"SchedProposals":       td.SchedProposals,
		"LastRejection":        td.LastRejection,
		"ResultPanel":          td.ResultPanel,
		"Card":                 resultCard,
		"Venues":               venues,
		"CanPost":              isParticipant || isAdmin,
		"CanPropose":           canPropose,
		"IsAdmin":              isAdmin,
		"IsParticipant":        isParticipant,
		"IsPlayoff":            isPlayoff,
		"CompModifiable":       compModifiable,
		"IsScheduled":          match.GetString("status") == league.StatusScheduled,
		"HasDateAndPlace":      match.GetString("date") != "" && match.GetString("club") != "",
		"Match":                match,
		"UnpaidWarning":        unpaidWarning,
		"ProposalDefaultVenue": match.GetString("club"),
		"ProposalDefaultTime":  defaultProposalTime(match),
	})
}

func defaultProposalTime(match *core.Record) string {
	if t := match.GetString("time"); t != "" {
		return t
	}
	return "20:00"
}

// buildResultCard builds the viewer's match card for the single result panel, so
// resultPanel reuses the same submit/correct/walkover capability logic — the result
// surface lives only there, never duplicated in the top card.
func (h *ThreadHandler) buildResultCard(match *core.Record, viewerID string, isAdmin, compModifiable bool) MatchCard {
	mode := PlayerFull
	if isAdmin {
		mode = AdminFull
	}
	c := NewMatchCard(h.app, match, mode, viewerID)
	if !compModifiable {
		c.CanSubmit = false
		c.CanCorrect = false
		c.CanWalkover = false
	}
	return c
}

func (h *ThreadHandler) checkUnpaid(match *core.Record, pair1ID, pair2ID string) string {
	comp, err := h.app.FindRecordById("competitions", match.GetString("competition"))
	if err != nil {
		return ""
	}
	ps := make(map[string]bool)
	_ = comp.UnmarshalJSONField("payment_status", &ps)
	if !ps[pair1ID] || !ps[pair2ID] {
		return "Una o ambas parejas no han pagado la inscripción"
	}
	return ""
}

// ThreadMessages returns the HTMX partial with updated thread messages.
func (h *ThreadHandler) ThreadMessages(e *core.RequestEvent) error {
	matchID := e.Request.PathValue("id")
	match, err := findMatchOr404(h.app, e, matchID)
	if err != nil {
		return err
	}

	isAdmin := isEffectiveAdmin(e)
	myTeam, _ := league.PlayerTeam(h.app, e.Auth.Id, match)

	compModifiable := true
	if comp, err := h.app.FindRecordById("competitions", match.GetString("competition")); err == nil {
		compModifiable = isAdmin || league.PlayerCanModify(comp, time.Now())
	}

	td := h.buildThreadData(match, matchID, e.Auth.Id, myTeam, compModifiable)

	return h.renderPartial(e, "thread-messages.html", map[string]any{
		"MatchID":  matchID,
		"Timeline": td.Timeline,
	})
}

// PostMessage handles POST to add a new chat message to the match thread.
func (h *ThreadHandler) PostMessage(e *core.RequestEvent) error {
	matchID := e.Request.PathValue("id")
	match, err := findMatchOr404(h.app, e, matchID)
	if err != nil {
		return err
	}

	if err := checkDocGate(h.app, e, match); err != nil {
		return err
	}
	if err := checkCompModifiable(h.app, e, match); err != nil {
		return err
	}

	myTeam, _ := league.PlayerTeam(h.app, e.Auth.Id, match)
	if err := checkParticipantOrAdmin(e, myTeam); err != nil {
		return err
	}

	content := strings.TrimSpace(e.Request.FormValue("content"))
	if content == "" {
		return alertError(e, "El mensaje no puede estar vacío")
	}
	content = truncateRunes(content, 2000)

	msgType := e.Request.FormValue("type")
	if msgType != "chat" && msgType != "score_discussion" {
		msgType = "chat"
	}

	col, err := h.app.FindCollectionByNameOrId("match_messages")
	if err != nil {
		return alertError(e, "Error interno")
	}

	record := core.NewRecord(col)
	record.Set("match", matchID)
	record.Set("author", e.Auth.Id)
	record.Set("type", msgType)
	record.Set("content", content)

	if err := h.app.Save(record); err != nil {
		return alertError(e, "Error al enviar mensaje")
	}

	// A player notifies the rival team; an admin (not on either team) notifies both.
	var recipients []string
	if myTeam == 0 {
		recipients = append(league.PlayersForPair(h.app, match.GetString("pair1")),
			league.PlayersForPair(h.app, match.GetString("pair2"))...)
	} else {
		rivalPairID := match.GetString("pair1")
		if myTeam == 1 {
			rivalPairID = match.GetString("pair2")
		}
		recipients = league.PlayersForPair(h.app, rivalPairID)
	}
	authorName := league.PlayerName(h.app, e.Auth.Id)
	compName := league.CompetitionName(h.app, match.GetString("competition"))
	h.notifier.NotifyPlayers(recipients, league.NotifNewMessage(matchID, authorName, content, compName))

	return redirectHX(e, "/match/"+matchID+"#mensajes")
}

// PostProposal creates a scheduling proposal message in the match thread.
func (h *ThreadHandler) PostProposal(e *core.RequestEvent) error {
	matchID := e.Request.PathValue("id")
	match, err := findMatchOr404(h.app, e, matchID)
	if err != nil {
		return err
	}

	if err := checkDocGate(h.app, e, match); err != nil {
		return err
	}

	if err := checkCompModifiable(h.app, e, match); err != nil {
		return err
	}

	if !league.IsPreScore(match.GetString("status")) {
		return alertError(e, "Solo se pueden proponer fechas para partidos pendientes")
	}

	pair1ID := match.GetString("pair1")
	pair2ID := match.GetString("pair2")
	if pair1ID == "" || pair2ID == "" {
		return alertError(e, "Este partido aun no tiene parejas asignadas")
	}

	myTeam, err := league.PlayerTeam(h.app, e.Auth.Id, match)
	if err != nil || myTeam == 0 {
		return alertError(e, "No eres participante de este partido")
	}

	var myPairID string
	if myTeam == 1 {
		myPairID = pair1ID
	} else {
		myPairID = pair2ID
	}
	if league.IsWithdrawn(h.app, myPairID, match.GetString("competition")) {
		return alertError(e, "Tu pareja se ha retirado de esta competición")
	}

	pd, err := h.parseProposalForm(e)
	if err != nil {
		return err
	}
	pdJSON, _ := json.Marshal(pd)

	if err := h.saveProposalRecord(matchID, e.Auth.Id, pdJSON); err != nil {
		return alertError(e, "Error al crear propuesta")
	}

	h.notifyProposal(match, myTeam, proposalNotice{AuthorID: e.Auth.Id, Date: pd.Date, Time: pd.Time, VenueName: pd.VenueName})
	flash(e, "Propuesta enviada")
	return redirectHX(e, "/match/"+matchID+"?scroll=mensajes")
}

func (h *ThreadHandler) saveProposalRecord(matchID, authorID string, pdJSON []byte) error {
	col, err := h.app.FindCollectionByNameOrId("match_messages")
	if err != nil {
		return err
	}
	record := core.NewRecord(col)
	record.Set("match", matchID)
	record.Set("author", authorID)
	record.Set("type", "scheduling_proposal")
	record.Set("proposal_data", string(pdJSON))
	record.Set("proposal_status", "pending")
	return h.app.Save(record)
}

type proposalNotice struct {
	AuthorID, Date, Time, VenueName string
}

func (h *ThreadHandler) notifyProposal(match *core.Record, myTeam int, n proposalNotice) {
	rivalPairID := match.GetString("pair1")
	if myTeam == 1 {
		rivalPairID = match.GetString("pair2")
	}
	rivalPlayers := league.PlayersForPair(h.app, rivalPairID)
	authorName := league.PlayerName(h.app, n.AuthorID)
	compName := league.CompetitionName(h.app, match.GetString("competition"))
	notif := league.NotifProposal(league.ProposalParams{
		MatchID: match.Id, AuthorName: authorName, Date: n.Date, Time: n.Time, VenueName: n.VenueName, CompName: compName,
	})
	h.notifier.NotifyPlayers(rivalPlayers, notif)
}

func (h *ThreadHandler) parseProposalForm(e *core.RequestEvent) (ProposalData, error) {
	date := e.Request.FormValue("date")
	timeVal := e.Request.FormValue("time")
	venueID := e.Request.FormValue("venue_id")
	venueText := e.Request.FormValue("venue_text")

	if date == "" || timeVal == "" {
		return ProposalData{}, alertError(e, "Fecha y hora son obligatorias")
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return ProposalData{}, alertError(e, "Formato de fecha no válido")
	}
	if _, err := time.Parse("15:04", timeVal); err != nil {
		return ProposalData{}, alertError(e, "Formato de hora no válido")
	}
	if parsed, _ := time.Parse("2006-01-02", date); !parsed.IsZero() && parsed.Before(time.Now().Truncate(24*time.Hour)) {
		return ProposalData{}, alertError(e, "La fecha no puede ser anterior a hoy")
	}

	venueID, venueName := h.resolveVenue(venueID, venueText)
	return ProposalData{
		Date:      date,
		Time:      timeVal,
		VenueID:   venueID,
		VenueName: venueName,
		VenueText: venueText,
	}, nil
}

// RespondProposal records a player's accept or reject on a scheduling proposal.
func (h *ThreadHandler) RespondProposal(e *core.RequestEvent) error {
	matchID := e.Request.PathValue("id")
	msgID := e.Request.PathValue("msgId")

	match, err := findMatchOr404(h.app, e, matchID)
	if err != nil {
		return err
	}

	if err := checkDocGate(h.app, e, match); err != nil {
		return err
	}

	if err := checkCompModifiable(h.app, e, match); err != nil {
		return err
	}

	if !league.IsPreScore(match.GetString("status")) {
		return alertError(e, "Este partido ya no acepta propuestas")
	}

	myTeam, err := league.PlayerTeam(h.app, e.Auth.Id, match)
	if err != nil || myTeam == 0 {
		return alertError(e, "No eres participante de este partido")
	}

	msg, err := h.app.FindRecordById("match_messages", msgID)
	if err != nil {
		return alertError(e, "Propuesta no encontrada")
	}

	if msg.GetString("match") != matchID {
		return alertError(e, "Propuesta no pertenece a este partido")
	}

	proposalStatus := msg.GetString("proposal_status")
	if proposalStatus == "accepted" {
		return redirectHX(e, "/match/"+matchID+"?scroll=mensajes")
	}
	if proposalStatus != "pending" {
		return alertError(e, "Esta propuesta ya fue respondida")
	}

	authorTeam, _ := league.PlayerTeam(h.app, msg.GetString("author"), match)
	if authorTeam == myTeam {
		return alertError(e, "No puedes responder a tu propia propuesta")
	}

	if err := h.dispatchProposalAction(e, match, msg, authorTeam); err != nil {
		return err
	}
	if msg.GetString("type") == "scheduling_proposal" {
		switch e.Request.FormValue("action") {
		case "accept":
			flash(e, "Fecha confirmada")
		case "reject":
			flash(e, "Propuesta rechazada")
		}
	}
	return redirectHX(e, "/match/"+matchID+"?scroll=mensajes")
}

// RejectAndCounterPropose rejects a pending scheduling proposal and immediately
// creates a new one in one combined action.
func (h *ThreadHandler) RejectAndCounterPropose(e *core.RequestEvent) error {
	matchID := e.Request.PathValue("id")
	msgID := e.Request.PathValue("msgId")

	match, err := findMatchOr404(h.app, e, matchID)
	if err != nil {
		return err
	}
	if err := checkDocGate(h.app, e, match); err != nil {
		return err
	}
	if err := checkCompModifiable(h.app, e, match); err != nil {
		return err
	}
	if !league.IsPreScore(match.GetString("status")) {
		return alertError(e, "Este partido ya no acepta propuestas")
	}

	myTeam, err := league.PlayerTeam(h.app, e.Auth.Id, match)
	if err != nil || myTeam == 0 {
		return alertError(e, "No eres participante de este partido")
	}

	msg, err := h.app.FindRecordById("match_messages", msgID)
	if err != nil {
		return alertError(e, "Propuesta no encontrada")
	}
	if msg.GetString("match") != matchID {
		return alertError(e, "Propuesta no pertenece a este partido")
	}
	if msg.GetString("type") != "scheduling_proposal" {
		return alertError(e, "Solo se puede contraproponer en propuestas de fecha")
	}
	if status := msg.GetString("proposal_status"); status == "accepted" {
		return redirectHX(e, "/match/"+matchID+"?scroll=mensajes")
	} else if status != "pending" {
		return alertError(e, "Esta propuesta ya fue respondida")
	}
	authorTeam, _ := league.PlayerTeam(h.app, msg.GetString("author"), match)
	if authorTeam == myTeam {
		return alertError(e, "No puedes responder a tu propia propuesta")
	}

	var myPairID string
	if myTeam == 1 {
		myPairID = match.GetString("pair1")
	} else {
		myPairID = match.GetString("pair2")
	}
	if league.IsWithdrawn(h.app, myPairID, match.GetString("competition")) {
		return alertError(e, "Tu pareja se ha retirado de esta competición")
	}

	// Validate and parse the counter-proposal form before any DB writes.
	date := e.Request.FormValue("date")
	timeVal := e.Request.FormValue("time")
	if date == "" || timeVal == "" {
		return alertError(e, "Fecha y hora son obligatorias")
	}
	pd, err := h.parseProposalForm(e)
	if err != nil || pd.Date == "" {
		return err
	}
	pdJSON, _ := json.Marshal(pd)

	proposerPairID := match.GetString("pair1")
	if authorTeam == 2 {
		proposerPairID = match.GetString("pair2")
	}

	reason := e.Request.FormValue("rejection_reason")
	text := e.Request.FormValue("rejection_text")

	// Reject old proposal + create counter-proposal atomically.
	if err := h.app.RunInTransaction(func(txApp core.App) error {
		msg.Set("proposal_status", "rejected")
		msg.Set("rejection_reason", reason)
		msg.Set("rejection_text", text)
		if err := txApp.Save(msg); err != nil {
			return err
		}

		proposerName := league.PlayerName(h.app, msg.GetString("author"))
		detail := "rechazó la propuesta de " + proposerName
		if text != "" {
			detail += ": " + text
		} else if reason != "" {
			detail += ": " + reason
		}
		addTimelineEntry(txApp, timelineEntry{
			MatchID: match.Id, ActorID: e.Auth.Id,
			Kind: "scheduling_response", Detail: detail,
			ParentID: msg.Id, Action: "reject",
			Data: ParseProposalData(msg.Get("proposal_data")),
		})

		col, err := txApp.FindCollectionByNameOrId("match_messages")
		if err != nil {
			return err
		}
		newMsg := core.NewRecord(col)
		newMsg.Set("match", matchID)
		newMsg.Set("author", e.Auth.Id)
		newMsg.Set("type", "scheduling_proposal")
		newMsg.Set("proposal_data", string(pdJSON))
		newMsg.Set("proposal_status", "pending")
		return txApp.Save(newMsg)
	}); err != nil {
		return alertError(e, "Error al procesar la contrapropuesta")
	}

	// Notifications after successful commit.
	notifReason := reason
	if reason == "Otro" && text != "" {
		notifReason = text
	} else if reason == "Otro" {
		notifReason = ""
	}
	proposerPlayers := league.PlayersForPair(h.app, proposerPairID)
	compName := league.CompetitionName(h.app, match.GetString("competition"))
	notif := league.NotifProposalRejected(match.Id, league.PlayerName(h.app, e.Auth.Id), notifReason, compName)
	h.notifier.NotifyPlayers(proposerPlayers, notif)
	h.notifyProposal(match, myTeam, proposalNotice{AuthorID: e.Auth.Id, Date: pd.Date, Time: pd.Time, VenueName: pd.VenueName})

	flash(e, "Propuesta rechazada y nueva propuesta enviada")
	return redirectHX(e, "/match/"+matchID+"?scroll=mensajes")
}

func (h *ThreadHandler) dispatchProposalAction(e *core.RequestEvent, match, msg *core.Record, authorTeam int) error {
	proposerPairID := match.GetString("pair1")
	if authorTeam == 2 {
		proposerPairID = match.GetString("pair2")
	}
	action := e.Request.FormValue("action")

	msgType := msg.GetString("type")
	if msgType == "result_submission" {
		if err := h.checkCaptainGuard(e, match); err != nil {
			return err
		}
		switch action {
		case "accept":
			return h.acceptResultProposal(e, match, msg, proposerPairID)
		case "reject":
			return h.rejectResultProposal(e, match, msg, proposerPairID)
		default:
			return alertError(e, "Acción no válida")
		}
	}

	switch action {
	case "accept":
		return h.acceptProposal(e, match, msg, proposerPairID)
	case "reject":
		return h.rejectProposal(e, msg, match, proposerPairID)
	default:
		return alertError(e, "Acción no válida")
	}
}

func (h *ThreadHandler) resolveVenue(venueID, venueText string) (string, string) {
	if venueID == "" || venueID == "otro" {
		return "", venueText
	}
	venue, err := h.app.FindRecordById("venues", venueID)
	if err != nil {
		return "", venueText
	}
	return venueID, venue.GetString("name")
}

func (h *ThreadHandler) supersedePending(matchID, excludeMsgID string) error {
	otherPending := findRecordsLogged(h.app, "supersedePending: find pending scheduling proposals", RecordQuery{
		Collection: "match_messages",
		Filter:     "match = {:mid} && type = 'scheduling_proposal' && proposal_status = 'pending' && id != {:msgid}",
		Params:     map[string]any{"mid": matchID, "msgid": excludeMsgID},
	})
	var failedIDs []string
	for _, other := range otherPending {
		other.Set("proposal_status", "superseded")
		if err := h.app.Save(other); err != nil {
			slog.Error("supersede proposal", "id", other.Id, "err", err)
			failedIDs = append(failedIDs, other.Id)
			continue
		}
		addTimelineEntry(h.app, timelineEntry{
			MatchID:  matchID,
			Kind:     "scheduling_response",
			Detail:   "propuesta sustituida por la aceptada",
			ParentID: other.Id,
			Action:   "supersede",
			Data:     ParseProposalData(other.Get("proposal_data")),
		})
	}
	if len(failedIDs) > 0 {
		return fmt.Errorf("failed to supersede proposals: %v", failedIDs)
	}
	return nil
}

func (h *ThreadHandler) supersedePendingAndNotify(match *core.Record, excludeMsgID string) {
	if err := h.supersedePending(match.Id, excludeMsgID); err != nil {
		slog.Error("supersede pending proposals", "match", match.Id, "err", err)
		pairNames := league.PairNames(h.app, []string{match.GetString("pair1"), match.GetString("pair2")})
		compName := league.CompetitionName(h.app, match.GetString("competition"))
		n := league.NotifAdminSupersedeFailed(match.Id, pairNames[match.GetString("pair1")], pairNames[match.GetString("pair2")], compName)
		_ = h.notifier.NotifyAdmins(n)
	}
}

// WithdrawProposal lets the author withdraw their own pending scheduling proposal.
func (h *ThreadHandler) WithdrawProposal(e *core.RequestEvent) error {
	matchID := e.Request.PathValue("id")
	msgID := e.Request.PathValue("msgId")

	match, err := findMatchOr404(h.app, e, matchID)
	if err != nil {
		return err
	}

	if err := checkDocGate(h.app, e, match); err != nil {
		return err
	}

	if err := checkCompModifiable(h.app, e, match); err != nil {
		return err
	}

	if !league.IsPreScore(match.GetString("status")) {
		return alertError(e, "Este partido ya no acepta propuestas")
	}

	msg, err := h.app.FindRecordById("match_messages", msgID)
	if err != nil {
		return alertError(e, "Propuesta no encontrada")
	}
	if msg.GetString("match") != matchID {
		return alertError(e, "Propuesta no pertenece a este partido")
	}
	if msg.GetString("type") != "scheduling_proposal" {
		return alertError(e, "Solo se pueden retirar propuestas de fecha")
	}
	if msg.GetString("proposal_status") != "pending" {
		return alertError(e, "Solo se pueden retirar propuestas pendientes")
	}
	authorTeam, _ := league.PlayerTeam(h.app, msg.GetString("author"), match)
	actorTeam, _ := league.PlayerTeam(h.app, e.Auth.Id, match)
	if actorTeam == 0 || authorTeam != actorTeam {
		return alertError(e, "Solo tu pareja puede retirar esta propuesta")
	}

	msg.Set("proposal_status", "withdrawn")
	if err := h.app.Save(msg); err != nil {
		return alertError(e, "Error al retirar la propuesta")
	}

	addTimelineEntry(h.app, timelineEntry{
		MatchID: match.Id, ActorID: e.Auth.Id,
		Kind:     "scheduling_response",
		Detail:   "retiró su propuesta de fecha",
		ParentID: msg.Id,
		Action:   "withdraw",
		Data:     ParseProposalData(msg.Get("proposal_data")),
	})

	myTeam, _ := league.PlayerTeam(h.app, e.Auth.Id, match)
	h.notifyWithdrawal(match, myTeam, e.Auth.Id)
	return redirectHX(e, "/match/"+matchID+"?scroll=mensajes")
}

// checkCaptainGuard returns an alertError if the request is not from an admin
// and the user is not their pair's captain. Returns nil when the check passes.
func (h *ThreadHandler) checkCaptainGuard(e *core.RequestEvent, match *core.Record) error {
	if isEffectiveAdmin(e) {
		return nil
	}
	if err := league.IsCaptainGuarded(h.app, e.Auth.Id, match); err != nil {
		if errors.Is(err, league.ErrNotCaptain) {
			return alertError(e, "Solo el capitán puede registrar resultados")
		}
		return alertError(e, "Error interno")
	}
	return nil
}

func (h *ThreadHandler) notifyWithdrawal(match *core.Record, myTeam int, authorID string) {
	rivalPairID := match.GetString("pair1")
	if myTeam == 1 {
		rivalPairID = match.GetString("pair2")
	}
	rivalPlayers := league.PlayersForPair(h.app, rivalPairID)
	authorName := league.PlayerName(h.app, authorID)
	compName := league.CompetitionName(h.app, match.GetString("competition"))
	h.notifier.NotifyPlayers(rivalPlayers, league.NotifProposalWithdrawn(match.Id, authorName, compName))
}
