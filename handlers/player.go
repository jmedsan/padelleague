package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

type PlayerHandler struct {
	app        core.App
	renderPage func(e *core.RequestEvent, page string, data map[string]any) error
}

func NewPlayerHandler(app core.App, renderPage func(e *core.RequestEvent, page string, data map[string]any) error) *PlayerHandler {
	return &PlayerHandler{app: app, renderPage: renderPage}
}

type PairWithELO struct {
	Pair    *core.Record
	Partner string
	Season  string
	ELO     int
}

type PlayerData struct {
	Player      *core.Record
	User        *core.Record
	Pairs       []PairWithELO
	WinRate     float64
	TotalPlayed int
	SetsWon     int
	SetsLost    int
	Streak      string
	Recent      []RecentMatch
	ELOHistory  []*core.Record
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
	jugador, err := h.app.FindRecordById("jugadores", id)
	if err != nil {
		return e.HTML(http.StatusNotFound, `<div class="alert alert-error">Jugador no encontrado</div>`)
	}

	user, err := h.app.FindRecordById("users", jugador.GetString("user"))
	if err != nil {
		return e.HTML(http.StatusNotFound, `<div class="alert alert-error">Usuario no encontrado</div>`)
	}

	parejas, _ := h.app.FindRecordsByFilter("parejas",
		"jugador1 = {:jid} || jugador2 = {:jid}",
		"", 0, 0,
		map[string]any{"jid": jugador.Id})

	var pairsWithELO []PairWithELO
	for _, p := range parejas {
		partnerID := p.GetString("jugador1")
		if partnerID == jugador.Id {
			partnerID = p.GetString("jugador2")
		}
		partnerName := resolvePlayerName(h.app, partnerID)

		seasonName := ""
		season, err := h.app.FindRecordById("temporadas", p.GetString("temporada"))
		if err == nil {
			seasonName = season.GetString("name")
		}

		elo := int(p.GetFloat("elo"))
		if elo == 0 {
			elo = 1500
		}

		pairsWithELO = append(pairsWithELO, PairWithELO{
			Pair:    p,
			Partner: partnerName,
			Season:  seasonName,
			ELO:     elo,
		})
	}

	totalWins := 0
	totalPlayed := 0
	setsWon := 0
	setsLost := 0
	type matchResult struct {
		won   bool
		date  string
		p1    string
		p2    string
		score string
	}
	var allResults []matchResult

	for _, p := range parejas {
		partidos, _ := h.app.FindRecordsByFilter("partidos",
			"(pareja1 = {:pid} || pareja2 = {:pid}) && status = 'final'",
			"-created", 0, 0,
			map[string]any{"pid": p.Id})

		pairIDs := make(map[string]bool)
		for _, m := range partidos {
			pairIDs[m.GetString("pareja1")] = true
			pairIDs[m.GetString("pareja2")] = true
		}
		pairIDSlice := make([]string, 0, len(pairIDs))
		for id := range pairIDs {
			pairIDSlice = append(pairIDSlice, id)
		}
		pairNames, _ := expandPairNames(h.app, pairIDSlice)

		for _, m := range partidos {
			totalPlayed++
			winner := m.GetString("winner")
			won := winner == p.Id
			if won {
				totalWins++
			}

			score := m.GetString("scores")
			if !strings.EqualFold(strings.TrimSpace(score), "WO") {
				s1, s2, _, _, err := parseScore(score)
				if err == nil {
					if m.GetString("pareja1") == p.Id {
						setsWon += s1
						setsLost += s2
					} else {
						setsWon += s2
						setsLost += s1
					}
				}
			}

			allResults = append(allResults, matchResult{
				won:   won,
				date:  m.GetString("date"),
				p1:    pairNames[m.GetString("pareja1")],
				p2:    pairNames[m.GetString("pareja2")],
				score: score,
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

	var eloHistory []*core.Record
	if len(parejas) > 0 {
		latestPair := parejas[len(parejas)-1]
		eloHistory, _ = h.app.FindRecordsByFilter("elo_history",
			"pareja = {:pid}",
			"-created", 10, 0,
			map[string]any{"pid": latestPair.Id})
	}

	data := PlayerData{
		Player:      jugador,
		User:        user,
		Pairs:       pairsWithELO,
		WinRate:     winRate,
		TotalPlayed: totalPlayed,
		SetsWon:     setsWon,
		SetsLost:    setsLost,
		Streak:      streak,
		Recent:      recent,
		ELOHistory:  eloHistory,
	}

	return h.renderPage(e, "jugador.html", map[string]any{
		"Data": data,
	})
}

func (h *PlayerHandler) H2H(e *core.RequestEvent) error {
	p1 := e.Request.URL.Query().Get("p1")
	p2 := e.Request.URL.Query().Get("p2")

	if p1 == "" || p2 == "" {
		return e.HTML(http.StatusBadRequest, `<div class="alert alert-error">Faltan parámetros p1 y p2</div>`)
	}

	pairNames, _ := expandPairNames(h.app, []string{p1, p2})

	matches, _ := h.app.FindRecordsByFilter("partidos",
		"((pareja1 = {:p1} && pareja2 = {:p2}) || (pareja1 = {:p2} && pareja2 = {:p1})) && status = 'final'",
		"-created", 0, 0,
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
				PairName1: pairNames[m.GetString("pareja1")],
				PairName2: pairNames[m.GetString("pareja2")],
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
