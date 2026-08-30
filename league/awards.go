package league

import (
	"strconv"

	"github.com/pocketbase/pocketbase/core"
)

// Award represents a competition award (e.g. best pair, longest streak).
type Award struct {
	Title    string
	PairID   string
	PairName string
	Value    string
}

// Awards computes end-of-competition awards based on standings and match history.
func (svc *Service) Awards(competitionID string) []Award {
	standings, err := svc.ComputeStandings(competitionID)
	if err != nil || len(standings) == 0 {
		return nil
	}

	var awards []Award

	best := standings[0]
	if best.Played > 0 {
		awards = append(awards, Award{
			Title:    "Mejor pareja",
			PairID:   best.PairID,
			PairName: best.PairName,
			Value:    strconv.Itoa(best.Points) + " pts",
		})
	}

	mostPlayed := standings[0]
	for _, s := range standings[1:] {
		if s.Played > mostPlayed.Played {
			mostPlayed = s
		}
	}
	if mostPlayed.Played > 0 {
		awards = append(awards, Award{
			Title:    "Más partidos",
			PairID:   mostPlayed.PairID,
			PairName: mostPlayed.PairName,
			Value:    strconv.Itoa(mostPlayed.Played) + " partidos",
		})
	}

	if a := svc.longestStreakAward(competitionID, standings); a != nil {
		awards = append(awards, *a)
	}

	return awards
}

type streakInfo struct {
	pairID  string
	current int
	best    int
}

func (svc *Service) longestStreakAward(competitionID string, standings []StandingRowFull) *Award {
	matches, _ := svc.app.FindRecordsByFilter("matches",
		"competition = {:cid} && status = 'final'",
		"", 0, 0,
		map[string]any{"cid": competitionID})

	streaks := make(map[string]*streakInfo, len(standings))
	for _, s := range standings {
		streaks[s.PairID] = &streakInfo{pairID: s.PairID}
	}

	for _, m := range matches {
		updateStreaks(streaks, m)
	}

	longestPairID, longestStreak := findLongestStreak(standings, streaks)

	if longestStreak <= 1 {
		return nil
	}
	pairNames := PairNames(svc.app, []string{longestPairID})
	return &Award{
		Title:    "Mayor racha",
		PairID:   longestPairID,
		PairName: pairNames[longestPairID],
		Value:    strconv.Itoa(longestStreak) + " victorias",
	}
}

func updateStreaks(streaks map[string]*streakInfo, m *core.Record) {
	p1 := m.GetString("pair1")
	p2 := m.GetString("pair2")
	winner := m.GetString("winner")
	for _, pid := range []string{p1, p2} {
		si, ok := streaks[pid]
		if !ok {
			continue
		}
		if pid == winner {
			si.current++
			if si.current > si.best {
				si.best = si.current
			}
		} else {
			si.current = 0
		}
	}
}

// Walk standings (not the map) for deterministic tie-breaking.
func findLongestStreak(standings []StandingRowFull, streaks map[string]*streakInfo) (string, int) {
	var longestPairID string
	longestStreak := 0
	for _, s := range standings {
		si, ok := streaks[s.PairID]
		if !ok {
			continue
		}
		if si.best > longestStreak {
			longestStreak = si.best
			longestPairID = si.pairID
		}
	}
	return longestPairID, longestStreak
}
