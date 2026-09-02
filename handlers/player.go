package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
)

// PlayerHandler serves player profile and head-to-head comparison pages.
type PlayerHandler struct {
	app             core.App
	renderPage      RenderFunc
	renderErrorPage RenderErrorFunc
}

// NewPlayerHandler creates a PlayerHandler with the given dependencies.
func NewPlayerHandler(app core.App, renderPage RenderFunc, renderErrorPage RenderErrorFunc) *PlayerHandler {
	return &PlayerHandler{app: app, renderPage: renderPage, renderErrorPage: renderErrorPage}
}

// PairInfo holds a pair record with the partner's display name.
type PairInfo struct {
	Pair      *core.Record
	Partner   string
	PartnerID string
}

// PlayerData bundles all statistics for a player's profile page.
type PlayerData struct {
	User             *core.Record
	Pairs            []PairInfo
	WinRate          float64
	TotalPlayed      int
	SetsWon          int
	SetsLost         int
	GamesWon         int
	GamesLost        int
	Streak           string
	BestStreak       string
	CompetitionStats []CompetitionStat
	Recent           []RecentMatch
}

// CompetitionStat holds win/loss totals for one competition on a player or pair profile.
type CompetitionStat struct {
	CompID   string
	CompName string
	Position int
	Wins     int
	Losses   int
	Played   int
}

// RecentMatch holds a finalized match for the player's recent-results list.
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

// Player renders the player profile page with stats and recent matches.
func (h *PlayerHandler) Player(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	user, err := h.app.FindRecordById("users", id)
	if err != nil {
		return h.renderErrorPage(e, http.StatusNotFound, "Jugador no encontrado")
	}

	pairs, _ := league.PairsForPlayer(h.app, user.Id)

	var pairInfos []PairInfo
	for _, p := range pairs {
		partnerID := p.GetString("player1")
		if partnerID == user.Id {
			partnerID = p.GetString("player2")
		}
		pairInfos = append(pairInfos, PairInfo{
			Pair:      p,
			Partner:   league.PlayerName(h.app, partnerID),
			PartnerID: partnerID,
		})
	}

	data := h.buildPlayerStats(user, pairs, pairInfos)

	return h.renderPage(e, "player.html", map[string]any{
		"PageTitle": user.GetString("display_name"),
		"Data":      data,
		"Mode":      PlayerFull,
	})
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
	compID  string
}

type playerTotals struct {
	wins, played                           int
	setsWon, setsLost, gamesWon, gamesLost int
}

func tallyScore(t *playerTotals, score string, isPair1 bool) {
	if strings.EqualFold(strings.TrimSpace(score), "WO") {
		return
	}
	sc, err := league.ParseScore(score)
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

func (h *PlayerHandler) buildPlayerStats(user *core.Record, pairs []*core.Record, pairInfos []PairInfo) PlayerData {
	var totals playerTotals
	var allResults []matchResult
	seen := map[string]bool{}

	for _, p := range pairs {
		results := pairMatchResults(h.app, p.Id)
		for _, r := range results {
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

	return PlayerData{
		User:             user,
		Pairs:            pairInfos,
		WinRate:          winRate,
		TotalPlayed:      totals.played,
		SetsWon:          totals.setsWon,
		SetsLost:         totals.setsLost,
		GamesWon:         totals.gamesWon,
		GamesLost:        totals.gamesLost,
		Streak:           computeCurrentStreak(allResults),
		BestStreak:       computeBestStreak(allResults),
		CompetitionStats: computeCompetitionStats(h.app, allResults),
		Recent:           buildRecentMatches(allResults, 20),
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
	pairNames := league.PairNames(app, pairIDSlice)

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
			compID:  m.GetString("competition"),
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

func computeCompetitionStats(app core.App, results []matchResult) []CompetitionStat {
	compStatsMap := map[string]*CompetitionStat{}
	for _, r := range results {
		cs, ok := compStatsMap[r.compID]
		if !ok {
			compName := r.compID
			if comp, err := app.FindRecordById("competitions", r.compID); err == nil {
				compName = comp.GetString("name")
			}
			cs = &CompetitionStat{CompName: compName, CompID: r.compID}
			compStatsMap[r.compID] = cs
		}
		cs.Played++
		if r.won {
			cs.Wins++
		} else {
			cs.Losses++
		}
	}
	compStats := make([]CompetitionStat, 0, len(compStatsMap))
	for _, cs := range compStatsMap {
		compStats = append(compStats, *cs)
	}
	// Sort by name: compStatsMap is a map, and Go randomises map iteration
	// order, so without this the profile table reordered on every page load.
	// Do not remove this as redundant — it is the fix for that bug.
	sort.Slice(compStats, func(i, j int) bool {
		return compStats[i].CompName < compStats[j].CompName
	})
	return compStats
}
