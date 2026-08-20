package handlers

import (
	"github.com/pocketbase/pocketbase/core"
)

type PublicHandler struct {
	app        core.App
	renderPage func(e *core.RequestEvent, page string, data map[string]any) error
}

func NewPublicHandler(app core.App, renderPage func(e *core.RequestEvent, page string, data map[string]any) error) *PublicHandler {
	return &PublicHandler{app: app, renderPage: renderPage}
}

func (h *PublicHandler) Home(e *core.RequestEvent) error {
	competitions, _ := h.app.FindRecordsByFilter("competitions",
		"active = true", "", 0, 0, nil)

	return h.renderPage(e, "home.html", map[string]any{
		"Competitions": competitions,
	})
}

type MatchdayView struct {
	Matchday *core.Record
	Matches  []MatchdayMatchView
}

type MatchdayMatchView struct {
	Match *core.Record
	Pair1 string
	Pair2 string
}

func (h *PublicHandler) Competition(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	comp, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return e.Redirect(302, "/")
	}

	matchdays, _ := h.app.FindRecordsByFilter("matchdays",
		"competition = {:cid}",
		"round_number", 0, 0,
		map[string]any{"cid": id})

	allPairIDs := make(map[string]bool)
	type mdMatches struct {
		matchday *core.Record
		matches  []*core.Record
	}
	mdData := make([]mdMatches, 0, len(matchdays))

	for _, md := range matchdays {
		matches, _ := h.app.FindRecordsByFilter("matches",
			"matchday = {:mid}",
			"", 0, 0,
			map[string]any{"mid": md.Id})
		for _, m := range matches {
			allPairIDs[m.GetString("pair1")] = true
			allPairIDs[m.GetString("pair2")] = true
		}
		mdData = append(mdData, mdMatches{matchday: md, matches: matches})
	}

	pairIDSlice := make([]string, 0, len(allPairIDs))
	for pid := range allPairIDs {
		pairIDSlice = append(pairIDSlice, pid)
	}
	pairNames, _ := expandPairNames(h.app, pairIDSlice)

	var rounds []MatchdayView
	for _, mdd := range mdData {
		var matchViews []MatchdayMatchView
		for _, m := range mdd.matches {
			matchViews = append(matchViews, MatchdayMatchView{
				Match: m,
				Pair1: pairNames[m.GetString("pair1")],
				Pair2: pairNames[m.GetString("pair2")],
			})
		}
		rounds = append(rounds, MatchdayView{Matchday: mdd.matchday, Matches: matchViews})
	}

	var standings []StandingRowFull
	if comp.GetString("type") == "league" {
		standings, _ = ComputeStandings(h.app, id)
	}

	var awards []Award
	if !comp.GetBool("active") {
		awards = computeAwards(h.app, id)
	}

	return h.renderPage(e, "competition.html", map[string]any{
		"Competition": comp,
		"Rounds":      rounds,
		"Standings":   standings,
		"Awards":      awards,
		"IsArchived":  !comp.GetBool("active"),
	})
}
