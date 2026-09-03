package handlers

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
)

// PairPageHandler serves the canonical pair page.
type PairPageHandler struct {
	app             core.App
	leagueSvc       *league.Service
	renderPage      RenderFunc
	renderErrorPage RenderErrorFunc
}

// NewPairPageHandler creates a PairPageHandler with the given dependencies.
func NewPairPageHandler(app core.App, leagueSvc *league.Service, renderPage RenderFunc, renderErrorPage RenderErrorFunc) *PairPageHandler {
	return &PairPageHandler{app: app, leagueSvc: leagueSvc, renderPage: renderPage, renderErrorPage: renderErrorPage}
}

type pairPlayerLink struct {
	ID   string
	Name string
}

// PairPageData bundles a pair's identity and shared stats for the pair page.
type PairPageData struct {
	Pair     *core.Record
	PairName string
	Player1  pairPlayerLink
	Player2  pairPlayerLink
	Stats    league.StatsSummary
}

// PairPage renders the canonical pair page with players, competitions, and matches.
func (h *PairPageHandler) PairPage(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	pair, err := h.app.FindRecordById("pairs", id)
	if err != nil {
		return h.renderErrorPage(e, http.StatusNotFound, "Pareja no encontrada")
	}

	p1ID := pair.GetString("player1")
	p2ID := pair.GetString("player2")

	data := PairPageData{
		Pair:     pair,
		PairName: pair.GetString("name"),
		Player1:  pairPlayerLink{ID: p1ID, Name: league.PlayerName(h.app, p1ID)},
		Player2:  pairPlayerLink{ID: p2ID, Name: league.PlayerName(h.app, p2ID)},
		Stats:    h.leagueSvc.Summarize([]string{id}),
	}

	return h.renderPage(e, "pair.html", map[string]any{
		"PageTitle": data.PairName,
		"Data":      data,
		"Mode":      PlayerSummary,
	})
}
