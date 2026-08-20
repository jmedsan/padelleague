package handlers

import (
	"strconv"

	"github.com/pocketbase/pocketbase/core"
)

type Award struct {
	Title    string
	PairName string
	Value    string
}

func computeAwards(app core.App, competitionID string) []Award {
	standings, err := ComputeStandings(app, competitionID)
	if err != nil || len(standings) == 0 {
		return nil
	}

	var awards []Award

	best := standings[0]
	if best.Played > 0 {
		awards = append(awards, Award{
			Title:    "Mejor pareja",
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
			PairName: mostPlayed.PairName,
			Value:    strconv.Itoa(mostPlayed.Played) + " partidos",
		})
	}

	matches, _ := app.FindRecordsByFilter("matches",
		"competition = {:cid} && status = 'final'",
		"", 0, 0,
		map[string]any{"cid": competitionID})

	type streakInfo struct {
		pairID  string
		current int
		best    int
	}
	streaks := make(map[string]*streakInfo)
	for _, s := range standings {
		streaks[s.PairID] = &streakInfo{pairID: s.PairID}
	}

	for _, m := range matches {
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

	var longestPairID string
	longestStreak := 0
	for _, si := range streaks {
		if si.best > longestStreak {
			longestStreak = si.best
			longestPairID = si.pairID
		}
	}

	if longestStreak > 1 {
		pairNames, _ := expandPairNames(app, []string{longestPairID})
		awards = append(awards, Award{
			Title:    "Mayor racha",
			PairName: pairNames[longestPairID],
			Value:    strconv.Itoa(longestStreak) + " victorias",
		})
	}

	return awards
}
