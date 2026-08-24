package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
)

// ICalHandler serves iCalendar downloads for matches and competitions.
type ICalHandler struct {
	app core.App
}

// NewICalHandler creates an ICalHandler with the given app.
func NewICalHandler(app core.App) *ICalHandler {
	return &ICalHandler{app: app}
}

func formatICalDate(dateStr, timeStr string) (string, string) {
	if len(dateStr) > 10 {
		dateStr = dateStr[:10]
	}
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

// Match serves an iCalendar file for a single match event.
func (h *ICalHandler) Match(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", id)
	if err != nil {
		return e.String(http.StatusNotFound, "Partido no encontrado")
	}

	dateStr := match.GetString("date")
	if dateStr == "" {
		return e.String(http.StatusBadRequest, "El partido no tiene fecha asignada")
	}

	pairNames := league.PairNames(h.app, []string{
		match.GetString("pair1"),
		match.GetString("pair2"),
	})

	dtStart, dtEnd := formatICalDate(dateStr, match.GetString("time"))
	if dtStart == "" {
		return e.String(http.StatusBadRequest, "Formato de fecha inválido")
	}

	summary := pairNames[match.GetString("pair1")] + " vs " + pairNames[match.GetString("pair2")]
	location := match.GetString("club")

	description := fmt.Sprintf("Jornada %d", int(match.GetFloat("round_number")))
	if cid := match.GetString("competition"); cid != "" {
		comp, _ := h.app.FindRecordById("competitions", cid)
		if comp != nil {
			description += " — " + comp.GetString("name")
		}
	}

	event := buildVEvent(match.Id+"@padelleague", dtStart, dtEnd, summary, location, description)
	ics := wrapVCalendar(event)

	e.Response.Header().Set("Content-Type", "text/calendar")
	e.Response.Header().Set("Content-Disposition", `attachment; filename="partido.ics"`)
	return e.String(http.StatusOK, ics)
}

func (h *ICalHandler) Competition(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	comp, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return e.String(http.StatusNotFound, "Competición no encontrada")
	}

	pairs, _ := league.PairsForPlayer(h.app, e.Auth.Id)
	if len(pairs) == 0 {
		return e.String(http.StatusOK, "No tienes parejas en esta competición")
	}

	playerPairIDs := make(map[string]bool)
	for _, p := range pairs {
		playerPairIDs[p.Id] = true
	}
	compPairIDs := make(map[string]bool)
	for _, pid := range comp.GetStringSlice("pairs") {
		if playerPairIDs[pid] {
			compPairIDs[pid] = true
		}
	}

	if len(compPairIDs) == 0 {
		return e.String(http.StatusOK, "No tienes parejas en esta competición")
	}

	allMatches, _ := h.app.FindRecordsByFilter("matches",
		"competition = {:cid}",
		"", 0, 0,
		map[string]any{"cid": id})

	seen := make(map[string]bool)
	pairIDSet := make(map[string]bool)
	var datedMatches []*core.Record
	for _, m := range allMatches {
		p1 := m.GetString("pair1")
		p2 := m.GetString("pair2")
		if !compPairIDs[p1] && !compPairIDs[p2] {
			continue
		}
		if seen[m.Id] || m.GetString("date") == "" {
			continue
		}
		seen[m.Id] = true
		pairIDSet[p1] = true
		pairIDSet[p2] = true
		datedMatches = append(datedMatches, m)
	}

	pairIDSlice := make([]string, 0, len(pairIDSet))
	for pid := range pairIDSet {
		pairIDSlice = append(pairIDSlice, pid)
	}
	pairNames := league.PairNames(h.app, pairIDSlice)

	var events strings.Builder
	for _, m := range datedMatches {
		dtStart, dtEnd := formatICalDate(m.GetString("date"), m.GetString("time"))
		if dtStart == "" {
			continue
		}

		summary := pairNames[m.GetString("pair1")] + " vs " + pairNames[m.GetString("pair2")]
		location := m.GetString("club")

		description := fmt.Sprintf("Jornada %d", int(m.GetFloat("round_number")))

		events.WriteString(buildVEvent(m.Id+"@padelleague", dtStart, dtEnd, summary, location, description))
	}

	ics := wrapVCalendar(events.String())

	filename := fmt.Sprintf("%s.ics", comp.GetString("name"))
	e.Response.Header().Set("Content-Type", "text/calendar")
	e.Response.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	return e.String(http.StatusOK, ics)
}
