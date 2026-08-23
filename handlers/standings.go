package handlers

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

type StandingRowFull struct {
	Position  int
	PairID    string
	PairName  string
	Played    int
	Wins      int
	Losses    int
	SetsWon   int
	SetsLost  int
	GamesWon  int
	GamesLost int
	Points    int
	Penalty   int
}

func ComputeStandings(app core.App, competitionID string) ([]StandingRowFull, error) {
	comp, err := app.FindRecordById("competitions", competitionID)
	if err != nil {
		return nil, err
	}

	pairIDs := comp.GetStringSlice("pairs")

	pairNames, _ := expandPairNames(app, pairIDs)

	matches, _ := app.FindRecordsByFilter("matches",
		"competition = {:cid} && status = 'final'",
		"", 0, 0,
		map[string]any{"cid": competitionID})

	type stats struct {
		wins, losses, setsWon, setsLost, gamesWon, gamesLost int
	}
	pairStats := make(map[string]*stats, len(pairIDs))
	for _, pid := range pairIDs {
		pairStats[pid] = &stats{}
	}

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

	penaltyMap := map[string]float64{}
	rawPen := comp.Get("penalty_points")
	switch v := rawPen.(type) {
	case string:
		if v != "" {
			json.Unmarshal([]byte(v), &penaltyMap)
		}
	case map[string]any:
		for k, val := range v {
			if f, ok := val.(float64); ok {
				penaltyMap[k] = f
			}
		}
	}

	rows := make([]StandingRowFull, 0, len(pairIDs))
	for _, pid := range pairIDs {
		s := pairStats[pid]
		penalty := int(penaltyMap[pid])
		rows = append(rows, StandingRowFull{
			PairID:    pid,
			PairName:  pairNames[pid],
			Played:    s.wins + s.losses,
			Wins:      s.wins,
			Losses:    s.losses,
			SetsWon:   s.setsWon,
			SetsLost:  s.setsLost,
			GamesWon:  s.gamesWon,
			GamesLost: s.gamesLost,
			Points:    s.wins*3 - penalty,
			Penalty:   penalty,
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
		if gameDiffI != gameDiffJ {
			return gameDiffI > gameDiffJ
		}
		h2hI, h2hJ := 0, 0
		for _, m := range matches {
			p1 := m.GetString("pair1")
			p2 := m.GetString("pair2")
			winner := m.GetString("winner")
			if (p1 == rows[i].PairID && p2 == rows[j].PairID) || (p1 == rows[j].PairID && p2 == rows[i].PairID) {
				if winner == rows[i].PairID {
					h2hI++
				} else if winner == rows[j].PairID {
					h2hJ++
				}
			}
		}
		return h2hI > h2hJ
	})

	for i := range rows {
		rows[i].Position = i + 1
	}

	return rows, nil
}
