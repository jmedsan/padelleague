package league

import (
	"encoding/json"
	"log/slog"
	"sort"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

// StandingRowFull holds a pair's full standings row including all tiebreaker fields.
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

// ComputeStandings calculates ranked standings for a competition.
func (svc *Service) ComputeStandings(competitionID string) ([]StandingRowFull, error) {
	comp, err := svc.app.FindRecordById("competitions", competitionID)
	if err != nil {
		return nil, err
	}

	pairIDs := comp.GetStringSlice("pairs")
	pairNames := PairNames(svc.app, pairIDs)

	matches, _ := svc.app.FindRecordsByFilter("matches",
		"competition = {:cid} && status = 'final'",
		"", 0, 0,
		map[string]any{"cid": competitionID})

	pairStats := tallyMatchStats(pairIDs, matches)
	penaltyMap := parsePenalties(comp)
	rows := buildStandingRows(pairIDs, pairNames, pairStats, penaltyMap)
	sortStandings(rows, matches)

	for i := range rows {
		rows[i].Position = i + 1
	}
	return rows, nil
}

type pairStats struct {
	wins, losses, setsWon, setsLost, gamesWon, gamesLost int
}

func tallyMatchStats(pairIDs []string, matches []*core.Record) map[string]*pairStats {
	stats := make(map[string]*pairStats, len(pairIDs))
	for _, pid := range pairIDs {
		stats[pid] = &pairStats{}
	}
	for _, m := range matches {
		tallyMatch(stats, m)
	}
	return stats
}

func tallyMatch(stats map[string]*pairStats, m *core.Record) {
	p1 := m.GetString("pair1")
	p2 := m.GetString("pair2")
	winner := m.GetString("winner")
	score := m.GetString("scores")

	s1, ok1 := stats[p1]
	s2, ok2 := stats[p2]
	if !ok1 || !ok2 {
		return
	}

	creditWin(s1, s2, winner, p1, p2)

	if strings.EqualFold(strings.TrimSpace(score), "WO") {
		return
	}
	sc, err := ParseScore(score)
	if err != nil {
		return
	}
	s1.setsWon += sc.Sets1
	s1.setsLost += sc.Sets2
	s1.gamesWon += sc.Games1
	s1.gamesLost += sc.Games2
	s2.setsWon += sc.Sets2
	s2.setsLost += sc.Sets1
	s2.gamesWon += sc.Games2
	s2.gamesLost += sc.Games1
}

func creditWin(s1, s2 *pairStats, winner, p1, p2 string) {
	switch winner {
	case p1:
		s1.wins++
		s2.losses++
	case p2:
		s2.wins++
		s1.losses++
	}
}

func parsePenalties(comp *core.Record) map[string]float64 {
	penaltyMap := map[string]float64{}
	rawPen := comp.Get("penalty_points")
	switch v := rawPen.(type) {
	case types.JSONRaw:
		if len(v) > 0 {
			if err := json.Unmarshal(v, &penaltyMap); err != nil {
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
	return penaltyMap
}

func buildStandingRows(pairIDs []string, pairNames map[string]string, stats map[string]*pairStats, penaltyMap map[string]float64) []StandingRowFull {
	rows := make([]StandingRowFull, 0, len(pairIDs))
	for _, pid := range pairIDs {
		s := stats[pid]
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
	return rows
}

func sortStandings(rows []StandingRowFull, matches []*core.Record) {
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
		return headToHead(rows[i].PairID, rows[j].PairID, matches)
	})
}

func headToHead(pairA, pairB string, matches []*core.Record) bool {
	winsA, winsB := 0, 0
	for _, m := range matches {
		p1 := m.GetString("pair1")
		p2 := m.GetString("pair2")
		winner := m.GetString("winner")
		if (p1 == pairA && p2 == pairB) || (p1 == pairB && p2 == pairA) {
			switch winner {
			case pairA:
				winsA++
			case pairB:
				winsB++
			}
		}
	}
	return winsA > winsB
}
