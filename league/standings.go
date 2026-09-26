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
	// HasProvisional is true when this row's stats include at least one
	// pending (not yet accepted) result proposal — the standingsTable
	// component shows a tooltip on such rows.
	HasProvisional bool
}

// ComputeStandings calculates ranked standings for a competition, counting a
// pending (submitted but not yet accepted) result proposal right away on
// both sides — the owner's decision so a pair's rank reflects results in
// flight, not just settled ones. A rejected (superseded) or a disputed
// match's proposal does not count until it's resolved. Use
// ComputeFinalStandings for a permanent, settled-only computation (awards).
func (svc *Service) ComputeStandings(competitionID string) ([]StandingRowFull, error) {
	return svc.computeStandings(competitionID, true)
}

// ComputeFinalStandings calculates standings from final matches only, with
// no provisional proposals — for computations that must never change once
// written (season-end awards), as opposed to ComputeStandings' current-state
// view used by every Clasificación display.
func (svc *Service) ComputeFinalStandings(competitionID string) ([]StandingRowFull, error) {
	return svc.computeStandings(competitionID, false)
}

func (svc *Service) computeStandings(competitionID string, includeProvisional bool) ([]StandingRowFull, error) {
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

	var provisional []*core.Record
	if includeProvisional {
		provisional, err = svc.provisionalMatches(competitionID)
		if err != nil {
			return nil, err
		}
	}
	allMatches := append(append([]*core.Record(nil), matches...), provisional...)

	pairStats := tallyMatchStats(pairIDs, allMatches)
	provisionalPairs := make(map[string]bool, len(provisional)*2)
	for _, m := range provisional {
		provisionalPairs[m.GetString("pair1")] = true
		provisionalPairs[m.GetString("pair2")] = true
	}
	penaltyMap, err := PenaltyTotals(svc.app, competitionID)
	if err != nil {
		return nil, err
	}
	rows := buildStandingRows(standingRowInputs{
		pairIDs:          pairIDs,
		pairNames:        pairNames,
		stats:            pairStats,
		penaltyMap:       penaltyMap,
		matches:          allMatches,
		provisionalPairs: provisionalPairs,
	})

	sortStandings(rows, allMatches)

	for i := range rows {
		rows[i].Position = i + 1
	}
	return rows, nil
}

// provisionalMatches returns one synthetic, unsaved match record per
// competition match that has a live pending result_submission proposal with
// a determined winner — a match already final or disputed is excluded (an
// admin-flagged dispute must not count until resolved), and a proposal
// that's an open, undecided set (EvaluateScore.Won == false) doesn't count
// either, matching the real accept path's own "not won yet" rule. The
// synthetic record carries just what tallyMatchStats/pairForm read: pair1,
// pair2, scores, winner, date (copied from the real match, for form
// ordering).
func (svc *Service) provisionalMatches(competitionID string) ([]*core.Record, error) {
	matches, err := svc.app.FindRecordsByFilter("matches",
		"competition = {:cid} && status != 'final' && status != 'disputed'",
		"", 0, 0, map[string]any{"cid": competitionID})
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, nil
	}
	col, err := svc.app.FindCollectionByNameOrId("matches")
	if err != nil {
		return nil, err
	}

	var out []*core.Record
	for _, m := range matches {
		proposals, err := svc.app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'result_submission' && proposal_status = 'pending'",
			"-created", 1, 0, map[string]any{"mid": m.Id})
		if err != nil || len(proposals) == 0 {
			continue
		}
		scores := parseProposalScores(proposals[0].GetString("proposal_data"))
		sc, err := ParseScoreMode(scores, AllowOpenSet)
		if err != nil || !EvaluateScore(sc).Won {
			continue
		}
		winner, err := DetermineWinner(m, scores)
		if err != nil {
			continue
		}
		synth := core.NewRecord(col)
		synth.Set("pair1", m.GetString("pair1"))
		synth.Set("pair2", m.GetString("pair2"))
		synth.Set("scores", scores)
		synth.Set("winner", winner)
		synth.Set("date", m.GetString("date"))
		out = append(out, synth)
	}
	return out, nil
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
	// provisionalPairs marks a pair whose stats include a pending result
	// proposal, nil when ComputeFinalStandings is computing (no provisional
	// data exists to mark).
	provisionalPairs map[string]bool
}

func buildStandingRows(in standingRowInputs) []StandingRowFull {
	rows := make([]StandingRowFull, 0, len(in.pairIDs))
	for _, pid := range in.pairIDs {
		s := in.stats[pid]
		penalty := int(in.penaltyMap[pid])
		rows = append(rows, StandingRowFull{
			PairID:         pid,
			PairName:       in.pairNames[pid],
			Played:         s.wins + s.losses,
			Wins:           s.wins,
			Losses:         s.losses,
			SetsWon:        s.setsWon,
			SetsLost:       s.setsLost,
			GamesWon:       s.gamesWon,
			GamesLost:      s.gamesLost,
			Points:         s.wins*3 - penalty,
			Penalty:        penalty,
			Form:           pairForm(pid, in.matches),
			HasProvisional: in.provisionalPairs[pid],
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
