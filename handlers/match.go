package handlers

import (
	"net/http"
	"sort"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

type MatchHandler struct {
	app        core.App
	renderPage func(e *core.RequestEvent, page string, data map[string]any) error
}

func NewMatchHandler(app core.App, renderPage func(e *core.RequestEvent, page string, data map[string]any) error) *MatchHandler {
	return &MatchHandler{app: app, renderPage: renderPage}
}

type MatchView struct {
	Partido     *core.Record
	Pareja1Name string
	Pareja2Name string
	JornadaNum  int
	CanSubmit   bool
	CanConfirm  bool
	CanDispute  bool
	CanEdit     bool
	CanWalkover bool
	StatusLabel string
	StatusClass string
}

type PartidoDetailData struct {
	Match        MatchView
	SeasonName   string
	CategoryName string
	SubmittedBy  string
	ConfirmedBy  string
	DisputedBy   string
	DisputeNotes string
}

func statusLabel(status string) string {
	switch status {
	case "pending":
		return "Pendiente"
	case "confirmed":
		return "Enviado — esperando confirmación"
	case "disputed":
		return "En disputa"
	case "final":
		return "Finalizado"
	}
	return status
}

func statusClass(status string) string {
	switch status {
	case "pending":
		return "badge-warning"
	case "confirmed":
		return "badge-info"
	case "disputed":
		return "badge-error"
	case "final":
		return "badge-success"
	}
	return "badge-ghost"
}

func (h *MatchHandler) buildMatchView(partido *core.Record, jugadorID string, pairNames map[string]string) MatchView {
	status := partido.GetString("status")
	submittedBy := partido.GetString("submitted_by")

	team, _ := getJugadorTeam(h.app, jugadorID, partido)
	isSubmitter := false
	if submittedBy != "" {
		submitterTeam, err := getJugadorTeam(h.app, submittedBy, partido)
		if err == nil {
			isSubmitter = (submitterTeam == team)
		}
	}

	jornada, _ := h.app.FindRecordById("jornadas", partido.GetString("jornada"))
	jornadaNum := 0
	if jornada != nil {
		jornadaNum = int(jornada.GetFloat("round_number"))
	}

	return MatchView{
		Partido:     partido,
		Pareja1Name: pairNames[partido.GetString("pareja1")],
		Pareja2Name: pairNames[partido.GetString("pareja2")],
		JornadaNum:  jornadaNum,
		CanSubmit:   status == "pending" && team > 0,
		CanConfirm:  status == "confirmed" && team > 0 && !isSubmitter,
		CanDispute:  status == "confirmed" && team > 0 && !isSubmitter,
		CanEdit:     status == "pending" && team > 0,
		CanWalkover: status == "pending" && team > 0,
		StatusLabel: statusLabel(status),
		StatusClass: statusClass(status),
	}
}

type SeasonMatchGroup struct {
	CategoryName string
	SeasonName   string
	Matches      []MatchView
}

func (h *MatchHandler) MisPartidos(e *core.RequestEvent) error {
	jugador, err := findJugadorByUser(h.app, e.Auth.Id)
	if err != nil {
		return h.renderPage(e, "mis-partidos.html", map[string]any{
			"Seasons": []SeasonMatchGroup{},
		})
	}

	parejas, _ := h.app.FindRecordsByFilter("parejas",
		"jugador1 = {:jid} || jugador2 = {:jid}",
		"", 0, 0,
		map[string]any{"jid": jugador.Id})

	if len(parejas) == 0 {
		return h.renderPage(e, "mis-partidos.html", map[string]any{
			"Seasons": []SeasonMatchGroup{},
		})
	}

	var allPartidos []*core.Record
	for _, p := range parejas {
		partidos, _ := h.app.FindRecordsByFilter("partidos",
			"pareja1 = {:pid} || pareja2 = {:pid}",
			"", 0, 0,
			map[string]any{"pid": p.Id})
		allPartidos = append(allPartidos, partidos...)
	}

	seen := make(map[string]bool)
	var uniquePartidos []*core.Record
	for _, p := range allPartidos {
		if !seen[p.Id] {
			seen[p.Id] = true
			uniquePartidos = append(uniquePartidos, p)
		}
	}

	pairIDSet := make(map[string]bool)
	for _, p := range uniquePartidos {
		pairIDSet[p.GetString("pareja1")] = true
		pairIDSet[p.GetString("pareja2")] = true
	}
	pairIDSlice := make([]string, 0, len(pairIDSet))
	for id := range pairIDSet {
		pairIDSlice = append(pairIDSlice, id)
	}
	pairNames, _ := expandPairNames(h.app, pairIDSlice)

	seasonGroups := map[string]*SeasonMatchGroup{}
	for _, p := range uniquePartidos {
		mv := h.buildMatchView(p, jugador.Id, pairNames)
		jornadaID := p.GetString("jornada")
		jornada, err := h.app.FindRecordById("jornadas", jornadaID)
		if err != nil {
			continue
		}
		seasonID := jornada.GetString("temporada")
		if _, ok := seasonGroups[seasonID]; !ok {
			season, err := h.app.FindRecordById("temporadas", seasonID)
			if err != nil {
				continue
			}
			catID := season.GetString("categoria")
			cat, _ := h.app.FindRecordById("categorias", catID)
			catName := ""
			if cat != nil {
				catName = cat.GetString("name")
			}
			seasonGroups[seasonID] = &SeasonMatchGroup{
				CategoryName: catName,
				SeasonName:   season.GetString("name"),
			}
		}
		seasonGroups[seasonID].Matches = append(seasonGroups[seasonID].Matches, mv)
	}

	seasons := make([]SeasonMatchGroup, 0, len(seasonGroups))
	for _, sg := range seasonGroups {
		sort.Slice(sg.Matches, func(i, j int) bool {
			return sg.Matches[i].JornadaNum < sg.Matches[j].JornadaNum
		})
		seasons = append(seasons, *sg)
	}

	return h.renderPage(e, "mis-partidos.html", map[string]any{
		"Seasons": seasons,
	})
}

func (h *MatchHandler) Partido(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	partido, err := h.app.FindRecordById("partidos", id)
	if err != nil {
		return e.HTML(http.StatusNotFound, `<div class="alert alert-error">Partido no encontrado</div>`)
	}

	jugador, err := findJugadorByUser(h.app, e.Auth.Id)
	if err != nil {
		return e.HTML(http.StatusForbidden, `<div class="alert alert-error">No tienes acceso a este partido</div>`)
	}

	_, err = getJugadorTeam(h.app, jugador.Id, partido)
	if err != nil {
		return e.HTML(http.StatusForbidden, `<div class="alert alert-error">No tienes acceso a este partido</div>`)
	}

	pairNames, _ := expandPairNames(h.app, []string{
		partido.GetString("pareja1"),
		partido.GetString("pareja2"),
	})

	mv := h.buildMatchView(partido, jugador.Id, pairNames)

	// Resolve context names
	seasonName := ""
	categoryName := ""
	jornada, _ := h.app.FindRecordById("jornadas", partido.GetString("jornada"))
	if jornada != nil {
		season, _ := h.app.FindRecordById("temporadas", jornada.GetString("temporada"))
		if season != nil {
			seasonName = season.GetString("name")
			cat, _ := h.app.FindRecordById("categorias", season.GetString("categoria"))
			if cat != nil {
				categoryName = cat.GetString("name")
			}
		}
	}

	submittedByName := ""
	if submittedByID := partido.GetString("submitted_by"); submittedByID != "" {
		submittedByName = resolvePlayerName(h.app, submittedByID)
	}
	confirmedByName := ""
	if confirmedByID := partido.GetString("confirmed_by"); confirmedByID != "" {
		confirmedByName = resolvePlayerName(h.app, confirmedByID)
	}
	disputedByName := ""
	if disputedByID := partido.GetString("disputed_by"); disputedByID != "" {
		disputedByName = resolvePlayerName(h.app, disputedByID)
	}

	detail := PartidoDetailData{
		Match:        mv,
		SeasonName:   seasonName,
		CategoryName: categoryName,
		SubmittedBy:  submittedByName,
		ConfirmedBy:  confirmedByName,
		DisputedBy:   disputedByName,
		DisputeNotes: partido.GetString("dispute_notes"),
	}

	return h.renderPage(e, "partido.html", map[string]any{
		"DisplayName": e.Auth.GetString("display_name"),
		"IsAdmin":     e.Auth.GetString("role") == "admin",
		"Detail":      detail,
	})
}

func (h *MatchHandler) PartidoSubmit(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	partido, err := h.app.FindRecordById("partidos", id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Partido no encontrado</div>`)
	}

	jugador, err := findJugadorByUser(h.app, e.Auth.Id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No estás registrado como jugador</div>`)
	}

	_, err = getJugadorTeam(h.app, jugador.Id, partido)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No eres participante de este partido</div>`)
	}

	if partido.GetString("status") != "pending" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Este partido ya tiene un resultado registrado</div>`)
	}

	scores := e.Request.FormValue("scores")
	if scores == "" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Debes indicar el marcador</div>`)
	}

	partido.Set("scores", scores)
	partido.Set("submitted_by", jugador.Id)
	partido.Set("submitted_at", time.Now().UTC().Format(time.RFC3339))
	partido.Set("status", "confirmed")

	if date := e.Request.FormValue("date"); date != "" {
		partido.Set("date", date)
	}
	if t := e.Request.FormValue("time"); t != "" {
		partido.Set("time", t)
	}
	if club := e.Request.FormValue("club"); club != "" {
		partido.Set("club", club)
	}
	if court := e.Request.FormValue("court_number"); court != "" {
		partido.Set("court_number", court)
	}

	if err := h.app.Save(partido); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al guardar el resultado</div>`)
	}

	e.Response.Header().Set("HX-Redirect", "/mis-partidos")
	return e.NoContent(http.StatusNoContent)
}

func (h *MatchHandler) PartidoEdit(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	partido, err := h.app.FindRecordById("partidos", id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Partido no encontrado</div>`)
	}

	jugador, err := findJugadorByUser(h.app, e.Auth.Id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No estás registrado como jugador</div>`)
	}

	_, err = getJugadorTeam(h.app, jugador.Id, partido)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No eres participante de este partido</div>`)
	}

	if partido.GetString("status") != "pending" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Solo se pueden editar partidos pendientes</div>`)
	}

	if date := e.Request.FormValue("date"); date != "" {
		partido.Set("date", date)
	}
	if t := e.Request.FormValue("time"); t != "" {
		partido.Set("time", t)
	}
	if club := e.Request.FormValue("club"); club != "" {
		partido.Set("club", club)
	}
	if court := e.Request.FormValue("court_number"); court != "" {
		partido.Set("court_number", court)
	}

	if err := h.app.Save(partido); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al guardar los cambios</div>`)
	}

	e.Response.Header().Set("HX-Redirect", "/partido/"+id)
	return e.NoContent(http.StatusNoContent)
}
