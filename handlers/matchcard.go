package handlers

import (
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"padelleague/league"
)

// MatchCard is the neutral view-model for a match, rendered by
// views/partials/match-card.html gated by Mode axes.
type MatchCard struct {
	Mode            Mode
	Match           *core.Record
	Pair1Name       string
	Pair2Name       string
	CompetitionName string
	RoundNum        int
	StatusLabel     string
	StatusClass     string

	Score          string
	SubmittedBy    string
	ConfirmedBy    string
	SubmittedScore string
	DisputedBy     string
	DisputedScore  string
	DisputeNotes   string
	ReviewType     string
	RequestedBy    string

	Feeder1   string
	Feeder2   string
	IsMyMatch bool

	CanSubmit   bool
	CanConfirm  bool
	CanDispute  bool
	CanEdit     bool
	CanWalkover bool
	CanCorrect  bool

	Venues []*core.Record
}

// NewMatchCard builds the neutral match view-model for the given render mode.
func NewMatchCard(app core.App, match *core.Record, mode Mode, viewerID string) MatchCard {
	status := match.GetString("status")
	pairNames := league.PairNames(app, []string{match.GetString("pair1"), match.GetString("pair2")})
	competitionName := ""
	if competition, err := app.FindRecordById("competitions", match.GetString("competition")); err == nil {
		competitionName = competition.GetString("name")
	}
	c := MatchCard{
		Mode:            mode,
		Match:           match,
		Pair1Name:       pairNames[match.GetString("pair1")],
		Pair2Name:       pairNames[match.GetString("pair2")],
		CompetitionName: competitionName,
		RoundNum:        int(match.GetFloat("round_number")),
		StatusLabel:     statusLabel(status),
		StatusClass:     statusClass(status),
		Score:           match.GetString("scores"),
		SubmittedScore:  match.GetString("scores"),
		DisputedScore:   match.GetString("disputed_scores"),
		DisputeNotes:    match.GetString("dispute_notes"),
		ReviewType:      match.GetString("review_type"),
		SubmittedBy:     pairPlayerLabel(app, match.GetString("submitted_by"), match),
		ConfirmedBy:     pairPlayerLabel(app, match.GetString("confirmed_by"), match),
		DisputedBy:      pairPlayerLabel(app, match.GetString("disputed_by"), match),
		RequestedBy:     playerNameIfSet(app, match.GetString("walkover_requested_by")),
	}
	if mode.Editable && !mode.Admin {
		c.fillPlayerActions(app, match, viewerID)
	}
	return c
}

// NewMatchRow builds a reduced read-only match view-model for list/row surfaces.
func NewMatchRow(match *core.Record, pairNames map[string]string, playerPairIDs map[string]struct{}) MatchCard {
	p1 := match.GetString("pair1")
	p2 := match.GetString("pair2")
	status := match.GetString("status")
	_, myP1 := playerPairIDs[p1]
	_, myP2 := playerPairIDs[p2]
	return MatchCard{
		Mode:        PlayerRow,
		Match:       match,
		Pair1Name:   pairNames[p1],
		Pair2Name:   pairNames[p2],
		RoundNum:    int(match.GetFloat("round_number")),
		StatusLabel: statusLabelShort(status),
		StatusClass: statusClass(status),
		Score:       match.GetString("scores"),
		IsMyMatch:   myP1 || myP2,
	}
}

func statusLabelShort(status string) string {
	switch status {
	case league.StatusPending:
		return "Pendiente"
	case league.StatusConfirmed:
		return "Enviado"
	case league.StatusDisputed:
		return "Disputa"
	case league.StatusFinal:
		return "Final"
	}
	return status
}

func (c *MatchCard) fillPlayerActions(app core.App, match *core.Record, viewerID string) {
	status := match.GetString("status")
	submittedBy := match.GetString("submitted_by")

	team, _ := league.PlayerTeam(app, viewerID, match)
	isSubmitter := false
	if submittedBy != "" {
		submitterTeam, err := league.PlayerTeam(app, submittedBy, match)
		if err == nil {
			isSubmitter = (submitterTeam == team)
		}
	}

	c.CanSubmit = status == league.StatusPending && team > 0
	c.CanConfirm = status == league.StatusConfirmed && team > 0 && !isSubmitter
	c.CanDispute = status == league.StatusConfirmed && team > 0 && !isSubmitter
	c.CanEdit = status == league.StatusPending && team > 0
	c.CanWalkover = canReportUnplayed(status, team)

	if status == league.StatusConfirmed && team > 0 && isSubmitter {
		if submittedAt := match.GetString("submitted_at"); submittedAt != "" {
			if dt, err := types.ParseDateTime(submittedAt); err == nil {
				c.CanCorrect = time.Since(dt.Time()) < 24*time.Hour
			}
		}
	}
}

// PopulateFeeder sets the playoff placeholder text for unresolved pairs.
func (c *MatchCard) PopulateFeeder(prevRound, matchIdx int) {
	if c.Pair1Name == "" {
		c.Feeder1 = fmt.Sprintf("Ganador de J%d-%d", prevRound, matchIdx*2+1)
	}
	if c.Pair2Name == "" {
		c.Feeder2 = fmt.Sprintf("Ganador de J%d-%d", prevRound, matchIdx*2+2)
	}
}

// SummaryLine is the notification-mode rendering: one raw line naming the
// pairs, the score, and the winner.
func (c MatchCard) SummaryLine() string {
	winner := c.Pair2Name
	if c.Match.GetString("winner") == c.Match.GetString("pair1") {
		winner = c.Pair1Name
	}
	return fmt.Sprintf("Resultado: %s %s %s. Ganador: %s!", c.Pair1Name, c.Score, c.Pair2Name, winner)
}

func pairPlayerLabel(app core.App, userID string, match *core.Record) string {
	if userID == "" {
		return ""
	}
	name := league.PlayerName(app, userID)
	team, err := league.PlayerTeam(app, userID, match)
	if err != nil || team == 0 {
		return name
	}
	pairID := match.GetString("pair1")
	if team == 2 {
		pairID = match.GetString("pair2")
	}
	pairName := league.PairNames(app, []string{pairID})[pairID]
	return fmt.Sprintf("%s (%s)", pairName, name)
}
