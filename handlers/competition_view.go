package handlers

import (
	"log/slog"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
)

// CompetitionView is the shared model for competition card/summary rendering.
type CompetitionView struct {
	Mode            Mode
	Competition     *core.Record
	Name            string
	CompetitionLogo string
	PairsCount      int
	TotalMatches    int
	PlayedMatches   int
	// HasProvisional is true when PlayedMatches counts an unconfirmed result.
	HasProvisional bool
	AlertCount     int // matches with status=disputed: open disputes AND pending walkover approvals
	PendingCount   int
	URL            string

	// Standing is non-nil only for a PlayerRow home card in a league with
	// computed standings, so the card can show the player's own position
	// and points ("3º · 9 pts") alongside the pending-matches line.
	Standing *PlayerStanding

	// Setup is non-nil only for an inactive competition's AdminSummary card,
	// so the setup checklist and "Activar" action render inline on the card.
	Setup *league.CompSetup
}

// PlayerStanding is the player's own row in a league's standings, reduced
// to what the home competition card shows.
type PlayerStanding struct {
	Position int
	Points   int
	// HasProvisional is true when Points counts an unconfirmed result.
	HasProvisional bool
}

// NewCompetitionView builds a CompetitionView from a competition record and its matches.
func NewCompetitionView(app core.App, comp *core.Record, mode Mode) CompetitionView {
	allMatches, _ := app.FindRecordsByFilter("matches",
		"competition = {:cid}", "", 0, 0,
		map[string]any{"cid": comp.Id})

	unconfirmed, err := league.ProvisionalResults(app, allMatches)
	if err != nil {
		slog.Error("competition view: provisional results", "comp", comp.Id, "err", err)
	}

	// A match with an unconfirmed result counts as played; it also stays
	// pending until the rival answers.
	played, alerts, pending := len(unconfirmed), 0, 0
	for _, m := range allMatches {
		switch m.GetString("status") {
		case league.StatusFinal:
			played++
		case league.StatusDisputed:
			alerts++
		case league.StatusPending, league.StatusScheduled:
			pending++
		}
	}

	url := "/competition/" + comp.Id
	if mode.Admin {
		url = "/admin/competitions/" + comp.Id
	}

	return CompetitionView{
		Mode:            mode,
		Competition:     comp,
		Name:            comp.GetString("name"),
		CompetitionLogo: league.CompetitionLogoURL(comp.Id, comp.GetString("logo")),
		PairsCount:      len(comp.GetStringSlice("pairs")),
		TotalMatches:    len(allMatches),
		PlayedMatches:   played,
		HasProvisional:  len(unconfirmed) > 0,
		AlertCount:      alerts,
		PendingCount:    pending,
		URL:             url,
	}
}

// NewHomeCompetitionView builds a PlayerRow CompetitionView for a player's
// home page, including the player's own standing when comp is a league with
// computed standings and one of playerPairIDs appears in them.
func NewHomeCompetitionView(leagueSvc *league.Service, comp *core.Record, pendingCount int, playerPairIDs map[string]struct{}) CompetitionView {
	return CompetitionView{
		Mode:            PlayerRow,
		Competition:     comp,
		Name:            comp.GetString("name"),
		CompetitionLogo: league.CompetitionLogoURL(comp.Id, comp.GetString("logo")),
		PendingCount:    pendingCount,
		URL:             "/competition/" + comp.Id,
		Standing:        findPlayerStanding(leagueSvc, comp, playerPairIDs),
	}
}

// findPlayerStanding returns the player's own row from comp's standings, or
// nil if comp isn't a league, has no computed standings, or none of
// playerPairIDs appears in them.
func findPlayerStanding(leagueSvc *league.Service, comp *core.Record, playerPairIDs map[string]struct{}) *PlayerStanding {
	if comp.GetString("type") != "league" {
		return nil
	}
	rows, err := leagueSvc.ComputeStandings(comp.Id)
	if err != nil {
		return nil
	}
	for _, r := range rows {
		if _, ok := playerPairIDs[r.PairID]; ok {
			return &PlayerStanding{Position: r.Position, Points: r.Points, HasProvisional: r.HasProvisional}
		}
	}
	return nil
}
