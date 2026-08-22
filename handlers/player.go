package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

type PlayerHandler struct {
	app             core.App
	renderPage      func(e *core.RequestEvent, page string, data map[string]any) error
	renderErrorPage func(e *core.RequestEvent, statusCode int, message string) error
}

func NewPlayerHandler(app core.App, renderPage func(e *core.RequestEvent, page string, data map[string]any) error, renderErrorPage func(e *core.RequestEvent, statusCode int, message string) error) *PlayerHandler {
	return &PlayerHandler{app: app, renderPage: renderPage, renderErrorPage: renderErrorPage}
}

type PairInfo struct {
	Pair    *core.Record
	Partner string
}

type PlayerData struct {
	User        *core.Record
	Pairs       []PairInfo
	WinRate     float64
	TotalPlayed int
	SetsWon     int
	SetsLost    int
	GamesWon         int
	GamesLost        int
	Streak           string
	BestStreak       string
	CompetitionStats []CompetitionStat
	Recent           []RecentMatch
}

type CompetitionStat struct {
	Name   string
	Wins   int
	Losses int
	Played int
}

type RecentMatch struct {
	PairName1 string
	PairName2 string
	Score     string
	Won       bool
	Date      string
}

type H2HData struct {
	Pair1Name string
	Pair2Name string
	Pair1ID   string
	Pair2ID   string
	Total     int
	Wins1     int
	Wins2     int
	Recent    []RecentMatch
}

func (h *PlayerHandler) Player(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	user, err := h.app.FindRecordById("users", id)
	if err != nil {
		return h.renderErrorPage(e, http.StatusNotFound, "Jugador no encontrado")
	}

	pairs, _ := findPairsForPlayer(h.app, user.Id)

	var pairInfos []PairInfo
	for _, p := range pairs {
		partnerID := p.GetString("player1")
		if partnerID == user.Id {
			partnerID = p.GetString("player2")
		}
		pairInfos = append(pairInfos, PairInfo{
			Pair:    p,
			Partner: resolvePlayerName(h.app, partnerID),
		})
	}

	totalWins := 0
	totalPlayed := 0
	setsWon := 0
	setsLost := 0
	gamesWon := 0
	gamesLost := 0
	type matchResult struct {
		won    bool
		date   string
		p1     string
		p2     string
		score  string
		compID string
	}
	var allResults []matchResult

	for _, p := range pairs {
		matches, _ := h.app.FindRecordsByFilter("matches",
			"(pair1 = {:pid} || pair2 = {:pid}) && status = 'final'",
			"", 0, 0,
			map[string]any{"pid": p.Id})

		pairIDSet := make(map[string]bool)
		for _, m := range matches {
			pairIDSet[m.GetString("pair1")] = true
			pairIDSet[m.GetString("pair2")] = true
		}
		pairIDSlice := make([]string, 0, len(pairIDSet))
		for pid := range pairIDSet {
			pairIDSlice = append(pairIDSlice, pid)
		}
		pairNames, _ := expandPairNames(h.app, pairIDSlice)

		for _, m := range matches {
			totalPlayed++
			winner := m.GetString("winner")
			won := winner == p.Id
			if won {
				totalWins++
			}

			score := m.GetString("scores")
			if !strings.EqualFold(strings.TrimSpace(score), "WO") {
				s1, s2, g1, g2, err := parseScore(score)
				if err == nil {
					if m.GetString("pair1") == p.Id {
						setsWon += s1
						setsLost += s2
						gamesWon += g1
						gamesLost += g2
					} else {
						setsWon += s2
						setsLost += s1
						gamesWon += g2
						gamesLost += g1
					}
				}
			}

			allResults = append(allResults, matchResult{
				won:    won,
				date:   m.GetString("date"),
				p1:     pairNames[m.GetString("pair1")],
				p2:     pairNames[m.GetString("pair2")],
				score:  score,
				compID: m.GetString("competition"),
			})
		}
	}

	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].date > allResults[j].date
	})

	streak := ""
	if len(allResults) > 0 {
		firstWon := allResults[0].won
		count := 0
		for _, r := range allResults {
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
		streak = fmt.Sprintf("%d%s", count, suffix)
	}

	bestStreak := ""
	if len(allResults) > 0 {
		bestWin, bestLoss := 0, 0
		curWin, curLoss := 0, 0
		for _, r := range allResults {
			if r.won {
				curWin++
				curLoss = 0
				if curWin > bestWin {
					bestWin = curWin
				}
			} else {
				curLoss++
				curWin = 0
				if curLoss > bestLoss {
					bestLoss = curLoss
				}
			}
		}
		if bestWin >= bestLoss {
			bestStreak = fmt.Sprintf("%dV", bestWin)
		} else {
			bestStreak = fmt.Sprintf("%dD", bestLoss)
		}
	}

	compStatsMap := map[string]*CompetitionStat{}
	for _, r := range allResults {
		cs, ok := compStatsMap[r.compID]
		if !ok {
			compName := r.compID
			if comp, err := h.app.FindRecordById("competitions", r.compID); err == nil {
				compName = comp.GetString("name")
			}
			cs = &CompetitionStat{Name: compName}
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

	var winRate float64
	if totalPlayed > 0 {
		winRate = float64(totalWins) / float64(totalPlayed) * 100
	}

	limit := 20
	if len(allResults) < limit {
		limit = len(allResults)
	}
	recent := make([]RecentMatch, 0, limit)
	for _, r := range allResults[:limit] {
		recent = append(recent, RecentMatch{
			PairName1: r.p1,
			PairName2: r.p2,
			Score:     r.score,
			Won:       r.won,
			Date:      r.date,
		})
	}

	data := PlayerData{
		User:             user,
		Pairs:            pairInfos,
		WinRate:          winRate,
		TotalPlayed:      totalPlayed,
		SetsWon:          setsWon,
		SetsLost:         setsLost,
		GamesWon:         gamesWon,
		GamesLost:        gamesLost,
		Streak:           streak,
		BestStreak:       bestStreak,
		CompetitionStats: compStats,
		Recent:           recent,
	}

	return h.renderPage(e, "player.html", map[string]any{
		"Data": data,
	})
}

func (h *PlayerHandler) H2H(e *core.RequestEvent) error {
	p1 := e.Request.URL.Query().Get("p1")
	p2 := e.Request.URL.Query().Get("p2")

	if p1 == "" && p2 == "" {
		return e.Redirect(http.StatusFound, "/")
	}
	if p2 == "" {
		return e.Redirect(http.StatusFound, "/player/"+p1)
	}
	if p1 == "" {
		return e.Redirect(http.StatusFound, "/player/"+p2)
	}

	pairNames, _ := expandPairNames(h.app, []string{p1, p2})

	matches, _ := h.app.FindRecordsByFilter("matches",
		"((pair1 = {:p1} && pair2 = {:p2}) || (pair1 = {:p2} && pair2 = {:p1})) && status = 'final'",
		"", 0, 0,
		map[string]any{"p1": p1, "p2": p2})

	wins1 := 0
	wins2 := 0
	var recent []RecentMatch

	for _, m := range matches {
		winner := m.GetString("winner")
		won := winner == p1
		if winner == p1 {
			wins1++
		} else if winner == p2 {
			wins2++
		}

		if len(recent) < 5 {
			recent = append(recent, RecentMatch{
				PairName1: pairNames[m.GetString("pair1")],
				PairName2: pairNames[m.GetString("pair2")],
				Score:     m.GetString("scores"),
				Won:       won,
				Date:      m.GetString("date"),
			})
		}
	}

	data := H2HData{
		Pair1Name: pairNames[p1],
		Pair2Name: pairNames[p2],
		Pair1ID:   p1,
		Pair2ID:   p2,
		Total:     len(matches),
		Wins1:     wins1,
		Wins2:     wins2,
		Recent:    recent,
	}

	return h.renderPage(e, "h2h.html", map[string]any{
		"Data": data,
	})
}
