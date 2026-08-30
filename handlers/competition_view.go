package handlers

import (
	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
)

// CompetitionView is the shared model for competition card/summary rendering.
type CompetitionView struct {
	Mode          Mode
	Competition   *core.Record
	Name          string
	PairsCount    int
	TotalMatches  int
	PlayedMatches int
	DisputeCount  int
	PendingCount  int
	URL           string

	PendingDetails []MatchCard
}

// NewCompetitionView builds a CompetitionView from a competition record and its matches.
func NewCompetitionView(app core.App, comp *core.Record, mode Mode) CompetitionView {
	allMatches, _ := app.FindRecordsByFilter("matches",
		"competition = {:cid}", "", 0, 0,
		map[string]any{"cid": comp.Id})

	played, disputes, pending := 0, 0, 0
	for _, m := range allMatches {
		switch m.GetString("status") {
		case league.StatusFinal:
			played++
		case league.StatusDisputed:
			disputes++
		case league.StatusPending:
			pending++
		}
	}

	url := "/competition/" + comp.Id
	if mode.Admin {
		url = "/admin/competitions/" + comp.Id
	}

	return CompetitionView{
		Mode:          mode,
		Competition:   comp,
		Name:          comp.GetString("name"),
		PairsCount:    len(comp.GetStringSlice("pairs")),
		TotalMatches:  len(allMatches),
		PlayedMatches: played,
		DisputeCount:  disputes,
		PendingCount:  pending,
		URL:           url,
	}
}

// NewHomeCompetitionView builds a PlayerRow CompetitionView with pending details pre-populated.
func NewHomeCompetitionView(comp *core.Record, pendingCount int, pendingDetails []MatchCard) CompetitionView {
	return CompetitionView{
		Mode:           PlayerRow,
		Competition:    comp,
		Name:           comp.GetString("name"),
		PendingCount:   pendingCount,
		PendingDetails: pendingDetails,
		URL:            "/competition/" + comp.Id,
	}
}
