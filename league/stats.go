package league

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// CompetitionStat holds win/loss totals for one competition on a player or pair profile.
type CompetitionStat struct {
	CompID   string
	CompName string
	Position int
	Wins     int
	Losses   int
	Played   int
}

// RecentMatch holds a finalized match for a player/pair's recent-results list.
type RecentMatch struct {
	MatchID    string
	Pair1ID    string
	Pair2ID    string
	PairName1  string
	PairName2  string
	OpponentID string
	Opponent   string
	Score      string
	Won        bool
	Date       string
}

// StatsSummary bundles all win/loss/streak/competition/history statistics
// shared by the player and pair profile pages.
type StatsSummary struct {
	WinRate          float64
	TotalPlayed      int
	Wins             int
	Losses           int
	SetsWon          int
	SetsLost         int
	GamesWon         int
	GamesLost        int
	Streak           string
	BestStreak       string
	Level            float64
	Reliability      float64
	CompetitionStats []CompetitionStat
	Recent           []RecentMatch
}

type matchResult struct {
	matchID string
	won     bool
	isPair1 bool
	date    string
	p1id    string
	p2id    string
	p1      string
	p2      string
	score   string
}

type playerTotals struct {
	wins, played                           int
	setsWon, setsLost, gamesWon, gamesLost int
}

// Summarize builds a StatsSummary for the given pair IDs: a pair page passes
// its own single ID; a player page passes every pair it belongs to, and
// results are unioned with matches deduplicated (a player on both sides of a
// match, via two different pairs, counts it once).
func (svc *Service) Summarize(pairIDs []string) StatsSummary {
	var totals playerTotals
	var allResults []matchResult
	seen := map[string]bool{}

	for _, pid := range pairIDs {
		for _, r := range pairMatchResults(svc.app, pid) {
			if seen[r.matchID] {
				continue
			}
			seen[r.matchID] = true
			totals.played++
			if r.won {
				totals.wins++
			}
			tallyScore(&totals, r.score, r.isPair1)
			allResults = append(allResults, r)
		}
	}

	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].date > allResults[j].date
	})

	var winRate float64
	if totals.played > 0 {
		winRate = float64(totals.wins) / float64(totals.played) * 100
	}

	currentStreakWins := currentStreakWinCount(allResults)

	return StatsSummary{
		WinRate:          winRate,
		TotalPlayed:      totals.played,
		Wins:             totals.wins,
		Losses:           totals.played - totals.wins,
		SetsWon:          totals.setsWon,
		SetsLost:         totals.setsLost,
		GamesWon:         totals.gamesWon,
		GamesLost:        totals.gamesLost,
		Streak:           computeCurrentStreak(allResults),
		BestStreak:       computeBestStreak(allResults),
		Level:            computeLevel(winRate, totals.played, currentStreakWins),
		Reliability:      svc.computeReliability(pairIDs, totals.played),
		CompetitionStats: svc.competitionStatsForPairs(pairIDs),
		Recent:           buildRecentMatches(allResults, 20),
	}
}

// competitionStatsForPairs returns one CompetitionStat per competition any of
// the given pairs is enrolled in, derived from ComputeStandings (not a
// separate match count) so the numbers always match the standings table.
// Position is set for leagues and left at zero (rendered as "-") for
// playoffs, matching pairCompStat's prior behavior.
func (svc *Service) competitionStatsForPairs(pairIDs []string) []CompetitionStat {
	statsByComp := map[string]CompetitionStat{}
	for _, pid := range pairIDs {
		comps, _ := svc.app.FindRecordsByFilter("competitions",
			"pairs ~ {:pid}", "", 0, 0,
			map[string]any{"pid": pid})
		for _, c := range comps {
			if _, ok := statsByComp[c.Id]; ok {
				continue
			}
			statsByComp[c.Id] = svc.pairCompStat(c, pid)
		}
	}
	out := make([]CompetitionStat, 0, len(statsByComp))
	for _, cs := range statsByComp {
		out = append(out, cs)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CompName < out[j].CompName
	})
	return out
}

// pairCompStat computes one pair's row for a competition from ComputeStandings.
func (svc *Service) pairCompStat(c *core.Record, pairID string) CompetitionStat {
	cs := CompetitionStat{
		CompID:   c.Id,
		CompName: c.GetString("name"),
	}
	rows, err := svc.ComputeStandings(c.Id)
	if err != nil {
		return cs
	}
	for _, r := range rows {
		if r.PairID == pairID {
			if !IsPlayoff(c) {
				cs.Position = r.Position
			}
			cs.Wins = r.Wins
			cs.Losses = r.Losses
			cs.Played = r.Played
			break
		}
	}
	return cs
}

// reliabilityResponseCap is the response time, in hours, at or beyond which
// a scheduling/result response contributes zero to the responsiveness score.
// A same-day response (well under this cap) scores near the maximum.
const reliabilityResponseCap = 72.0

// computeReliability blends scheduling responsiveness (how quickly this
// pair's players respond to proposals) with a show-up rate (finalized
// matches vs. matches lost by an approved walkover) into a 0-100 score. A
// pair with no response history and no walkovers gets a neutral 100 rather
// than an artificially low score from lack of data.
func (svc *Service) computeReliability(pairIDs []string, played int) float64 {
	authorIDs := map[string]struct{}{}
	for _, pid := range pairIDs {
		for _, uid := range PlayersForPair(svc.app, pid) {
			authorIDs[uid] = struct{}{}
		}
	}

	responseScore, hasResponses := svc.responsivenessScore(authorIDs)
	showUpScore, hasMatches := svc.showUpScore(pairIDs, played)

	switch {
	case hasResponses && hasMatches:
		return math.Round(responseScore*0.5 + showUpScore*0.5)
	case hasMatches:
		return math.Round(showUpScore)
	case hasResponses:
		return math.Round(responseScore)
	default:
		return 100
	}
}

// responsivenessScore averages how quickly the given users responded to
// scheduling/result proposals (scheduling_response, result_response
// messages with a parent), scored 100 at 0h down to 0 at
// reliabilityResponseCap hours or more. The second return is false when the
// authors have no response history to score.
func (svc *Service) responsivenessScore(authorIDs map[string]struct{}) (float64, bool) {
	if len(authorIDs) == 0 {
		return 0, false
	}
	var authorFilter strings.Builder
	params := map[string]any{}
	i := 0
	for id := range authorIDs {
		if i > 0 {
			authorFilter.WriteString(" || ")
		}
		key := fmt.Sprintf("author%d", i)
		authorFilter.WriteString("author = {:" + key + "}")
		params[key] = id
		i++
	}
	responses, err := svc.app.FindRecordsByFilter("match_messages",
		"("+authorFilter.String()+") && (type = 'scheduling_response' || type = 'result_response') && parent != ''",
		"", 0, 0, params)
	if err != nil || len(responses) == 0 {
		return 0, false
	}

	var total float64
	var count int
	for _, resp := range responses {
		parent, err := svc.app.FindRecordById("match_messages", resp.GetString("parent"))
		if err != nil {
			continue
		}
		hours := resp.GetDateTime("created").Time().Sub(parent.GetDateTime("created").Time()).Hours()
		if hours < 0 {
			continue
		}
		total += 100 * (1 - min(hours/reliabilityResponseCap, 1.0))
		count++
	}
	if count == 0 {
		return 0, false
	}
	return total / float64(count), true
}

// showUpScore is the fraction of this pair's finalized matches that were not
// lost by an approved walkover, as a 0-100 score. The second return is false
// when the pair has no finalized matches to score.
func (svc *Service) showUpScore(pairIDs []string, played int) (float64, bool) {
	if played == 0 {
		return 0, false
	}
	var walkoverLosses int
	for _, pid := range pairIDs {
		losses, err := svc.app.FindRecordsByFilter("matches",
			"status = 'final' && review_type = 'walkover' && winner != {:pid} && (pair1 = {:pid} || pair2 = {:pid})",
			"", 0, 0, map[string]any{"pid": pid})
		if err != nil {
			continue
		}
		walkoverLosses += len(losses)
	}
	return 100 * (1 - min(float64(walkoverLosses)/float64(played), 1.0)), true
}

func tallyScore(t *playerTotals, score string, isPair1 bool) {
	if strings.EqualFold(strings.TrimSpace(score), "WO") {
		return
	}
	sc, err := ParseScore(score)
	if err != nil {
		return
	}
	if isPair1 {
		t.setsWon += sc.Sets1
		t.setsLost += sc.Sets2
		t.gamesWon += sc.Games1
		t.gamesLost += sc.Games2
	} else {
		t.setsWon += sc.Sets2
		t.setsLost += sc.Sets1
		t.gamesWon += sc.Games2
		t.gamesLost += sc.Games1
	}
}

func pairMatchResults(app core.App, pairID string) []matchResult {
	matches, _ := app.FindRecordsByFilter("matches",
		"(pair1 = {:pid} || pair2 = {:pid}) && status = 'final'",
		"", 0, 0,
		map[string]any{"pid": pairID})

	pairIDSet := make(map[string]struct{})
	for _, m := range matches {
		pairIDSet[m.GetString("pair1")] = struct{}{}
		pairIDSet[m.GetString("pair2")] = struct{}{}
	}
	pairIDSlice := make([]string, 0, len(pairIDSet))
	for pid := range pairIDSet {
		pairIDSlice = append(pairIDSlice, pid)
	}
	pairNames := PairNames(app, pairIDSlice)

	var results []matchResult
	for _, m := range matches {
		won := m.GetString("winner") == pairID
		results = append(results, matchResult{
			matchID: m.Id,
			won:     won,
			isPair1: m.GetString("pair1") == pairID,
			date:    m.GetString("date"),
			p1id:    m.GetString("pair1"),
			p2id:    m.GetString("pair2"),
			p1:      pairNames[m.GetString("pair1")],
			p2:      pairNames[m.GetString("pair2")],
			score:   m.GetString("scores"),
		})
	}
	return results
}

func buildRecentMatches(allResults []matchResult, limit int) []RecentMatch {
	if len(allResults) < limit {
		limit = len(allResults)
	}
	recent := make([]RecentMatch, 0, limit)
	for _, r := range allResults[:limit] {
		opponentID, opponent := r.p2id, r.p2
		if !r.isPair1 {
			opponentID, opponent = r.p1id, r.p1
		}
		recent = append(recent, RecentMatch{
			MatchID:    r.matchID,
			Pair1ID:    r.p1id,
			Pair2ID:    r.p2id,
			PairName1:  r.p1,
			PairName2:  r.p2,
			OpponentID: opponentID,
			Opponent:   opponent,
			Score:      r.score,
			Won:        r.won,
			Date:       r.date,
		})
	}
	return recent
}

func computeCurrentStreak(results []matchResult) string {
	if len(results) == 0 {
		return ""
	}
	firstWon := results[0].won
	count := 0
	for _, r := range results {
		if r.won == firstWon {
			count++
		} else {
			break
		}
	}
	suffix := "D"
	if firstWon {
		suffix = "V"
	}
	return fmt.Sprintf("%d%s", count, suffix)
}

// currentStreakWinCount returns the length of the current streak if it is a
// winning streak, or 0 if the player's last result was a loss (or there are
// no results yet).
func currentStreakWinCount(results []matchResult) int {
	count := 0
	for _, r := range results {
		if !r.won {
			break
		}
		count++
	}
	return count
}

// levelExperienceCap is the match count at which the experience component of
// computeLevel reaches its maximum — a new player's level is pulled down
// regardless of win rate until they have played this many matches.
const levelExperienceCap = 30

// levelMomentumCap is the winning-streak length at which the momentum
// component of computeLevel reaches its maximum.
const levelMomentumCap = 8

// computeLevel derives a Playtomic-style 1.0-10.0 skill level from win rate
// (60%), match experience (20%), and current winning-streak momentum (20%).
// A new player with few matches scores low regardless of win rate, since the
// experience component dominates until levelExperienceCap matches are played.
func computeLevel(winRatePct float64, played, currentStreakWins int) float64 {
	if played == 0 {
		return 1.0
	}
	winComponent := winRatePct / 100 * 10
	experienceComponent := min(float64(played)/levelExperienceCap, 1.0) * 10
	momentumComponent := min(float64(currentStreakWins)/levelMomentumCap, 1.0) * 10

	level := winComponent*0.6 + experienceComponent*0.2 + momentumComponent*0.2
	return math.Round(max(level, 1.0)*10) / 10
}

func computeBestStreak(results []matchResult) string {
	if len(results) == 0 {
		return ""
	}
	best, cur := 0, 0
	for _, r := range results {
		if r.won {
			cur++
			if cur > best {
				best = cur
			}
		} else {
			cur = 0
		}
	}
	if best == 0 {
		return "0V"
	}
	return fmt.Sprintf("%dV", best)
}
