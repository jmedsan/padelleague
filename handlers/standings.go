package handlers

import (
	"sort"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

type StandingRowFull struct {
	Position  int
	PairID    string
	PairName  string
	ELO       int
	Played    int
	Wins      int
	Losses    int
	SetsWon   int
	SetsLost  int
	GamesWon  int
	GamesLost int
	Points    int
}

func ComputeStandings(app core.App, competitionID string) ([]StandingRowFull, error) {
	compPairs, err := app.FindRecordsByFilter("competition_pairs",
		"competition = {:cid}",
		"", 0, 0,
		map[string]any{"cid": competitionID})
	if err != nil {
		return nil, err
	}

	pairIDs := make([]string, len(compPairs))
	for i, cp := range compPairs {
		pairIDs[i] = cp.GetString("pair")
	}

	pairNames, _ := expandPairNames(app, pairIDs)

	pairELO := make(map[string]int, len(pairIDs))
	for _, pid := range pairIDs {
		players := getPlayersForPair(app, pid)
		totalELO := 0
		count := 0
		for _, uid := range players {
			u, err := app.FindRecordById("users", uid)
			if err != nil {
				continue
			}
			elo := int(u.GetFloat("elo"))
			if elo == 0 {
				elo = 1500
			}
			totalELO += elo
			count++
		}
		if count > 0 {
			pairELO[pid] = totalELO / count
		} else {
			pairELO[pid] = 1500
		}
	}

	matchdays, err := app.FindRecordsByFilter("matchdays",
		"competition = {:cid}",
		"", 0, 0,
		map[string]any{"cid": competitionID})
	if err != nil {
		return nil, err
	}

	type stats struct {
		wins, losses, setsWon, setsLost, gamesWon, gamesLost int
	}
	pairStats := make(map[string]*stats, len(pairIDs))
	for _, pid := range pairIDs {
		pairStats[pid] = &stats{}
	}

	for _, md := range matchdays {
		matches, _ := app.FindRecordsByFilter("matches",
			"matchday = {:mid} && status = 'final'",
			"", 0, 0,
			map[string]any{"mid": md.Id})

		for _, m := range matches {
			p1 := m.GetString("pair1")
			p2 := m.GetString("pair2")
			winner := m.GetString("winner")
			score := m.GetString("scores")

			s1, ok1 := pairStats[p1]
			s2, ok2 := pairStats[p2]
			if !ok1 || !ok2 {
				continue
			}

			if strings.EqualFold(strings.TrimSpace(score), "WO") {
				if winner == p1 {
					s1.wins++
					s2.losses++
				} else if winner == p2 {
					s2.wins++
					s1.losses++
				}
				continue
			}

			sets1, sets2, games1, games2, err := parseScore(score)
			if err != nil {
				continue
			}

			s1.setsWon += sets1
			s1.setsLost += sets2
			s1.gamesWon += games1
			s1.gamesLost += games2

			s2.setsWon += sets2
			s2.setsLost += sets1
			s2.gamesWon += games2
			s2.gamesLost += games1

			if winner == p1 {
				s1.wins++
				s2.losses++
			} else if winner == p2 {
				s2.wins++
				s1.losses++
			}
		}
	}

	rows := make([]StandingRowFull, 0, len(pairIDs))
	for _, pid := range pairIDs {
		s := pairStats[pid]
		rows = append(rows, StandingRowFull{
			PairID:    pid,
			PairName:  pairNames[pid],
			ELO:       pairELO[pid],
			Played:    s.wins + s.losses,
			Wins:      s.wins,
			Losses:    s.losses,
			SetsWon:   s.setsWon,
			SetsLost:  s.setsLost,
			GamesWon:  s.gamesWon,
			GamesLost: s.gamesLost,
			Points:    s.wins * 3,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Points != rows[j].Points {
			return rows[i].Points > rows[j].Points
		}
		setDiffI := rows[i].SetsWon - rows[i].SetsLost
		setDiffJ := rows[j].SetsWon - rows[j].SetsLost
		if setDiffI != setDiffJ {
			return setDiffI > setDiffJ
		}
		gameDiffI := rows[i].GamesWon - rows[i].GamesLost
		gameDiffJ := rows[j].GamesWon - rows[j].GamesLost
		return gameDiffI > gameDiffJ
	})

	for i := range rows {
		rows[i].Position = i + 1
	}

	return rows, nil
}
