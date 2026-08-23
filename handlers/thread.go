package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
	"padelleague/notify"
)

type ThreadHandler struct {
	app           core.App
	notifier      *notify.Notifier
	renderPage    func(e *core.RequestEvent, page string, data map[string]any) error
	renderPartial func(e *core.RequestEvent, page string, data map[string]any) error
}

func NewThreadHandler(app core.App, notifier *notify.Notifier, renderPage func(e *core.RequestEvent, page string, data map[string]any) error, renderPartial func(e *core.RequestEvent, page string, data map[string]any) error) *ThreadHandler {
	return &ThreadHandler{app: app, notifier: notifier, renderPage: renderPage, renderPartial: renderPartial}
}

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

type ProposalData struct {
	Date      string `json:"date"`
	Time      string `json:"time"`
	VenueID   string `json:"venue_id"`
	VenueName string `json:"venue_name"`
	VenueText string `json:"venue_text"`
}

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
	case map[string]any:
		b, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		if err := json.Unmarshal(b, &pd); err != nil {
			return nil
		}
	default:
		return nil
	}
	return &pd
}

func (h *ThreadHandler) buildThreadMessages(match *core.Record, matchID string, myTeam int) []ThreadMessage {
	messages, _ := h.app.FindRecordsByFilter("match_messages",
		"match = {:mid}", "", 0, 0,
		map[string]any{"mid": matchID})

	sort.Slice(messages, func(i, j int) bool {
		return messages[i].GetDateTime("created").Time().Before(
			messages[j].GetDateTime("created").Time())
	})

	pair1Players := league.PlayersForPair(h.app, match.GetString("pair1"))
	pair2Players := league.PlayersForPair(h.app, match.GetString("pair2"))
	nameCache := make(map[string]string)

	authorTeamOf := func(uid string) int {
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

	cachedName := func(uid string) string {
		if n, ok := nameCache[uid]; ok {
			return n
		}
		n := league.PlayerName(h.app, uid)
		nameCache[uid] = n
		return n
	}

	var threadMessages []ThreadMessage
	for _, msg := range messages {
		authorID := msg.GetString("author")
		authorTeam := authorTeamOf(authorID)

		msgType := msg.GetString("type")
		var pd *ProposalData
		if msgType == "scheduling_proposal" {
			pd = ParseProposalData(msg.GetString("proposal_data"))
		}

		status := msg.GetString("proposal_status")
		isParticipant := myTeam != 0
		canRespond := msgType == "scheduling_proposal" &&
			status == "pending" &&
			match.GetString("status") == StatusPending &&
			isParticipant &&
			authorTeam != myTeam

		canChangeDecision := msgType == "scheduling_proposal" &&
			(status == "accepted" || status == "rejected") &&
			match.GetString("status") == StatusPending &&
			isParticipant &&
			authorTeam != myTeam

		threadMessages = append(threadMessages, ThreadMessage{
			Record:            msg,
			AuthorName:        cachedName(authorID),
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

func (h *ThreadHandler) Thread(e *core.RequestEvent) error {
	matchID := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", matchID)
	if err != nil {
		return alertError(e, "Partido no encontrado")
	}

	pair1ID := match.GetString("pair1")
	pair2ID := match.GetString("pair2")
	if pair1ID == "" || pair2ID == "" {
		return e.HTML(http.StatusOK, `<div class="text-center py-6 opacity-60">Parejas pendientes de asignacion</div>`)
	}

	isAdmin := e.Auth.GetString("role") == "admin"
	myTeam, teamErr := league.PlayerTeam(h.app, e.Auth.Id, match)
	if teamErr != nil && !isAdmin {
		return alertError(e, "No tienes acceso a este hilo")
	}

	threadMessages := h.buildThreadMessages(match, matchID, myTeam)

	venues, _ := h.app.FindRecordsByFilter("venues",
		"id != ''", "name", 0, 0, nil)

	isParticipant := myTeam != 0
	canPost := isParticipant
	canPropose := canPost && match.GetString("status") == StatusPending

	return h.renderPartial(e, "thread.html", map[string]any{
		"MatchID":       matchID,
		"Messages":      threadMessages,
		"Venues":        venues,
		"CanPost":       canPost,
		"CanPropose":    canPropose,
		"IsAdmin":       isAdmin,
		"IsParticipant": isParticipant,
	})
}

func (h *ThreadHandler) ThreadMessages(e *core.RequestEvent) error {
	matchID := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", matchID)
	if err != nil {
		return alertError(e, "Partido no encontrado")
	}

	isAdmin := e.Auth.GetString("role") == "admin"
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

func (h *ThreadHandler) PostMessage(e *core.RequestEvent) error {
	matchID := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", matchID)
	if err != nil {
		return alertError(e, "Partido no encontrado")
	}

	myTeam, err := league.PlayerTeam(h.app, e.Auth.Id, match)
	if err != nil || myTeam == 0 {
		return alertError(e, "No eres participante de este partido")
	}

	content := e.Request.FormValue("content")
	if content == "" {
		return alertError(e, "El mensaje no puede estar vacio")
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
	h.notifier.NotifyPlayers(rivalPlayers, "general",
		"Nuevo mensaje",
		fmt.Sprintf("%s escribio: %s", authorName, league.Truncate(content, 60)),
		matchID)

	return redirectHX(e, "/match/"+matchID)
}

func (h *ThreadHandler) PostProposal(e *core.RequestEvent) error {
	matchID := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", matchID)
	if err != nil {
		return alertError(e, "Partido no encontrado")
	}

	if match.GetString("status") != StatusPending {
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

	date := e.Request.FormValue("date")
	time := e.Request.FormValue("time")
	venueID := e.Request.FormValue("venue_id")
	venueText := e.Request.FormValue("venue_text")

	if date == "" || time == "" {
		return alertError(e, "Fecha y hora son obligatorias")
	}

	venueName := venueText
	if venueID != "" && venueID != "otro" {
		venue, err := h.app.FindRecordById("venues", venueID)
		if err != nil {
			venueID = ""
			venueName = venueText
		} else {
			venueName = venue.GetString("name")
		}
	} else {
		venueID = ""
	}

	pd := ProposalData{
		Date:      date,
		Time:      time,
		VenueID:   venueID,
		VenueName: venueName,
		VenueText: venueText,
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

	rivalPairID := match.GetString("pair1")
	if myTeam == 1 {
		rivalPairID = match.GetString("pair2")
	}
	rivalPlayers := league.PlayersForPair(h.app, rivalPairID)
	authorName := league.PlayerName(h.app, e.Auth.Id)
	h.notifier.NotifyPlayers(rivalPlayers, "scheduling",
		"Propuesta de fecha",
		fmt.Sprintf("%s propone jugar el %s a las %s en %s", authorName, date, time, venueName),
		matchID)

	return redirectHX(e, "/match/"+matchID)
}

func (h *ThreadHandler) RespondProposal(e *core.RequestEvent) error {
	matchID := e.Request.PathValue("id")
	msgID := e.Request.PathValue("msgId")

	match, err := h.app.FindRecordById("matches", matchID)
	if err != nil {
		return alertError(e, "Partido no encontrado")
	}

	if match.GetString("status") != StatusPending {
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

	proposerPairID := match.GetString("pair1")
	if authorTeam == 2 {
		proposerPairID = match.GetString("pair2")
	}

	action := e.Request.FormValue("action")

	switch action {
	case "accept":
		existing, _ := h.app.FindRecordsByFilter("match_messages",
			"match = {:mid} && proposal_status = 'accepted'",
			"", 0, 1,
			map[string]any{"mid": matchID})
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

		otherPending, _ := h.app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'scheduling_proposal' && proposal_status = 'pending' && id != {:msgid}",
			"", 0, 0,
			map[string]any{"mid": matchID, "msgid": msgID})
		for _, other := range otherPending {
			other.Set("proposal_status", "superseded")
			if err := h.app.Save(other); err != nil {
				slog.Error("supersede proposal", "id", other.Id, "err", err)
			}
		}

		proposerPlayers := league.PlayersForPair(h.app, proposerPairID)
		responderName := league.PlayerName(h.app, e.Auth.Id)
		h.notifier.NotifyPlayers(proposerPlayers, "scheduling",
			"Propuesta aceptada",
			fmt.Sprintf("%s acepto tu propuesta para el %s a las %s", responderName, pd.Date, pd.Time),
			matchID)
	case "reject":
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
		h.notifier.NotifyPlayers(proposerPlayers, "scheduling",
			"Propuesta rechazada",
			fmt.Sprintf("%s rechazo tu propuesta: %s", responderName, reason),
			matchID)
	default:
		return alertError(e, "Accion no valida")
	}

	return redirectHX(e, "/match/"+matchID)
}

func (h *ThreadHandler) ProposalChangeDecision(e *core.RequestEvent) error {
	matchID := e.Request.PathValue("id")
	msgID := e.Request.PathValue("msgId")

	match, err := h.app.FindRecordById("matches", matchID)
	if err != nil {
		return alertError(e, "Partido no encontrado")
	}

	if match.GetString("status") != StatusPending {
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
		return alertError(e, "No puedes cambiar la decision de tu propia propuesta")
	}

	currentStatus := msg.GetString("proposal_status")
	if currentStatus != "accepted" && currentStatus != "rejected" {
		return alertError(e, "Solo se pueden cambiar decisiones de propuestas aceptadas o rechazadas")
	}

	proposerPairID := match.GetString("pair1")
	if authorTeam == 2 {
		proposerPairID = match.GetString("pair2")
	}

	responderName := league.PlayerName(h.app, e.Auth.Id)

	if currentStatus == "accepted" {
		msg.Set("proposal_status", "rejected")
		msg.Set("rejection_reason", "Decision cambiada")
		if err := h.app.Save(msg); err != nil {
			return alertError(e, "Error al cambiar la decision")
		}

		match.Set("date", "")
		match.Set("time", "")
		match.Set("club", "")
		if err := h.app.Save(match); err != nil {
			slog.Error("save match after rejection", "match", matchID, "err", err)
			return alertError(e, "Error al actualizar el partido")
		}

		proposerPlayers := league.PlayersForPair(h.app, proposerPairID)
		h.notifier.NotifyPlayers(proposerPlayers, "scheduling",
			"Decision cambiada",
			fmt.Sprintf("%s cambio su decision: propuesta ahora rechazada", responderName),
			matchID)
	} else {
		existing, _ := h.app.FindRecordsByFilter("match_messages",
			"match = {:mid} && proposal_status = 'accepted'",
			"", 0, 1,
			map[string]any{"mid": matchID})
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
			return alertError(e, "Error al cambiar la decision")
		}

		match.Set("date", pd.Date)
		match.Set("time", pd.Time)
		match.Set("club", pd.VenueName)
		if err := h.app.Save(match); err != nil {
			slog.Error("save match after acceptance", "match", matchID, "err", err)
			return alertError(e, "Error al actualizar el partido")
		}

		otherPending, _ := h.app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'scheduling_proposal' && proposal_status = 'pending' && id != {:msgid}",
			"", 0, 0,
			map[string]any{"mid": matchID, "msgid": msgID})
		for _, other := range otherPending {
			other.Set("proposal_status", "superseded")
			if err := h.app.Save(other); err != nil {
				slog.Error("supersede proposal", "id", other.Id, "err", err)
			}
		}

		proposerPlayers := league.PlayersForPair(h.app, proposerPairID)
		h.notifier.NotifyPlayers(proposerPlayers, "scheduling",
			"Decision cambiada",
			fmt.Sprintf("%s cambio su decision: propuesta ahora aceptada para el %s a las %s", responderName, pd.Date, pd.Time),
			matchID)
	}

	return redirectHX(e, "/match/"+matchID)
}
