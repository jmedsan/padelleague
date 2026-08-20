package handlers

import (
	"sort"

	"github.com/pocketbase/pocketbase/core"
)

type PublicHandler struct {
	app        core.App
	renderPage func(e *core.RequestEvent, page string, data map[string]any) error
}

func NewPublicHandler(app core.App, renderPage func(e *core.RequestEvent, page string, data map[string]any) error) *PublicHandler {
	return &PublicHandler{app: app, renderPage: renderPage}
}

func (h *PublicHandler) Home(e *core.RequestEvent) error {
	categories, err := h.app.FindAllRecords("categorias")
	if err != nil {
		categories = []*core.Record{}
	}

	type categoryView struct {
		Record          *core.Record
		ActiveSeason    *core.Record
		HasActiveSeason bool
	}

	var catViews []categoryView
	for _, c := range categories {
		seasons, _ := h.app.FindRecordsByFilter("temporadas",
			"categoria = {:cid} && active = true",
			"", 1, 0,
			map[string]any{"cid": c.Id})

		cv := categoryView{Record: c}
		if len(seasons) > 0 {
			cv.ActiveSeason = seasons[0]
			cv.HasActiveSeason = true
		}
		catViews = append(catViews, cv)
	}

	displayName := ""
	if e.Auth != nil {
		displayName = e.Auth.GetString("display_name")
	}

	return h.renderPage(e, "home.html", map[string]any{
		"DisplayName": displayName,
		"IsAdmin":     e.Auth != nil && e.Auth.GetString("role") == "admin",
		"Categories":  catViews,
	})
}

type StandingRow struct {
	Position int
	PairName string
	Wins     int
	Losses   int
	Points   int
}

func (h *PublicHandler) Categoria(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	category, err := h.app.FindRecordById("categorias", id)
	if err != nil {
		return e.Redirect(302, "/")
	}

	seasons, _ := h.app.FindRecordsByFilter("temporadas",
		"categoria = {:cat} && active = true",
		"", 1, 0,
		map[string]any{"cat": id})

	var season *core.Record
	var standings []StandingRow

	if len(seasons) > 0 {
		season = seasons[0]

		rows, _ := h.app.FindRecordsByFilter("clasificacion",
			"temporada = {:sid}",
			"-points", 0, 0,
			map[string]any{"sid": season.Id})

		pairIDs := make([]string, 0, len(rows))
		for _, r := range rows {
			pairIDs = append(pairIDs, r.GetString("pareja"))
		}
		pairNames, _ := expandPairNames(h.app, pairIDs)

		standings = make([]StandingRow, 0, len(rows))
		for _, r := range rows {
			standings = append(standings, StandingRow{
				PairName: pairNames[r.GetString("pareja")],
				Wins:     int(r.GetFloat("wins")),
				Losses:   int(r.GetFloat("losses")),
				Points:   int(r.GetFloat("points")),
			})
		}

		sort.Slice(standings, func(i, j int) bool {
			if standings[i].Points != standings[j].Points {
				return standings[i].Points > standings[j].Points
			}
			return standings[i].Wins > standings[j].Wins
		})

		for i := range standings {
			standings[i].Position = i + 1
		}
	}

	return h.renderPage(e, "categoria.html", map[string]any{
		"DisplayName": e.Auth.GetString("display_name"),
		"IsAdmin":     e.Auth.GetString("role") == "admin",
		"Category":    category,
		"Season":      season,
		"Standings":   standings,
	})
}

type JornadaView struct {
	Jornada  *core.Record
	Partidos []PartidoView
}

type PartidoView struct {
	Partido *core.Record
	Pareja1 string
	Pareja2 string
}

func (h *PublicHandler) Temporada(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	season, err := h.app.FindRecordById("temporadas", id)
	if err != nil {
		return e.Redirect(302, "/")
	}

	category, _ := h.app.FindRecordById("categorias", season.GetString("categoria"))

	jornadas, _ := h.app.FindRecordsByFilter("jornadas",
		"temporada = {:sid}",
		"round_number", 0, 0,
		map[string]any{"sid": id})

	allPairIDs := make(map[string]bool)
	type jornadaPartidos struct {
		jornada  *core.Record
		partidos []*core.Record
	}
	jornadaData := make([]jornadaPartidos, 0, len(jornadas))

	for _, j := range jornadas {
		partidos, _ := h.app.FindRecordsByFilter("partidos",
			"jornada = {:jid}",
			"", 0, 0,
			map[string]any{"jid": j.Id})
		for _, p := range partidos {
			allPairIDs[p.GetString("pareja1")] = true
			allPairIDs[p.GetString("pareja2")] = true
		}
		jornadaData = append(jornadaData, jornadaPartidos{jornada: j, partidos: partidos})
	}

	pairIDSlice := make([]string, 0, len(allPairIDs))
	for pid := range allPairIDs {
		pairIDSlice = append(pairIDSlice, pid)
	}
	pairNames, _ := expandPairNames(h.app, pairIDSlice)

	var leagueRounds, playoffRounds []JornadaView

	for _, jd := range jornadaData {
		var partidoViews []PartidoView
		for _, p := range jd.partidos {
			partidoViews = append(partidoViews, PartidoView{
				Partido: p,
				Pareja1: pairNames[p.GetString("pareja1")],
				Pareja2: pairNames[p.GetString("pareja2")],
			})
		}
		jv := JornadaView{Jornada: jd.jornada, Partidos: partidoViews}
		if jd.jornada.GetBool("is_playoff") {
			playoffRounds = append(playoffRounds, jv)
		} else {
			leagueRounds = append(leagueRounds, jv)
		}
	}

	return h.renderPage(e, "temporada.html", map[string]any{
		"DisplayName":   e.Auth.GetString("display_name"),
		"IsAdmin":       e.Auth.GetString("role") == "admin",
		"Season":        season,
		"Category":      category,
		"LeagueRounds":  leagueRounds,
		"PlayoffRounds": playoffRounds,
		"IsArchived":    !season.GetBool("active"),
	})
}
