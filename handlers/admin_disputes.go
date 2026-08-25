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
		})
	}

	return h.renderPage(e, "admin/disputes.html", map[string]any{
		"Disputes": views,
	})
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
