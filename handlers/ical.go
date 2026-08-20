package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

type ICalHandler struct {
	app core.App
}

func NewICalHandler(app core.App) *ICalHandler {
	return &ICalHandler{app: app}
}

func formatICalDate(dateStr, timeStr string) (string, string) {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return "", ""
	}

	hour, min := 19, 0
	if timeStr != "" {
		_, _ = fmt.Sscanf(timeStr, "%d:%d", &hour, &min)
	}

	start := time.Date(t.Year(), t.Month(), t.Day(), hour, min, 0, 0, time.Local)
	end := start.Add(2 * time.Hour)

	const icalFmt = "20060102T150405"
	return start.Format(icalFmt), end.Format(icalFmt)
}

func buildVEvent(uid, dtStart, dtEnd, summary, location, description string) string {
	var b strings.Builder
	b.WriteString("BEGIN:VEVENT\r\n")
	b.WriteString("UID:" + uid + "\r\n")
	b.WriteString("DTSTART:" + dtStart + "\r\n")
	b.WriteString("DTEND:" + dtEnd + "\r\n")
	b.WriteString("SUMMARY:" + summary + "\r\n")
	if location != "" {
		b.WriteString("LOCATION:" + location + "\r\n")
	}
	if description != "" {
		b.WriteString("DESCRIPTION:" + description + "\r\n")
	}
	b.WriteString("END:VEVENT\r\n")
	return b.String()
}

func wrapVCalendar(events string) string {
	return "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//PadelLeague//ES\r\n" + events + "END:VCALENDAR\r\n"
}

func (h *ICalHandler) Match(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	partido, err := h.app.FindRecordById("partidos", id)
	if err != nil {
		return e.String(http.StatusNotFound, "Partido no encontrado")
	}

	dateStr := partido.GetString("date")
	if dateStr == "" {
		return e.String(http.StatusBadRequest, "El partido no tiene fecha asignada")
	}

	pairNames, _ := expandPairNames(h.app, []string{
		partido.GetString("pareja1"),
		partido.GetString("pareja2"),
	})

	dtStart, dtEnd := formatICalDate(dateStr, partido.GetString("time"))
	if dtStart == "" {
		return e.String(http.StatusBadRequest, "Formato de fecha inválido")
	}

	summary := pairNames[partido.GetString("pareja1")] + " vs " + pairNames[partido.GetString("pareja2")]
	location := partido.GetString("club")

	description := ""
	jornada, _ := h.app.FindRecordById("jornadas", partido.GetString("jornada"))
	if jornada != nil {
		description = fmt.Sprintf("Jornada %d", int(jornada.GetFloat("round_number")))
		season, _ := h.app.FindRecordById("temporadas", jornada.GetString("temporada"))
		if season != nil {
			description += " — " + season.GetString("name")
		}
	}

	event := buildVEvent(partido.Id+"@padelleague", dtStart, dtEnd, summary, location, description)
	ics := wrapVCalendar(event)

	e.Response.Header().Set("Content-Type", "text/calendar")
	e.Response.Header().Set("Content-Disposition", `attachment; filename="partido.ics"`)
	return e.String(http.StatusOK, ics)
}

func (h *ICalHandler) Season(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	season, err := h.app.FindRecordById("temporadas", id)
	if err != nil {
		return e.String(http.StatusNotFound, "Temporada no encontrada")
	}

	jugador, err := findJugadorByUser(h.app, e.Auth.Id)
	if err != nil {
		return e.String(http.StatusForbidden, "No estás registrado como jugador")
	}

	parejas, _ := h.app.FindRecordsByFilter("parejas",
		"(jugador1 = {:jid} || jugador2 = {:jid}) && temporada = {:sid}",
		"", 0, 0,
		map[string]any{"jid": jugador.Id, "sid": id})

	if len(parejas) == 0 {
		return e.String(http.StatusOK, "No tienes parejas en esta temporada")
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
	pairIDSet := make(map[string]bool)
	var datedPartidos []*core.Record
	for _, p := range allPartidos {
		if seen[p.Id] || p.GetString("date") == "" {
			continue
		}
		seen[p.Id] = true
		pairIDSet[p.GetString("pareja1")] = true
		pairIDSet[p.GetString("pareja2")] = true
		datedPartidos = append(datedPartidos, p)
	}

	pairIDSlice := make([]string, 0, len(pairIDSet))
	for pid := range pairIDSet {
		pairIDSlice = append(pairIDSlice, pid)
	}
	pairNames, _ := expandPairNames(h.app, pairIDSlice)

	var events strings.Builder
	for _, p := range datedPartidos {
		dtStart, dtEnd := formatICalDate(p.GetString("date"), p.GetString("time"))
		if dtStart == "" {
			continue
		}

		summary := pairNames[p.GetString("pareja1")] + " vs " + pairNames[p.GetString("pareja2")]
		location := p.GetString("club")

		description := ""
		jornada, _ := h.app.FindRecordById("jornadas", p.GetString("jornada"))
		if jornada != nil {
			description = fmt.Sprintf("Jornada %d", int(jornada.GetFloat("round_number")))
		}

		events.WriteString(buildVEvent(p.Id+"@padelleague", dtStart, dtEnd, summary, location, description))
	}

	ics := wrapVCalendar(events.String())

	filename := fmt.Sprintf("%s.ics", season.GetString("name"))
	e.Response.Header().Set("Content-Type", "text/calendar")
	e.Response.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	return e.String(http.StatusOK, ics)
}
