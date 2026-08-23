package league

import (
	"encoding/json"
	"log/slog"
	"sort"
	"strings"

	"github.com/pocketbase/pocketbase/tools/types"
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

func (svc *Service) ComputeStandings(competitionID string) ([]StandingRowFull, error) {
	comp, err := svc.App.FindRecordById("competitions", competitionID)
	if err != nil {
		return nil, err
	}

	pairIDs := comp.GetStringSlice("pairs")
	pairNames := PairNames(svc.App, pairIDs)

	matches, _ := svc.App.FindRecordsByFilter("matches",
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
			switch winner {
			case p1:
				s1.wins++
				s2.losses++
			case p2:
				s2.wins++
				s1.losses++
			}
			continue
		}

		sc, err := ParseScore(score)
		if err != nil {
			continue
		}

		s1.setsWon += sc.Sets1
		s1.setsLost += sc.Sets2
		s1.gamesWon += sc.Games1
		s1.gamesLost += sc.Games2

		s2.setsWon += sc.Sets2
		s2.setsLost += sc.Sets1
		s2.gamesWon += sc.Games2
		s2.gamesLost += sc.Games1

		switch winner {
		case p1:
			s1.wins++
			s2.losses++
		case p2:
			s2.wins++
			s1.losses++
		}
	}

	penaltyMap := map[string]float64{}
	rawPen := comp.Get("penalty_points")
	switch v := rawPen.(type) {
	case types.JSONRaw:
		if len(v) > 0 {
			if err := json.Unmarshal(v, &penaltyMap); err != nil {
				slog.Warn("unmarshal penalty_points", "err", err)
			}
		}
	case string:
		if v != "" {
			if err := json.Unmarshal([]byte(v), &penaltyMap); err != nil {
				slog.Warn("unmarshal penalty_points", "err", err)
			}
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
