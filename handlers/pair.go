package handlers

import (
	"net/http"
	"sort"

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

// PairPageData bundles all data for the pair page.
type PairPageData struct {
	Pair         *core.Record
	PairName     string
	Player1      pairPlayerLink
	Player2      pairPlayerLink
	Competitions []CompetitionStat
	Recent       []RecentMatch
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
	}

	comps, _ := h.app.FindRecordsByFilter("competitions",
		"pairs ~ {:pid}", "", 0, 0,
		map[string]any{"pid": id})

	for _, c := range comps {
		cs := CompetitionStat{
			CompID:   c.Id,
			CompName: c.GetString("name"),
		}
		if rows, err := h.leagueSvc.ComputeStandings(c.Id); err == nil {
			for _, r := range rows {
				if r.PairID == id {
					cs.Position = r.Position
					cs.Wins = r.Wins
					cs.Losses = r.Losses
					cs.Played = r.Played
					break
				}
			}
		}
		data.Competitions = append(data.Competitions, cs)
	}
	sort.Slice(data.Competitions, func(i, j int) bool {
		return data.Competitions[i].CompName < data.Competitions[j].CompName
	})

	results := pairMatchResults(h.app, id)
	sort.Slice(results, func(i, j int) bool {
		return results[i].date > results[j].date
	})
	data.Recent = buildRecentMatches(results, 20)

	return h.renderPage(e, "pair.html", map[string]any{
		"PageTitle": data.PairName,
		"Data":      data,
		"Mode":      PlayerSummary,
	})
}
