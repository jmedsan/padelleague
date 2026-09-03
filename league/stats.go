package league

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

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
	HasLevel         bool
	Reliability      float64
	HasReliability   bool
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
	level, hasLevel := computeLevel(winRate, totals.played, currentStreakWins)
	reliability, hasReliability := svc.computeReliability(pairIDs, totals.played)

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
		Level:            level,
		HasLevel:         hasLevel,
		Reliability:      reliability,
		HasReliability:   hasReliability,
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
// an answered proposal contributes zero to the responsiveness score. A
// same-day response (well under this cap) scores near the maximum.
const reliabilityResponseCap = 72.0

// reliabilityDefaultTimeoutHours is the elapsed time used to score an
// unanswered scheduling proposal, which has no quorum auto-accept of its
// own: it is scored as if fully unresponsive once this much time has
// passed, matching the default quorum_timeout_hours (league/settings.go).
const reliabilityDefaultTimeoutHours = 48.0

// MinMatchesForReliability is the minimum finalized-match count below which
// Reliability has too little data to be meaningful and StatsSummary omits
// it (HasReliability is false).
const MinMatchesForReliability = 3

// computeReliability blends scheduling responsiveness (how quickly this
// pair responded to proposals addressed to them, scoring an ignored
// proposal at 0 once it has been open past its quorum timeout) with a
// show-up rate (finalized matches vs. matches lost by an approved
// walkover) into a 0-100 score. The second return is false when the pair
// has fewer than MinMatchesForReliability finalized matches, in which case
// the score has too little data to display.
func (svc *Service) computeReliability(pairIDs []string, played int) (float64, bool) {
	if played < MinMatchesForReliability {
		return 0, false
	}

	responseScore, hasResponses := svc.responsivenessScore(pairIDs)
	showUpScore := svc.showUpScore(pairIDs, played)

	if !hasResponses {
		return math.Round(showUpScore), true
	}
	return math.Round(responseScore*0.5 + showUpScore*0.5), true
}

// responsivenessScore scores how quickly the given pairs responded to
// scheduling/result proposals addressed to them (proposals authored by the
// opposing side of the match). Each incoming proposal scores 100 at 0h
// response time down to 0 at reliabilityResponseCap hours; an unanswered
// proposal scores 0 once it has been open past its quorum timeout (or
// reliabilityDefaultTimeoutHours for scheduling proposals, which have no
// auto-accept) and is otherwise excluded as still-pending. The second
// return is false when the pair has no incoming proposals to score.
func (svc *Service) responsivenessScore(pairIDs []string) (float64, bool) {
	var total float64
	var count int
	for _, pid := range pairIDs {
		playerIDs := PlayersForPair(svc.app, pid)
		if len(playerIDs) == 0 {
			continue
		}
		matches, err := svc.app.FindRecordsByFilter("matches",
			"pair1 = {:pid} || pair2 = {:pid}", "", 0, 0, map[string]any{"pid": pid})
		if err != nil {
			continue
		}
		for _, m := range matches {
			t, c := svc.incomingProposalScores(m, pid, playerIDs)
			total += t
			count += c
		}
	}
	if count == 0 {
		return 0, false
	}
	return total / float64(count), true
}

// incomingProposalScores sums the response scores for every proposal on
// match m that was addressed to pid (authored by the opposing pair's
// players), returning the running total and how many proposals were
// scored, for the caller to average across every match the pair played.
func (svc *Service) incomingProposalScores(m *core.Record, pid string, playerIDs []string) (float64, int) {
	opponentID := m.GetString("pair1")
	if opponentID == pid {
		opponentID = m.GetString("pair2")
	}
	if opponentID == "" {
		return 0, 0
	}
	timeout := svc.reliabilityTimeout(m)
	proposals, err := svc.app.FindRecordsByFilter("match_messages",
		"match = {:mid} && (type = 'scheduling_proposal' || type = 'result_submission')",
		"", 0, 0, map[string]any{"mid": m.Id})
	if err != nil {
		return 0, 0
	}
	opponentPlayerIDs := PlayersForPair(svc.app, opponentID)

	var total float64
	var count int
	for _, p := range proposals {
		if !slices.Contains(opponentPlayerIDs, p.GetString("author")) {
			continue
		}
		score, scored := svc.proposalResponseScore(p, playerIDs, timeout)
		if !scored {
			continue
		}
		total += score
		count++
	}
	return total, count
}

// proposalResponseScore scores one incoming proposal: 100 at 0h down to 0
// at reliabilityResponseCap for the first response by one of responderIDs,
// 0 for a proposal still unanswered past timeout hours, or unscored (still
// pending, not yet timed out) when neither applies.
func (svc *Service) proposalResponseScore(proposal *core.Record, responderIDs []string, timeout float64) (float64, bool) {
	responses, err := svc.app.FindRecordsByFilter("match_messages",
		"parent = {:pid} && (type = 'scheduling_response' || type = 'result_response')",
		"created", 0, 0, map[string]any{"pid": proposal.Id})
	if err == nil {
		for _, resp := range responses {
			if !slices.Contains(responderIDs, resp.GetString("author")) {
				continue
			}
			hours := resp.GetDateTime("created").Time().Sub(proposal.GetDateTime("created").Time()).Hours()
			if hours < 0 {
				continue
			}
			return 100 * (1 - min(hours/reliabilityResponseCap, 1.0)), true
		}
	}

	elapsed := time.Since(proposal.GetDateTime("created").Time()).Hours()
	if elapsed >= timeout {
		return 0, true
	}
	return 0, false
}

// reliabilityTimeout returns the hours after which an unanswered proposal
// on this match is scored as ignored: the competition's quorum_timeout_hours
// for a result_submission, or reliabilityDefaultTimeoutHours for a
// scheduling_proposal, which has no quorum auto-accept of its own.
func (svc *Service) reliabilityTimeout(m *core.Record) float64 {
	comp, err := svc.app.FindRecordById("competitions", m.GetString("competition"))
	if err != nil {
		return reliabilityDefaultTimeoutHours
	}
	hours := comp.GetFloat("quorum_timeout_hours")
	if hours <= 0 {
		return reliabilityDefaultTimeoutHours
	}
	return hours
}

// showUpScore is the fraction of this pair's finalized matches that were not
// lost by an approved walkover, as a 0-100 score. Called only once played
// is known to be at least MinMatchesForReliability.
func (svc *Service) showUpScore(pairIDs []string, played int) float64 {
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
	return 100 * (1 - min(float64(walkoverLosses)/float64(played), 1.0))
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

// MinMatchesForLevel is the minimum finalized-match count below which Level
// has too little data to be meaningful and StatsSummary omits it
// (HasLevel is false): a new player's level is not shown at all rather
// than defaulting to a misleadingly low number.
const MinMatchesForLevel = 5

// computeLevel derives a relative 1.0-10.0 skill level (relative to this
// league's pool of players, not an absolute rating) from win rate (70%),
// match experience (20%), and current winning-streak momentum (10%). A
// small momentum weight avoids a single loss swinging the level sharply,
// since win rate already rewards winning overall. The second return is
// false below MinMatchesForLevel matches, where the level has too little
// data to be meaningful.
func computeLevel(winRatePct float64, played, currentStreakWins int) (float64, bool) {
	if played < MinMatchesForLevel {
		return 0, false
	}
	winComponent := winRatePct / 100 * 10
	experienceComponent := min(float64(played)/levelExperienceCap, 1.0) * 10
	momentumComponent := min(float64(currentStreakWins)/levelMomentumCap, 1.0) * 10

	level := winComponent*0.7 + experienceComponent*0.2 + momentumComponent*0.1
	return math.Round(max(level, 1.0)*10) / 10, true
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
