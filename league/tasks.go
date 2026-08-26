package league

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// TaskKind ranks the urgency of player tasks (lower = more urgent).
type TaskKind int

// Task urgency levels (lower = more urgent).
const (
	TaskDispute  TaskKind = 1
	TaskPlay     TaskKind = 2
	TaskOrganize TaskKind = 3
)

// PlayerTask represents one action the player should take.
type PlayerTask struct {
	Kind            TaskKind
	MatchID         string
	Opponent        string
	CompetitionName string
	RoundNumber     int
	Description     string
	ArrangeBy       string // "DD/MM" deadline for organize tasks
	Warning         Warning
	ScheduleStatus  string // "confirmed"/"proposed"/"" for play tasks
	ProposedDate    string
	ProposedVenue   string
}

// PlayerTasks returns the player's urgent tasks across all active competitions,
// ranked: disputes first, then matches to play, then matches to organize.
func PlayerTasks(app core.App, userID string, now time.Time) ([]PlayerTask, error) {
	pairs, err := PairsForPlayer(app, userID)
	if err != nil {
		return nil, err
	}
	playerPairIDs := make(map[string]bool, len(pairs))
	for _, p := range pairs {
		playerPairIDs[p.Id] = true
	}

	comps, err := app.FindRecordsByFilter("competitions",
		"active = true", "", 0, 0, nil)
	if err != nil {
		return nil, err
	}

	var tasks []PlayerTask

	for _, c := range comps {
		compPairs := c.GetStringSlice("pairs")
		inComp := false
		for _, pid := range compPairs {
			if playerPairIDs[pid] {
				inComp = true
				break
			}
		}
		if !inComp {
			continue
		}

		compTasks := competitionTasks(app, c, playerPairIDs, now)
		tasks = append(tasks, compTasks...)
	}

	sortTasks(tasks)
	return tasks, nil
}

func competitionTasks(app core.App, comp *core.Record, playerPairIDs map[string]bool, now time.Time) []PlayerTask {
	compName := comp.GetString("name")

	var tasks []PlayerTask

	// Disputes involving the player
	disputed, _ := app.FindRecordsByFilter("matches",
		"competition = {:cid} && status = 'disputed'",
		"round_number", 0, 0,
		map[string]any{"cid": comp.Id})
	for _, m := range disputed {
		if !playerPairIDs[m.GetString("pair1")] && !playerPairIDs[m.GetString("pair2")] {
			continue
		}
		tasks = append(tasks, PlayerTask{
			Kind:            TaskDispute,
			MatchID:         m.Id,
			Opponent:        opponentName(app, m, playerPairIDs),
			CompetitionName: compName,
			RoundNumber:     m.GetInt("round_number"),
			Description:     "Disputa abierta",
		})
	}

	pending, _ := app.FindRecordsByFilter("matches",
		"competition = {:cid} && status = 'pending'",
		"round_number", 0, 0,
		map[string]any{"cid": comp.Id})

	pendingTasks := pendingMatchTasks(app, comp, pending, playerPairIDs, compName, now)
	tasks = append(tasks, pendingTasks...)
	return tasks
}

func pendingMatchTasks(app core.App, comp *core.Record, pending []*core.Record, playerPairIDs map[string]bool, compName string, now time.Time) []PlayerTask {
	graceDays := comp.GetInt("arrange_grace_days")

	var tasks []PlayerTask
	for _, m := range pending {
		if !playerPairIDs[m.GetString("pair1")] && !playerPairIDs[m.GetString("pair2")] {
			continue
		}
		roundNum := m.GetInt("round_number")
		opponent := opponentName(app, m, playerPairIDs)

		if hasAcceptedProposal(app, m.Id) {
			task := PlayerTask{
				Kind: TaskPlay, MatchID: m.Id, Opponent: opponent,
				CompetitionName: compName, RoundNumber: roundNum,
				Description: "Próximo partido", ScheduleStatus: "confirmed",
			}
			enrichPlaySchedule(app, m.Id, &task)
			tasks = append(tasks, task)
			continue
		}

		deadline, ok := RoundArrangeDate(comp, roundNum)
		if !ok {
			continue
		}
		wl := WarningLevel(deadline, graceDays, now)
		if wl >= WarnHeadsUp {
			tasks = append(tasks, PlayerTask{
				Kind: TaskOrganize, MatchID: m.Id, Opponent: opponent,
				CompetitionName: compName, RoundNumber: roundNum,
				Description: fmt.Sprintf("Organiza antes del %s", deadline.Format("02/01")),
				ArrangeBy:   deadline.Format("02/01"), Warning: wl,
			})
		}
	}
	return tasks
}

func enrichPlaySchedule(app core.App, matchID string, task *PlayerTask) {
	proposals, _ := app.FindRecordsByFilter("match_messages",
		"match = {:mid} && type = 'scheduling_proposal' && proposal_status != 'rejected' && proposal_status != 'superseded'",
		"-created", 1, 0, map[string]any{"mid": matchID})
	if len(proposals) == 0 {
		return
	}
	prop := proposals[0]
	if prop.GetString("proposal_status") == "accepted" {
		task.ScheduleStatus = "confirmed"
	} else {
		task.ScheduleStatus = "proposed"
	}
	raw := prop.GetString("proposal_data")
	if raw == "" {
		return
	}
	var pd struct {
		Date      string `json:"date"`
		Time      string `json:"time"`
		VenueName string `json:"venue_name"`
		VenueText string `json:"venue_text"`
	}
	if json.Unmarshal([]byte(raw), &pd) != nil {
		return
	}
	task.ProposedDate = pd.Date + " " + pd.Time
	if pd.VenueName != "" {
		task.ProposedVenue = pd.VenueName
	} else if pd.VenueText != "" {
		task.ProposedVenue = pd.VenueText
	}
}

func opponentName(app core.App, m *core.Record, playerPairIDs map[string]bool) string {
	opponent := m.GetString("pair1")
	if playerPairIDs[opponent] {
		opponent = m.GetString("pair2")
	}
	if pair, err := app.FindRecordById("pairs", opponent); err == nil {
		return pair.GetString("name")
	}
	return "?"
}

func hasAcceptedProposal(app core.App, matchID string) bool {
	accepted, _ := app.FindRecordsByFilter("match_messages",
		"match = {:mid} && type = 'scheduling_proposal' && proposal_status = 'accepted'",
		"", 1, 0, map[string]any{"mid": matchID})
	return len(accepted) > 0
}

func sortTasks(tasks []PlayerTask) {
	for i := 1; i < len(tasks); i++ {
		for j := i; j > 0; j-- {
			if taskLess(tasks[j], tasks[j-1]) {
				tasks[j], tasks[j-1] = tasks[j-1], tasks[j]
			}
		}
	}
}

func taskLess(a, b PlayerTask) bool {
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if a.Kind == TaskOrganize {
		return a.Warning > b.Warning
	}
	return a.RoundNumber < b.RoundNumber
}
