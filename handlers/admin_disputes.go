package handlers

import (
	"log/slog"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
	"padelleague/notify"
)

// DisputeHandler handles admin dispute resolution and walkover approval.
type DisputeHandler struct {
	app        core.App
	notifier   *notify.Notifier
	renderPage RenderFunc
}

// NewDisputeHandler creates a DisputeHandler with the given dependencies.
func NewDisputeHandler(app core.App, notifier *notify.Notifier, renderPage RenderFunc) *DisputeHandler {
	return &DisputeHandler{app: app, notifier: notifier, renderPage: renderPage}
}

// Disputes renders the admin disputes page listing all disputed matches.
func (h *DisputeHandler) Disputes(e *core.RequestEvent) error {
	report := league.HealthReport(h.app, time.Now())
	var items []league.HealthItem
	for _, cat := range report {
		if cat.Key == "disputes" || cat.Key == "walkovers" {
			items = append(items, cat.Items...)
		}
	}

	return h.renderPage(e, "admin/disputes.html", map[string]any{
		"PageTitle": "Disputas",
		"Items":     items,
	})
}

// WalkoverApprove handles POST to approve a walkover request, finalizing the match.
func (h *DisputeHandler) WalkoverApprove(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := findMatchOr404(h.app, e, id)
	if err != nil {
		return err
	}

	if match.GetString("review_type") != "walkover" {
		return alertError(e, "Este partido no es una solicitud de incomparecencia")
	}
	if match.GetString("status") == league.StatusFinal {
		return alertError(e, "Este partido ya está resuelto")
	}

	winnerID := e.Request.FormValue("winner")
	if winnerID != match.GetString("pair1") && winnerID != match.GetString("pair2") {
		return alertError(e, "El ganador debe ser una de las dos parejas")
	}

	compID := match.GetString("competition")
	comp, err := h.app.FindRecordById("competitions", compID)
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}

	woScore := comp.GetString("walkover_score")
	if woScore == "" {
		woScore = "6-0 6-0"
	}
	if _, err := league.ParseScore(woScore); err != nil {
		return alertError(e, "El marcador de incomparecencia configurado no es válido:"+woScore)
	}

	loserID := match.GetString("pair2")
	if winnerID == loserID {
		loserID = match.GetString("pair1")
	}

	match.Set("scores", woScore)
	match.Set("winner", winnerID)
	match.Set("status", league.StatusFinal)

	if err := h.app.Save(match); err != nil {
		return alertError(e, "Error al aprobar la incomparecencia")
	}

	addTimelineEntry(h.app, timelineEntry{
		MatchID: match.Id, ActorID: e.Auth.Id, Kind: "result_event",
		Detail: "aprobó incomparecencia a favor de " + league.PairNames(h.app, []string{winnerID})[winnerID],
	})
	if err := h.applyWalkoverPenalty(e, comp, loserID); err != nil {
		return err
	}

	n := league.NotifWalkoverApproved(match.Id, comp.GetString("name"))
	h.notifyMatchPlayers(match, n.Type, n.Title, n.Body)

	flash(e, "Incomparecencia aprobada")
	return redirectHX(e, "/admin/competitions/"+compID)
}

func (h *DisputeHandler) applyWalkoverPenalty(e *core.RequestEvent, comp *core.Record, loserID string) error {
	penalty := comp.GetFloat("default_penalty")
	if penalty <= 0 {
		return nil
	}
	if err := league.ApplyPenalty(h.app, league.PenaltyInput{CompetitionID: comp.Id, PairID: loserID, Reason: "Incomparecencia aprobada", AdminID: e.Auth.Id, Amount: penalty}); err != nil {
		slog.Error("apply walkover penalty", "comp", comp.Id, "pair", loserID, "err", err)
		return alertError(e, "Incomparecencia aprobada, pero no se pudo aplicar la penalización. Aplícala manualmente.")
	}
	return nil
}

// DisputesResolve handles POST to resolve a disputed match with the admin's chosen score.
func (h *DisputeHandler) DisputesResolve(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := findMatchOr404(h.app, e, id)
	if err != nil {
		return err
	}

	if match.GetString("status") != league.StatusDisputed {
		return alertError(e, "Este partido no está en disputa")
	}

	score := e.Request.FormValue("score")
	if score == "" {
		return alertError(e, "Debes ingresar un marcador")
	}

	winnerID, err := league.DetermineWinner(match, score)
	if err != nil {
		slog.Error("determine winner in dispute resolution", "match", match.Id, "err", err)
		return alertError(e, "Marcador no válido")
	}

	match.Set("scores", score)
	match.Set("winner", winnerID)
	match.Set("status", league.StatusFinal)

	if err := h.app.Save(match); err != nil {
		return alertError(e, "Error al resolver la disputa")
	}

	addTimelineEntry(h.app, timelineEntry{
		MatchID: match.Id, ActorID: e.Auth.Id, Kind: "result_event",
		Detail: "resolvió la disputa: " + score,
	})
	compID := match.GetString("competition")
	n := league.NotifDisputeResolved(match.Id, league.CompetitionName(h.app, compID))
	h.notifyMatchPlayers(match, n.Type, n.Title, n.Body)

	flash(e, "Disputa resuelta")
	return redirectHX(e, "/admin/competitions/"+compID)
}

func (h *DisputeHandler) notifyMatchPlayers(match *core.Record, notifType, title, body string) {
	allPlayers := append(league.PlayersForPair(h.app, match.GetString("pair1")),
		league.PlayersForPair(h.app, match.GetString("pair2"))...)
	h.notifier.NotifyPlayers(allPlayers, league.Notification{
		Type: notifType, Title: title, Body: body, MatchID: match.Id,
	})
	h.notifier.EmailPlayers(allPlayers, title, body, "/match/"+match.Id)
}
