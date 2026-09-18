package league

import (
	"math"
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
	// Adjustment is the schedule-strength correction for leveled leagues
	// (recipe §4). Zero for round-robin leagues.
	Adjustment float64
	// Score is Points + Adjustment (rounded to 0.1). Equal to Points for
	// round-robin leagues, so the sort is byte-identical.
	Score float64
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

	if IsLeveled(comp) {
		applyScheduleStrengthAdjustment(rows, pairStats, matches)
	} else {
		for i := range rows {
			rows[i].Score = float64(rows[i].Points)
		}
	}

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
	sc, ok := TallyScore(score)
	if !ok {
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

// sortStandings ranks pairs by Liga Dale Fuerte tiebreaker rules (FEP RTG 2026
// §3.3.10 supplemented by the league rulebook). Rows are first sorted by
// points; consecutive rows level on points form a tie group resolved by size:
//   - 1 pair: nothing to resolve.
//   - 2 pairs: matches played → head-to-head result → mutual set/game diff →
//     overall set diff → overall game diff → pair name.
//   - 3+ pairs: recursive mini-league partition (matches played → mini-league
//     wins → mini set diff → mini game diff); separated pairs recurse on the
//     remaining sub-group; unseparated sub-groups fall to overall criteria.
//
// Pair name is the deterministic final tiebreaker, so output is stable
// regardless of input order.
func sortStandings(rows []StandingRowFull, matches []*core.Record) {
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Score > rows[j].Score
	})

	for start := 0; start < len(rows); {
		end := start + 1
		for end < len(rows) && rows[end].Score == rows[start].Score {
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

// resolveTwoWayTie orders two pairs level on points.
// Chain: matches played → head-to-head result → mutual set diff →
// mutual game diff → overall set diff → overall game diff → pair name.
func resolveTwoWayTie(group []StandingRowFull, matches []*core.Record) {
	a, b := group[0], group[1]
	mutual := matchesBetween(matches, []string{a.PairID, b.PairID})
	h2hStats := tallyMatchStats([]string{a.PairID, b.PairID}, mutual)

	sort.SliceStable(group, func(i, j int) bool {
		return lessTwoWay(group[i], group[j], h2hStats)
	})
}

func lessTwoWay(a, b StandingRowFull, h2hStats map[string]*pairStats) bool {
	// 1. Matches played (more = ranks higher)
	if a.Played != b.Played {
		return a.Played > b.Played
	}
	// 2-4. Head-to-head: wins, set diff, game diff in mutual matches
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
	// 5. Overall stats, then pair name
	return lessByOverallThenName(a, b)
}

// resolveMiniLeague orders 3+ pairs tied on points using the FEP RTG 2026 §3.3.10
// recursive partition algorithm, adapted for the Liga Dale Fuerte rulebook.
//
// Pre-step: sort by matches played (overall, more = higher). Pairs that separate
// on this criterion are done; pairs still tied proceed to the mini-league chain.
//
// Mini-league chain (applied fresh at each recursion level per §3.3.10 P2):
//  1. Mini wins among the current sub-group.
//  2. Mini set diff among the current sub-group.
//  3. Mini game diff among the current sub-group.
//
// After each criterion, separated sub-groups recurse (recomputing fresh stats).
// Sub-groups of size 2 use the two-way rule. Groups unseparated after all three
// mini criteria fall to overall set diff → game diff → pair name.
func resolveMiniLeague(group []StandingRowFull, matches []*core.Record) {
	// Pre-step: partidos jugados (overall matches played). Sort and partition;
	// any sub-group still tied on Played proceeds to the mini-league chain.
	sort.SliceStable(group, func(i, j int) bool {
		return group[i].Played > group[j].Played
	})
	for start := 0; start < len(group); {
		end := start + 1
		for end < len(group) && group[end].Played == group[start].Played {
			end++
		}
		sub := group[start:end]
		if len(sub) >= 2 {
			miniLeaguePartition(sub, matches)
		}
		start = end
	}
}

// miniLeaguePartition applies the mini-league criteria chain to group,
// recomputing mutual stats fresh for the current group at each call (§3.3.10 P2).
func miniLeaguePartition(group []StandingRowFull, matches []*core.Record) {
	if len(group) <= 1 {
		return
	}
	if len(group) == 2 {
		resolveTwoWayTie(group, matches)
		return
	}

	pairIDs := pairIDsOf(group)
	mutual := matchesBetween(matches, pairIDs)
	mini := tallyMatchStats(pairIDs, mutual)

	// Try each mini criterion in order. After each, recurse on still-tied sub-groups.
	// If a criterion partitions the group, the sub-groups recurse from the top of
	// miniLeaguePartition (fresh stats). Only fall to overall when no criterion helps.
	criteria := []func(StandingRowFull) int{
		func(r StandingRowFull) int { return mini[r.PairID].wins * 3 },
		func(r StandingRowFull) int { s := mini[r.PairID]; return s.setsWon - s.setsLost },
		func(r StandingRowFull) int { s := mini[r.PairID]; return s.gamesWon - s.gamesLost },
	}

	for _, score := range criteria {
		partitioned := partitionBy(group, score, func(sub []StandingRowFull) {
			miniLeaguePartition(sub, matches)
		})
		if partitioned {
			return
		}
	}

	// All mini criteria exhausted with no separation — fall to overall.
	sort.SliceStable(group, func(i, j int) bool {
		return lessByOverallThenName(group[i], group[j])
	})
}

// partitionBy sorts group descending by score. If the criterion separates at
// least one pair (not all equal), it calls resolve on each equal-score
// sub-group of size ≥2 and returns true. If all scores are equal (no
// separation), it returns false without calling resolve — the caller must
// try the next criterion.
func partitionBy(group []StandingRowFull, score func(StandingRowFull) int, resolve func([]StandingRowFull)) bool {
	sort.SliceStable(group, func(i, j int) bool {
		return score(group[i]) > score(group[j])
	})

	if score(group[0]) == score(group[len(group)-1]) {
		return false // all tied on this criterion
	}

	for start := 0; start < len(group); {
		end := start + 1
		for end < len(group) && score(group[end]) == score(group[start]) {
			end++
		}
		sub := group[start:end]
		if len(sub) >= 2 {
			resolve(sub)
		}
		start = end
	}
	return true
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

// applyScheduleStrengthAdjustment computes the SOS-based adjustment for each
// row in a leveled league (recipe §4) and sets Adjustment and Score.
//
//	SOS(t)      = mean win_rate of t's played opponents
//	Adjustment  = 3 × Played × 1.5 × (SOS − 0.5), rounded to 0.1
//	Score       = Points + Adjustment
func applyScheduleStrengthAdjustment(rows []StandingRowFull, stats map[string]*pairStats, matches []*core.Record) {
	// win_rate per pair.
	winRate := make(map[string]float64, len(rows))
	for _, r := range rows {
		s := stats[r.PairID]
		played := s.wins + s.losses
		if played > 0 {
			winRate[r.PairID] = float64(s.wins) / float64(played)
		}
	}

	// Opponents per pair from the final match records.
	opponents := make(map[string][]string, len(rows))
	for _, m := range matches {
		p1, p2 := m.GetString("pair1"), m.GetString("pair2")
		opponents[p1] = append(opponents[p1], p2)
		opponents[p2] = append(opponents[p2], p1)
	}

	for i := range rows {
		r := &rows[i]
		opps := opponents[r.PairID]
		if len(opps) == 0 || r.Played == 0 {
			r.Score = float64(r.Points)
			continue
		}
		var sosSum float64
		for _, opp := range opps {
			sosSum += winRate[opp]
		}
		sos := sosSum / float64(len(opps))
		adj := 3.0 * float64(r.Played) * 1.5 * (sos - 0.5)
		r.Adjustment = math.Round(adj*10) / 10
		r.Score = float64(r.Points) + r.Adjustment
	}
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

func pairIDsOf(group []StandingRowFull) []string {
	ids := make([]string, len(group))
	for i, r := range group {
		ids[i] = r.PairID
	}
	return ids
}
