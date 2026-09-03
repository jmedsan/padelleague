package handlers

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
)

// PlayerHandler serves player profile and head-to-head comparison pages.
type PlayerHandler struct {
	app             core.App
	leagueSvc       *league.Service
	renderPage      RenderFunc
	renderErrorPage RenderErrorFunc
}

// NewPlayerHandler creates a PlayerHandler with the given dependencies.
func NewPlayerHandler(app core.App, leagueSvc *league.Service, renderPage RenderFunc, renderErrorPage RenderErrorFunc) *PlayerHandler {
	return &PlayerHandler{app: app, leagueSvc: leagueSvc, renderPage: renderPage, renderErrorPage: renderErrorPage}
}

// PairInfo holds a pair record with the partner's display name.
type PairInfo struct {
	Pair      *core.Record
	Partner   string
	PartnerID string
}

// PlayerData bundles a player's identity, pairs, and shared stats for the profile page.
type PlayerData struct {
	User  *core.Record
	Pairs []PairInfo
	Stats league.StatsSummary
}

// Player renders the player profile page with stats and recent matches.
func (h *PlayerHandler) Player(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	user, err := h.app.FindRecordById("users", id)
	if err != nil {
		return h.renderErrorPage(e, http.StatusNotFound, "Jugador no encontrado")
	}

	pairs, _ := league.PairsForPlayer(h.app, user.Id)

	pairInfos := make([]PairInfo, 0, len(pairs))
	pairIDs := make([]string, 0, len(pairs))
	for _, p := range pairs {
		partnerID := p.GetString("player1")
		if partnerID == user.Id {
			partnerID = p.GetString("player2")
		}
		pairInfos = append(pairInfos, PairInfo{
			Pair:      p,
			Partner:   league.PlayerName(h.app, partnerID),
			PartnerID: partnerID,
		})
		pairIDs = append(pairIDs, p.Id)
	}

	data := PlayerData{
		User:  user,
		Pairs: pairInfos,
		Stats: h.leagueSvc.Summarize(pairIDs),
	}

	return h.renderPage(e, "player.html", map[string]any{
		"PageTitle": user.GetString("display_name"),
		"Data":      data,
		"Mode":      PlayerFull,
	})
}
