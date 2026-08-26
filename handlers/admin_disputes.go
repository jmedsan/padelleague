package handlers

import (
	"log/slog"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
	"padelleague/notify"
)

// DisputeView holds a disputed match with resolved pair and player names.
type DisputeView struct {
	Match        *core.Record
	Pair1Name    string
	Pair2Name    string
	SubmittedBy  string
	DisputedBy   string
	DisputeNotes string
	ReviewType   string
	RequestedBy  string
}

// Disputes renders the admin disputes page listing all disputed matches.
func (h *AdminHandler) Disputes(e *core.RequestEvent) error {
	matches, _ := h.app.FindRecordsByFilter("matches",
		"status = 'disputed'", "", 0, 0, nil)

	var views []DisputeView
	for _, m := range matches {
		pairIDs := []string{m.GetString("pair1"), m.GetString("pair2")}
		pairNames := league.PairNames(h.app, pairIDs)

		views = append(views, DisputeView{
			Match:        m,
			Pair1Name:    pairNames[pairIDs[0]],
			Pair2Name:    pairNames[pairIDs[1]],
			SubmittedBy:  league.PlayerName(h.app, m.GetString("submitted_by")),
			DisputedBy:   league.PlayerName(h.app, m.GetString("disputed_by")),
			DisputeNotes: m.GetString("dispute_notes"),
			ReviewType:   m.GetString("review_type"),
			RequestedBy:  league.PlayerName(h.app, m.GetString("walkover_requested_by")),
		})
	}

	return h.renderPage(e, "admin/disputes.html", map[string]any{
		"Disputes": views,
	})
}

// WalkoverApprove handles POST to approve a walkover request, finalizing the match.
func (h *AdminHandler) WalkoverApprove(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", id)
	if err != nil {
		return alertError(e, "Partido no encontrado")
	}

	if match.GetString("review_type") != "walkover" {
		return alertError(e, "Este partido no es una solicitud de walkover")
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
		return alertError(e, "El marcador de walkover configurado no es válido: "+woScore)
	}

	loserID := match.GetString("pair2")
	if winnerID == loserID {
		loserID = match.GetString("pair1")
	}

	match.Set("scores", woScore)
	match.Set("winner", winnerID)
	match.Set("status", league.StatusFinal)

	if err := h.app.Save(match); err != nil {
		return alertError(e, "Error al aprobar el walkover")
	}

	if penalty := comp.GetFloat("default_penalty"); penalty > 0 {
		if err := league.ApplyPenalty(h.app, comp, loserID, penalty, true); err != nil {
			slog.Error("apply walkover penalty", "comp", compID, "pair", loserID, "err", err)
			return alertError(e, "Walkover aprobado, pero no se pudo aplicar la penalización. Aplícala manualmente.")
		}
	}

	allPlayers := append(league.PlayersForPair(h.app, match.GetString("pair1")),
		league.PlayersForPair(h.app, match.GetString("pair2"))...)
	h.notifier.NotifyPlayers(allPlayers, "general", "Walkover aprobado", "Un administrador ha resuelto el partido como walkover.", match.Id)
	notify.EmailNotifyPlayers(h.app, allPlayers, "Walkover aprobado", "Un administrador ha resuelto el partido como walkover.", "/match/"+match.Id)

	return redirectHX(e, "/admin/competitions/"+compID)
}

// DisputesResolve handles POST to resolve a disputed match with the admin's chosen score.
func (h *AdminHandler) DisputesResolve(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", id)
	if err != nil {
		return alertError(e, "Partido no encontrado")
	}

	if match.GetString("status") != league.StatusDisputed {
		return alertError(e, "Este partido no está en disputa")
	}

	score := e.Request.FormValue("score")
	if score == "" {
		return alertError(e, "Debes ingresar un marcador")
	}

	manualWinner := e.Request.FormValue("winner")
	var winnerID string
	if manualWinner != "" {
		if manualWinner != match.GetString("pair1") && manualWinner != match.GetString("pair2") {
			return alertError(e, "El ganador debe ser una de las dos parejas")
		}
		if _, err := league.ParseScore(score); err != nil {
			return alertError(e, "Marcador inválido")
		}
		winnerID = manualWinner
	} else {
		var err error
		winnerID, err = league.DetermineWinner(match, score)
		if err != nil {
			slog.Error("determine winner in dispute resolution", "match", match.Id, "err", err)
			return alertError(e, "Marcador inválido. Selecciona el ganador manualmente.")
		}
	}

	match.Set("scores", score)
	match.Set("winner", winnerID)
	match.Set("status", league.StatusFinal)

	if err := h.app.Save(match); err != nil {
		return alertError(e, "Error al resolver la disputa")
	}

	allPlayers := append(league.PlayersForPair(h.app, match.GetString("pair1")),
		league.PlayersForPair(h.app, match.GetString("pair2"))...)
	h.notifier.NotifyPlayers(allPlayers, "dispute", "Disputa resuelta", "Un administrador ha resuelto la disputa de tu partido.", match.Id)
	notify.EmailNotifyPlayers(h.app, allPlayers, "Disputa resuelta", "Un administrador ha resuelto la disputa de tu partido.", "/match/"+match.Id)

	compID := match.GetString("competition")
	return redirectHX(e, "/admin/competitions/"+compID)
}
