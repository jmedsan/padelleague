package handlers

import (
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"padelleague/league"
)

// ScoreInputVM drives the shared score-entry partial (views/partials/score-input.html).
type ScoreInputVM struct {
	FieldName string // hidden field the handler reads: "scores" | "score" | "counter_scores"
	Value     string // prefill value, e.g. "6-3 4-6"; empty for blank entry
	IDSuffix  string // unique per instance on a page (usually the match ID)
	Pair1Name string
	Pair2Name string
}

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

	Score             string
	SubmittedScore    string
	SubmitterPairName string
	DisputedScore     string
	DisputeNotes      string
	DisputerPairName  string
	ReviewType        string
	RequestedBy       string

	Feeder1   string
	Feeder2   string
	IsMyMatch bool

	CanSubmit       bool
	CanEdit         bool
	CanWalkover     bool
	CanCorrect      bool
	HasDateAndPlace bool

	Venues []*core.Record

	ScoreSubmit   ScoreInputVM
	ScoreCorrect  ScoreInputVM
	ScoreResolve  ScoreInputVM
	ScoreOverride ScoreInputVM
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
		Mode:              mode,
		Match:             match,
		Pair1Name:         pairNames[match.GetString("pair1")],
		Pair2Name:         pairNames[match.GetString("pair2")],
		CompetitionName:   competitionName,
		RoundNum:          int(match.GetFloat("round_number")),
		StatusLabel:       league.StatusLabel(status),
		StatusClass:       statusClass(status),
		Score:             match.GetString("scores"),
		SubmittedScore:    match.GetString("scores"),
		DisputedScore:     match.GetString("disputed_scores"),
		DisputeNotes:      match.GetString("dispute_notes"),
		ReviewType:        match.GetString("review_type"),
		SubmitterPairName: userPairName(app, match.GetString("submitted_by"), match, pairNames),
		DisputerPairName:  userPairName(app, match.GetString("disputed_by"), match, pairNames),
		RequestedBy:       playerNameIfSet(app, match.GetString("walkover_requested_by")),
	}
	if mode.Editable && !mode.Admin {
		c.fillPlayerActions(app, match, viewerID)
	}
	if mode.Editable && mode.Admin {
		c.fillAdminScoreVMs(match)
	}
	if c.Pair1Name == "" || c.Pair2Name == "" {
		if prevRound, matchIdx, _, ok := league.PlayoffFeederInfo(app, match); ok {
			c.PopulateFeeder(prevRound, matchIdx)
		}
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
		StatusLabel: league.StatusLabel(status),
		StatusClass: statusClass(status),
		Score:       match.GetString("scores"),
		IsMyMatch:   myP1 || myP2,
	}
}

func (c *MatchCard) fillPlayerActions(app core.App, match *core.Record, viewerID string) {
	status := match.GetString("status")
	team, _ := league.PlayerTeam(app, viewerID, match)
	isSubmitter := viewerIsSubmitter(app, match, team)

	c.HasDateAndPlace = match.GetString("date") != "" && match.GetString("club") != ""
	c.CanSubmit = league.IsPreScore(status) && team > 0
	c.CanEdit = league.IsPreScore(status) && team > 0
	c.CanWalkover = canReportUnplayed(status, team)
	c.CanCorrect = isSubmitter && canCorrectNow(match, status)

	mid := match.Id
	c.ScoreSubmit = ScoreInputVM{FieldName: "scores", IDSuffix: mid, Pair1Name: c.Pair1Name, Pair2Name: c.Pair2Name}
	c.ScoreCorrect = ScoreInputVM{FieldName: "scores", Value: match.GetString("scores"), IDSuffix: mid + "-correct", Pair1Name: c.Pair1Name, Pair2Name: c.Pair2Name}

}

func viewerIsSubmitter(app core.App, match *core.Record, viewerTeam int) bool {
	submittedBy := match.GetString("submitted_by")
	if submittedBy == "" || viewerTeam == 0 {
		return false
	}
	submitterTeam, err := league.PlayerTeam(app, submittedBy, match)
	return err == nil && submitterTeam == viewerTeam
}

func canCorrectNow(match *core.Record, status string) bool {
	if !league.IsPreScore(status) && status != league.StatusConfirmed {
		return false
	}
	submittedAt := match.GetString("submitted_at")
	if submittedAt == "" {
		return false
	}
	dt, err := types.ParseDateTime(submittedAt)
	return err == nil && time.Since(dt.Time()) < resultCorrectionWindow
}

func (c *MatchCard) fillAdminScoreVMs(match *core.Record) {
	mid := match.Id
	c.ScoreResolve = ScoreInputVM{FieldName: "score", Value: c.SubmittedScore, IDSuffix: mid + "-resolve", Pair1Name: c.Pair1Name, Pair2Name: c.Pair2Name}
	c.ScoreOverride = ScoreInputVM{FieldName: "scores", Value: match.GetString("scores"), IDSuffix: mid + "-override", Pair1Name: c.Pair1Name, Pair2Name: c.Pair2Name}
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

func matchParticipantUserIDs(app core.App, match *core.Record) []string {
	p1 := league.PlayersForPair(app, match.GetString("pair1"))
	p2 := league.PlayersForPair(app, match.GetString("pair2"))
	return append(p1, p2...)
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
	return fmt.Sprintf("%s (%s)", name, pairName)
}

func userPairName(app core.App, userID string, match *core.Record, pairNames map[string]string) string {
	if userID == "" {
		return ""
	}
	team, err := league.PlayerTeam(app, userID, match)
	if err != nil || team == 0 {
		return ""
	}
	pairID := match.GetString("pair1")
	if team == 2 {
		pairID = match.GetString("pair2")
	}
	return pairNames[pairID]
}
