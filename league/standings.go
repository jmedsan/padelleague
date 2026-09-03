package league

import (
	"sort"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// standingFormLimit is how many recent results StandingRowFull.Form carries.
const standingFormLimit = 5

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
	// Form holds up to the last standingFormLimit results, most recent
	// first: true = win, false = loss.
	Form []bool
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
	penaltyMap, err := PenaltyTotals(svc.app, competitionID)
	if err != nil {
		return nil, err
	}
	rows := buildStandingRows(standingRowInputs{
		pairIDs:    pairIDs,
		pairNames:  pairNames,
		stats:      pairStats,
		penaltyMap: penaltyMap,
		matches:    matches,
	})
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

	switch winner {
	case p1:
		s1.wins++
		s2.losses++
	case p2:
		s2.wins++
		s1.losses++
	}

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

// standingRowInputs bundles the per-competition data buildStandingRows needs,
// keeping the function under the project's argument-count lint limit.
type standingRowInputs struct {
	pairIDs    []string
	pairNames  map[string]string
	stats      map[string]*pairStats
	penaltyMap map[string]float64
	matches    []*core.Record
}

func buildStandingRows(in standingRowInputs) []StandingRowFull {
	rows := make([]StandingRowFull, 0, len(in.pairIDs))
	for _, pid := range in.pairIDs {
		s := in.stats[pid]
		penalty := int(in.penaltyMap[pid])
		rows = append(rows, StandingRowFull{
			PairID:    pid,
			PairName:  in.pairNames[pid],
			Played:    s.wins + s.losses,
			Wins:      s.wins,
			Losses:    s.losses,
			SetsWon:   s.setsWon,
			SetsLost:  s.setsLost,
			GamesWon:  s.gamesWon,
			GamesLost: s.gamesLost,
			Points:    s.wins*3 - penalty,
			Penalty:   penalty,
			Form:      pairForm(pid, in.matches),
		})
	}
	return rows
}

// pairForm returns up to standingFormLimit results for pid from matches,
// most recent first (true = win, false = loss), ordered by the match date.
func pairForm(pid string, matches []*core.Record) []bool {
	var involved []*core.Record
	for _, m := range matches {
		if m.GetString("pair1") == pid || m.GetString("pair2") == pid {
			involved = append(involved, m)
		}
	}
	sort.Slice(involved, func(i, j int) bool {
		return involved[i].GetString("date") > involved[j].GetString("date")
	})

	limit := min(len(involved), standingFormLimit)
	form := make([]bool, limit)
	for i := 0; i < limit; i++ {
		form[i] = involved[i].GetString("winner") == pid
	}
	return form
}

// sortStandings ranks pairs by FEP Reglamento Técnico General 2024 art.
// 3.3.10. Rows are first sorted by points; consecutive rows level on points
// form a tie group, resolved by size:
//   - 1 pair: nothing to resolve.
//   - 2 pairs: head-to-head result, then head-to-head set/game diff (their
//     mutual matches only), then overall set diff → game diff.
//   - 3+ pairs: a mini-league among just the tied pairs (points, then set
//     diff, then game diff from their mutual matches only), then overall set
//     diff → game diff.
//
// A pairwise head-to-head comparator is not transitive across 3+ pairs
// (A beats B, B beats C, C beats A is a valid cycle), so it cannot be used
// as a sort.Slice Less function for groups above size 2 — the mini-league
// resolves the cycle by scoring standalone, not through pairwise diffs.
// Pair name is the deterministic final tiebreaker, so output is stable
// regardless of input order.
func sortStandings(rows []StandingRowFull, matches []*core.Record) {
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Points > rows[j].Points
	})

	for start := 0; start < len(rows); {
		end := start + 1
		for end < len(rows) && rows[end].Points == rows[start].Points {
			end++
		}
		resolveTieGroup(rows[start:end], matches)
		start = end
	}
}

func resolveTieGroup(group []StandingRowFull, matches []*core.Record) {
	switch len(group) {
	case 0, 1:
		return
	case 2:
		resolveTwoWayTie(group, matches)
	default:
		resolveMiniLeague(group, matches)
	}
}

// resolveTwoWayTie orders two pairs level on points by head-to-head result,
// then by set/game diff computed from their mutual matches only, then by
// overall set diff → game diff → pair name.
func resolveTwoWayTie(group []StandingRowFull, matches []*core.Record) {
	a, b := group[0], group[1]
	mutual := matchesBetween(matches, []string{a.PairID, b.PairID})
	h2hStats := tallyMatchStats([]string{a.PairID, b.PairID}, mutual)

	sort.SliceStable(group, func(i, j int) bool {
		return lessByH2HThenOverall(group[i], group[j], h2hStats)
	})
}

func lessByH2HThenOverall(a, b StandingRowFull, h2hStats map[string]*pairStats) bool {
	sa, sb := h2hStats[a.PairID], h2hStats[b.PairID]
	if sa.wins != sb.wins {
		return sa.wins > sb.wins
	}
	if setDiff := sa.setsWon - sa.setsLost - (sb.setsWon - sb.setsLost); setDiff != 0 {
		return setDiff > 0
	}
	if gameDiff := sa.gamesWon - sa.gamesLost - (sb.gamesWon - sb.gamesLost); gameDiff != 0 {
		return gameDiff > 0
	}
	return lessByOverallThenName(a, b)
}

// resolveMiniLeague orders 3+ pairs level on points by a sub-standings table
// computed from matches played among just the tied pairs (points, then set
// diff, then game diff), then falls back to overall set diff → game diff →
// pair name for anything the mini-league itself leaves tied.
func resolveMiniLeague(group []StandingRowFull, matches []*core.Record) {
	pairIDs := make([]string, len(group))
	for i, r := range group {
		pairIDs[i] = r.PairID
	}
	mutual := matchesBetween(matches, pairIDs)
	miniStats := tallyMatchStats(pairIDs, mutual)

	sort.SliceStable(group, func(i, j int) bool {
		a, b := group[i], group[j]
		ma, mb := miniStats[a.PairID], miniStats[b.PairID]
		miniPointsA, miniPointsB := ma.wins*3, mb.wins*3
		if miniPointsA != miniPointsB {
			return miniPointsA > miniPointsB
		}
		if setDiff := ma.setsWon - ma.setsLost - (mb.setsWon - mb.setsLost); setDiff != 0 {
			return setDiff > 0
		}
		if gameDiff := ma.gamesWon - ma.gamesLost - (mb.gamesWon - mb.gamesLost); gameDiff != 0 {
			return gameDiff > 0
		}
		return lessByOverallThenName(a, b)
	})
}

// lessByOverallThenName is the last-resort comparator shared by both tie
// group sizes: overall set diff → overall game diff → pair name, so output
// never depends on input/registration order.
func lessByOverallThenName(a, b StandingRowFull) bool {
	setDiffA, setDiffB := a.SetsWon-a.SetsLost, b.SetsWon-b.SetsLost
	if setDiffA != setDiffB {
		return setDiffA > setDiffB
	}
	gameDiffA, gameDiffB := a.GamesWon-a.GamesLost, b.GamesWon-b.GamesLost
	if gameDiffA != gameDiffB {
		return gameDiffA > gameDiffB
	}
	return a.PairName < b.PairName
}

// matchesBetween returns the final matches played between two or more of
// the given pairs, excluding matches involving any pair outside the set.
func matchesBetween(matches []*core.Record, pairIDs []string) []*core.Record {
	inGroup := make(map[string]bool, len(pairIDs))
	for _, pid := range pairIDs {
		inGroup[pid] = true
	}
	var out []*core.Record
	for _, m := range matches {
		if inGroup[m.GetString("pair1")] && inGroup[m.GetString("pair2")] {
			out = append(out, m)
		}
	}
	return out
}
