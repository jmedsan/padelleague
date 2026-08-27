package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
	"padelleague/notify"
)

// ThreadHandler handles the match thread: messages, scheduling proposals, and responses.
type ThreadHandler struct {
	app           core.App
	notifier      *notify.Notifier
	renderPage    RenderFunc
	renderPartial RenderFunc
}

// NewThreadHandler creates a ThreadHandler with the given dependencies.
func NewThreadHandler(app core.App, notifier *notify.Notifier, renderPage RenderFunc, renderPartial RenderFunc) *ThreadHandler {
	return &ThreadHandler{app: app, notifier: notifier, renderPage: renderPage, renderPartial: renderPartial}
}

// ThreadMessage holds a thread message record with display-ready fields.
type ThreadMessage struct {
	Record            *core.Record
	AuthorName        string
	AuthorTeam        int
	IsMyTeam          bool
	Type              string
	Content           string
	ProposalData      *ProposalData
	Status            string
	RejectionReason   string
	RejectionText     string
	CanRespond        bool
	CanChangeDecision bool
	CreatedAt         string
}

// ProposalData holds parsed scheduling proposal details from a thread message.
type ProposalData struct {
	Date      string `json:"date"`
	Time      string `json:"time"`
	VenueID   string `json:"venue_id"`
	VenueName string `json:"venue_name"`
	VenueText string `json:"venue_text"`
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

func (h *ThreadHandler) buildThreadMessages(match *core.Record, matchID string, myTeam int) []ThreadMessage {
	messages, _ := h.app.FindRecordsByFilter("match_messages",
		"match = {:mid}", "created", 0, 0,
		map[string]any{"mid": matchID})

	pair1Players := league.PlayersForPair(h.app, match.GetString("pair1"))
	pair2Players := league.PlayersForPair(h.app, match.GetString("pair2"))
	nameCache := make(map[string]string)

	var threadMessages []ThreadMessage
	for _, msg := range messages {
		authorID := msg.GetString("author")
		authorTeam := playerTeamOf(authorID, pair1Players, pair2Players)

		if _, ok := nameCache[authorID]; !ok {
			nameCache[authorID] = league.PlayerName(h.app, authorID)
		}

		msgType := msg.GetString("type")
		var pd *ProposalData
		if msgType == "scheduling_proposal" {
			pd = ParseProposalData(msg.GetString("proposal_data"))
		}

		status := msg.GetString("proposal_status")
		canRespond, canChangeDecision := proposalActions(msgType, match.GetString("status"), authorTeam == myTeam || myTeam == 0, status)

		threadMessages = append(threadMessages, ThreadMessage{
			Record:            msg,
			AuthorName:        nameCache[authorID],
			AuthorTeam:        authorTeam,
			IsMyTeam:          myTeam != 0 && authorTeam == myTeam,
			Type:              msgType,
			Content:           msg.GetString("content"),
			ProposalData:      pd,
			Status:            status,
			RejectionReason:   msg.GetString("rejection_reason"),
			RejectionText:     msg.GetString("rejection_text"),
			CanRespond:        canRespond,
			CanChangeDecision: canChangeDecision,
			CreatedAt:         msg.GetDateTime("created").Time().Format("02/01 15:04"),
		})
	}
	return threadMessages
}

func playerTeamOf(uid string, pair1Players, pair2Players []string) int {
	for _, p := range pair1Players {
		if p == uid {
			return 1
		}
	}
	for _, p := range pair2Players {
		if p == uid {
			return 2
		}
	}
	return 0
}

func proposalActions(msgType, matchStatus string, sameTeamOrOutsider bool, proposalStatus string) (canRespond, canChange bool) {
	canAct := msgType == "scheduling_proposal" &&
		matchStatus == league.StatusPending &&
		!sameTeamOrOutsider
	return canAct && proposalStatus == "pending",
		canAct && (proposalStatus == "accepted" || proposalStatus == "rejected")
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
		return e.HTML(http.StatusOK, `<div class="text-center py-6 opacity-60">Parejas pendientes de asignacion</div>`)
	}

	isAdmin := slices.Contains(e.Auth.GetStringSlice("roles"), "admin")
	myTeam, teamErr := league.PlayerTeam(h.app, e.Auth.Id, match)
	if teamErr != nil && !isAdmin {
		return alertError(e, "No tienes acceso a este hilo")
	}

	threadMessages := h.buildThreadMessages(match, matchID, myTeam)

	venues, _ := h.app.FindRecordsByFilter("venues",
		"id != ''", "name", 0, 0, nil)

	isParticipant := myTeam != 0
	canPost := isParticipant
	isPlayoff := false
	if comp, err := h.app.FindRecordById("competitions", match.GetString("competition")); err == nil {
		isPlayoff = league.IsPlayoff(comp)
	}
	canPropose := canPost && match.GetString("status") == league.StatusPending && !isPlayoff

	return h.renderPartial(e, "thread.html", map[string]any{
		"MatchID":       matchID,
		"Messages":      threadMessages,
		"Venues":        venues,
		"CanPost":       canPost,
		"CanPropose":    canPropose,
		"IsAdmin":       isAdmin,
		"IsParticipant": isParticipant,
		"IsPlayoff":     isPlayoff,
		"Match":         match,
	})
}

// ThreadMessages returns the HTMX partial with updated thread messages.
func (h *ThreadHandler) ThreadMessages(e *core.RequestEvent) error {
	matchID := e.Request.PathValue("id")
	match, err := findMatchOr404(h.app, e, matchID)
	if err != nil {
		return err
	}

	isAdmin := slices.Contains(e.Auth.GetStringSlice("roles"), "admin")
	myTeam, teamErr := league.PlayerTeam(h.app, e.Auth.Id, match)
	if teamErr != nil && !isAdmin {
		return alertError(e, "No tienes acceso a este hilo")
	}

	threadMessages := h.buildThreadMessages(match, matchID, myTeam)

	return h.renderPartial(e, "thread-messages.html", map[string]any{
		"MatchID":  matchID,
		"Messages": threadMessages,
	})
}

// PostMessage handles POST to add a new chat message to the match thread.
func (h *ThreadHandler) PostMessage(e *core.RequestEvent) error {
	matchID := e.Request.PathValue("id")
	match, err := findMatchOr404(h.app, e, matchID)
	if err != nil {
		return err
	}

	myTeam, err := league.PlayerTeam(h.app, e.Auth.Id, match)
	if err != nil || myTeam == 0 {
		return alertError(e, "No eres participante de este partido")
	}

	content := e.Request.FormValue("content")
	if content == "" {
		return alertError(e, "El mensaje no puede estar vacío")
	}

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

	rivalPairID := match.GetString("pair1")
	if myTeam == 1 {
		rivalPairID = match.GetString("pair2")
	}
	rivalPlayers := league.PlayersForPair(h.app, rivalPairID)
	authorName := league.PlayerName(h.app, e.Auth.Id)
	h.notifier.NotifyPlayers(rivalPlayers, league.Notification{
		Type: "general", Title: "Nuevo mensaje",
		Body: fmt.Sprintf("%s escribió: %s", authorName, league.Truncate(content, 60)), MatchID: matchID,
	})

	return redirectHX(e, "/match/"+matchID)
}

// PostProposal creates a scheduling proposal message in the match thread.
func (h *ThreadHandler) PostProposal(e *core.RequestEvent) error {
	matchID := e.Request.PathValue("id")
	match, err := findMatchOr404(h.app, e, matchID)
	if err != nil {
		return err
	}

	if match.GetString("status") != league.StatusPending {
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

	pd, err := h.parseProposalForm(e)
	if err != nil {
		return err
	}
	pdJSON, _ := json.Marshal(pd)

	col, err := h.app.FindCollectionByNameOrId("match_messages")
	if err != nil {
		return alertError(e, "Error interno")
	}

	record := core.NewRecord(col)
	record.Set("match", matchID)
	record.Set("author", e.Auth.Id)
	record.Set("type", "scheduling_proposal")
	record.Set("proposal_data", string(pdJSON))
	record.Set("proposal_status", "pending")

	if err := h.app.Save(record); err != nil {
		return alertError(e, "Error al crear propuesta")
	}

	h.notifyProposal(match, myTeam, proposalNotice{AuthorID: e.Auth.Id, Date: pd.Date, Time: pd.Time, VenueName: pd.VenueName})
	return redirectHX(e, "/match/"+matchID)
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
	h.notifier.NotifyPlayers(rivalPlayers, league.Notification{
		Type: "scheduling", Title: "Propuesta de fecha",
		Body: fmt.Sprintf("%s propone jugar el %s a las %s en %s", authorName, n.Date, n.Time, n.VenueName), MatchID: match.Id,
	})
}

func (h *ThreadHandler) parseProposalForm(e *core.RequestEvent) (ProposalData, error) {
	date := e.Request.FormValue("date")
	timeVal := e.Request.FormValue("time")
	venueID := e.Request.FormValue("venue_id")
	venueText := e.Request.FormValue("venue_text")

	if date == "" || timeVal == "" {
		return ProposalData{}, alertError(e, "Fecha y hora son obligatorias")
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

	if match.GetString("status") != league.StatusPending {
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
		return redirectHX(e, "/match/"+matchID)
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
	return redirectHX(e, "/match/"+matchID)
}

func (h *ThreadHandler) dispatchProposalAction(e *core.RequestEvent, match, msg *core.Record, authorTeam int) error {
	proposerPairID := match.GetString("pair1")
	if authorTeam == 2 {
		proposerPairID = match.GetString("pair2")
	}
	action := e.Request.FormValue("action")
	switch action {
	case "accept":
		return h.acceptProposal(e, match, msg, proposerPairID)
	case "reject":
		return h.rejectProposal(e, msg, match.Id, proposerPairID)
	default:
		return alertError(e, "Acción no válida")
	}
}

func (h *ThreadHandler) acceptProposal(e *core.RequestEvent, match, msg *core.Record, proposerPairID string) error {
	existing, _ := h.app.FindRecordsByFilter("match_messages",
		"match = {:mid} && proposal_status = 'accepted'",
		"", 0, 1,
		map[string]any{"mid": match.Id})
	if len(existing) > 0 {
		return alertError(e, "Ya hay una propuesta aceptada para este partido")
	}

	pd := ParseProposalData(msg.Get("proposal_data"))
	if pd == nil {
		return alertError(e, "Error al leer los datos de la propuesta")
	}

	match.Set("date", pd.Date)
	match.Set("time", pd.Time)
	match.Set("club", pd.VenueName)
	if err := h.app.Save(match); err != nil {
		return alertError(e, "Error al actualizar el partido")
	}

	msg.Set("proposal_status", "accepted")
	if err := h.app.Save(msg); err != nil {
		return alertError(e, "Error al marcar la propuesta como aceptada")
	}

	if err := h.supersedePending(match.Id, msg.Id); err != nil {
		slog.Error("supersede pending proposals", "match", match.Id, "err", err)
		_ = h.notifier.NotifyAdmins("admin_message",
			"Propuestas pendientes no actualizadas",
			fmt.Sprintf("El partido %s tiene propuestas que no se pudieron marcar como superadas. Revisa el hilo.", match.Id),
			match.Id)
	}

	proposerPlayers := league.PlayersForPair(h.app, proposerPairID)
	responderName := league.PlayerName(h.app, e.Auth.Id)
	h.notifier.NotifyPlayers(proposerPlayers, league.Notification{
		Type: "scheduling", Title: "Propuesta aceptada",
		Body: fmt.Sprintf("%s aceptó tu propuesta para el %s a las %s", responderName, pd.Date, pd.Time), MatchID: match.Id,
	})
	return nil
}

func (h *ThreadHandler) rejectProposal(e *core.RequestEvent, msg *core.Record, matchID, proposerPairID string) error {
	reason := e.Request.FormValue("rejection_reason")
	text := e.Request.FormValue("rejection_text")

	msg.Set("proposal_status", "rejected")
	msg.Set("rejection_reason", reason)
	msg.Set("rejection_text", text)
	if err := h.app.Save(msg); err != nil {
		return alertError(e, "Error al rechazar la propuesta")
	}

	proposerPlayers := league.PlayersForPair(h.app, proposerPairID)
	responderName := league.PlayerName(h.app, e.Auth.Id)
	h.notifier.NotifyPlayers(proposerPlayers, league.Notification{
		Type: "scheduling", Title: "Propuesta rechazada",
		Body: fmt.Sprintf("%s rechazó tu propuesta: %s", responderName, reason), MatchID: matchID,
	})
	return nil
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
	otherPending, _ := h.app.FindRecordsByFilter("match_messages",
		"match = {:mid} && type = 'scheduling_proposal' && proposal_status = 'pending' && id != {:msgid}",
		"", 0, 0,
		map[string]any{"mid": matchID, "msgid": excludeMsgID})
	var failedIDs []string
	for _, other := range otherPending {
		other.Set("proposal_status", "superseded")
		if err := h.app.Save(other); err != nil {
			slog.Error("supersede proposal", "id", other.Id, "err", err)
			failedIDs = append(failedIDs, other.Id)
		}
	}
	if len(failedIDs) > 0 {
		return fmt.Errorf("failed to supersede proposals: %v", failedIDs)
	}
	return nil
}

func (h *ThreadHandler) revokeAcceptance(e *core.RequestEvent, match, msg *core.Record, proposerPairID string) error {
	msg.Set("proposal_status", "rejected")
	msg.Set("rejection_reason", "Decisión cambiada")
	if err := h.app.Save(msg); err != nil {
		return alertError(e, "Error al cambiar la decisión")
	}
	match.Set("date", "")
	match.Set("time", "")
	match.Set("club", "")
	if err := h.app.Save(match); err != nil {
		slog.Error("save match after rejection", "match", match.Id, "err", err)
		return alertError(e, "Error al actualizar el partido")
	}
	proposerPlayers := league.PlayersForPair(h.app, proposerPairID)
	responderName := league.PlayerName(h.app, e.Auth.Id)
	h.notifier.NotifyPlayers(proposerPlayers, league.Notification{
		Type: "scheduling", Title: "Decisión cambiada",
		Body: fmt.Sprintf("%s cambió su decisión: propuesta ahora rechazada", responderName), MatchID: match.Id,
	})
	return nil
}

func (h *ThreadHandler) changeToAccepted(e *core.RequestEvent, match, msg *core.Record, proposerPairID string) error {
	existing, _ := h.app.FindRecordsByFilter("match_messages",
		"match = {:mid} && proposal_status = 'accepted'",
		"", 0, 1,
		map[string]any{"mid": match.Id})
	if len(existing) > 0 {
		return alertError(e, "Ya hay otra propuesta aceptada")
	}
	pd := ParseProposalData(msg.Get("proposal_data"))
	if pd == nil {
		return alertError(e, "Error al leer los datos de la propuesta")
	}
	msg.Set("proposal_status", "accepted")
	msg.Set("rejection_reason", "")
	msg.Set("rejection_text", "")
	if err := h.app.Save(msg); err != nil {
		return alertError(e, "Error al cambiar la decisión")
	}
	match.Set("date", pd.Date)
	match.Set("time", pd.Time)
	match.Set("club", pd.VenueName)
	if err := h.app.Save(match); err != nil {
		slog.Error("save match after acceptance", "match", match.Id, "err", err)
		return alertError(e, "Error al actualizar el partido")
	}
	if err := h.supersedePending(match.Id, msg.Id); err != nil {
		slog.Error("supersede pending proposals", "match", match.Id, "err", err)
		_ = h.notifier.NotifyAdmins("admin_message",
			"Propuestas pendientes no actualizadas",
			fmt.Sprintf("El partido %s tiene propuestas que no se pudieron marcar como superadas. Revisa el hilo.", match.Id),
			match.Id)
	}
	proposerPlayers := league.PlayersForPair(h.app, proposerPairID)
	responderName := league.PlayerName(h.app, e.Auth.Id)
	h.notifier.NotifyPlayers(proposerPlayers, league.Notification{
		Type: "scheduling", Title: "Decisión cambiada",
		Body: fmt.Sprintf("%s cambió su decisión: propuesta ahora aceptada para el %s a las %s", responderName, pd.Date, pd.Time), MatchID: match.Id,
	})
	return nil
}

// ProposalChangeDecision lets a player revoke or change their proposal response.
func (h *ThreadHandler) ProposalChangeDecision(e *core.RequestEvent) error {
	matchID := e.Request.PathValue("id")
	msgID := e.Request.PathValue("msgId")

	match, err := findMatchOr404(h.app, e, matchID)
	if err != nil {
		return err
	}

	if match.GetString("status") != league.StatusPending {
		return alertError(e, "Este partido ya no acepta cambios")
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

	authorTeam, _ := league.PlayerTeam(h.app, msg.GetString("author"), match)
	if authorTeam == myTeam {
		return alertError(e, "No puedes cambiar la decisión de tu propia propuesta")
	}

	currentStatus := msg.GetString("proposal_status")
	if currentStatus != "accepted" && currentStatus != "rejected" {
		return alertError(e, "Solo se pueden cambiar decisiones de propuestas aceptadas o rechazadas")
	}

	proposerPairID := match.GetString("pair1")
	if authorTeam == 2 {
		proposerPairID = match.GetString("pair2")
	}

	if currentStatus == "accepted" {
		if err := h.revokeAcceptance(e, match, msg, proposerPairID); err != nil {
			return err
		}
	} else {
		if err := h.changeToAccepted(e, match, msg, proposerPairID); err != nil {
			return err
		}
	}

	return redirectHX(e, "/match/"+matchID)
}

// PostAvailability posts a quick availability message to the match thread.
func (h *ThreadHandler) PostAvailability(e *core.RequestEvent) error {
	matchID := e.Request.PathValue("id")
	match, err := findMatchOr404(h.app, e, matchID)
	if err != nil {
		return err
	}

	myTeam, err := league.PlayerTeam(h.app, e.Auth.Id, match)
	if err != nil || myTeam == 0 {
		return alertError(e, "No eres participante de este partido")
	}

	available := e.Request.FormValue("available")
	content := "No puedo"
	if available == "1" {
		content = "Estoy libre"
	}

	col, err := h.app.FindCollectionByNameOrId("match_messages")
	if err != nil {
		return alertError(e, "Error interno")
	}

	record := core.NewRecord(col)
	record.Set("match", matchID)
	record.Set("author", e.Auth.Id)
	record.Set("type", "availability")
	record.Set("content", content)

	if err := h.app.Save(record); err != nil {
		return alertError(e, "Error al enviar disponibilidad")
	}

	rivalPairID := match.GetString("pair1")
	if myTeam == 1 {
		rivalPairID = match.GetString("pair2")
	}
	rivalPlayers := league.PlayersForPair(h.app, rivalPairID)
	authorName := league.PlayerName(h.app, e.Auth.Id)
	h.notifier.NotifyPlayers(rivalPlayers, league.Notification{
		Type: "general", Title: "Disponibilidad",
		Body: fmt.Sprintf("%s: %s", authorName, content), MatchID: matchID,
	})

	return redirectHX(e, "/match/"+matchID)
}
