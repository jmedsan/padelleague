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

// madrid is the league's fixed timezone for event times. Northflank's system
// clock runs UTC, so time.Local cannot be used to mean "Spain time."
var madrid = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		return time.UTC
	}
	return loc
}()

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

	start := time.Date(t.Year(), t.Month(), t.Day(), hour, min, 0, 0, madrid)
	end := start.Add(2 * time.Hour)

	const icalFmt = "20060102T150405"
	return start.Format(icalFmt), end.Format(icalFmt)
}

// icsEscape escapes text-value special characters per RFC 5545 §3.3.11.
func icsEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, ";", `\;`)
	s = strings.ReplaceAll(s, ",", `\,`)
	return s
}

type vEvent struct {
	UID         string
	DTStamp     string
	DTStart     string
	DTEnd       string
	Summary     string
	Location    string
	Description string
}

func buildVEvent(ev vEvent) string {
	var b strings.Builder
	b.WriteString("BEGIN:VEVENT\r\n")
	b.WriteString("UID:" + ev.UID + "\r\n")
	b.WriteString("DTSTAMP:" + ev.DTStamp + "\r\n")
	b.WriteString("DTSTART;TZID=Europe/Madrid:" + ev.DTStart + "\r\n")
	b.WriteString("DTEND;TZID=Europe/Madrid:" + ev.DTEnd + "\r\n")
	b.WriteString("SUMMARY:" + icsEscape(ev.Summary) + "\r\n")
	if ev.Location != "" {
		b.WriteString("LOCATION:" + icsEscape(ev.Location) + "\r\n")
	}
	if ev.Description != "" {
		b.WriteString("DESCRIPTION:" + icsEscape(ev.Description) + "\r\n")
	}
	b.WriteString("END:VEVENT\r\n")
	return b.String()
}

func wrapVCalendar(events string) string {
	return "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//PadelLeague//ES\r\n" +
		"BEGIN:VTIMEZONE\r\nTZID:Europe/Madrid\r\n" +
		"BEGIN:STANDARD\r\nDTSTART:19701025T030000\r\nRRULE:FREQ=YEARLY;BYDAY=-1SU;BYMONTH=10\r\nTZOFFSETFROM:+0200\r\nTZOFFSETTO:+0100\r\nTZNAME:CET\r\nEND:STANDARD\r\n" +
		"BEGIN:DAYLIGHT\r\nDTSTART:19700329T020000\r\nRRULE:FREQ=YEARLY;BYDAY=-1SU;BYMONTH=3\r\nTZOFFSETFROM:+0100\r\nTZOFFSETTO:+0200\r\nTZNAME:CEST\r\nEND:DAYLIGHT\r\n" +
		"END:VTIMEZONE\r\n" +
		events + "END:VCALENDAR\r\n"
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
	location := h.venueLocation(match.GetString("club"))

	description := fmt.Sprintf("Jornada %d", int(match.GetFloat("round_number")))
	if cid := match.GetString("competition"); cid != "" {
		comp, _ := h.app.FindRecordById("competitions", cid)
		if comp != nil {
			description += " — " + comp.GetString("name")
		}
	}

	event := buildVEvent(vEvent{
		UID:         match.Id + "@padelleague",
		DTStamp:     time.Now().UTC().Format("20060102T150405Z"),
		DTStart:     dtStart,
		DTEnd:       dtEnd,
		Summary:     summary,
		Location:    location,
		Description: description,
	})
	ics := wrapVCalendar(event)

	e.Response.Header().Set("Content-Type", "text/calendar")
	e.Response.Header().Set("Content-Disposition", `attachment; filename="partido.ics"`)
	return e.String(http.StatusOK, ics)
}

// Competition generates an iCal feed with all matches for a competition.
func (h *ICalHandler) Competition(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	comp, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return e.String(http.StatusNotFound, "Competición no encontrada")
	}

	compPairIDs := h.playerCompPairIDs(e.Auth.Id, comp)
	if len(compPairIDs) == 0 {
		return e.String(http.StatusOK, "No tienes parejas en esta competición")
	}

	allMatches, _ := h.app.FindRecordsByFilter("matches",
		"competition = {:cid}",
		"round_number", 0, 0,
		map[string]any{"cid": id})

	datedMatches, pairNames := h.filterDatedMatches(allMatches, compPairIDs)

	var events strings.Builder
	for _, m := range datedMatches {
		dtStart, dtEnd := formatICalDate(m.GetString("date"), m.GetString("time"))
		if dtStart == "" {
			continue
		}

		summary := pairNames[m.GetString("pair1")] + " vs " + pairNames[m.GetString("pair2")]
		location := h.venueLocation(m.GetString("club"))

		description := fmt.Sprintf("Jornada %d", int(m.GetFloat("round_number")))

		events.WriteString(buildVEvent(vEvent{
			UID:         m.Id + "@padelleague",
			DTStamp:     time.Now().UTC().Format("20060102T150405Z"),
			DTStart:     dtStart,
			DTEnd:       dtEnd,
			Summary:     summary,
			Location:    location,
			Description: description,
		}))
	}

	ics := wrapVCalendar(events.String())

	filename := fmt.Sprintf("%s.ics", comp.GetString("name"))
	e.Response.Header().Set("Content-Type", "text/calendar")
	e.Response.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	return e.String(http.StatusOK, ics)
}

// venueLocation appends the venue's address to its name, when a venue
// matching club is on record with a non-empty address.
func (h *ICalHandler) venueLocation(club string) string {
	if club == "" {
		return ""
	}
	venues, _ := h.app.FindRecordsByFilter("venues",
		"name = {:n}", "", 1, 0, map[string]any{"n": club})
	if len(venues) == 0 {
		return club
	}
	addr := venues[0].GetString("address")
	if addr == "" {
		return club
	}
	return club + ", " + addr
}

func (h *ICalHandler) filterDatedMatches(allMatches []*core.Record, compPairIDs map[string]struct{}) ([]*core.Record, map[string]string) {
	seen := make(map[string]struct{})
	pairIDSet := make(map[string]struct{})
	var datedMatches []*core.Record
	for _, m := range allMatches {
		p1 := m.GetString("pair1")
		p2 := m.GetString("pair2")
		_, hasP1 := compPairIDs[p1]
		_, hasP2 := compPairIDs[p2]
		if !hasP1 && !hasP2 {
			continue
		}
		// Defensive: allMatches comes from a single FindRecordsByFilter call,
		// which returns distinct records, so dup cannot occur today. The
		// guard protects against a future caller passing duplicates.
		if _, dup := seen[m.Id]; dup || m.GetString("date") == "" {
			continue
		}
		seen[m.Id] = struct{}{}
		pairIDSet[p1] = struct{}{}
		pairIDSet[p2] = struct{}{}
		datedMatches = append(datedMatches, m)
	}

	pairIDSlice := make([]string, 0, len(pairIDSet))
	for pid := range pairIDSet {
		pairIDSlice = append(pairIDSlice, pid)
	}
	pairNames := league.PairNames(h.app, pairIDSlice)
	return datedMatches, pairNames
}

func (h *ICalHandler) playerCompPairIDs(userID string, comp *core.Record) map[string]struct{} {
	pairs, _ := league.PairsForPlayer(h.app, userID)
	if len(pairs) == 0 {
		return nil
	}
	playerPairIDs := make(map[string]struct{}, len(pairs))
	for _, p := range pairs {
		playerPairIDs[p.Id] = struct{}{}
	}
	compPairIDs := make(map[string]struct{})
	for _, pid := range comp.GetStringSlice("pairs") {
		if _, ok := playerPairIDs[pid]; ok {
			compPairIDs[pid] = struct{}{}
		}
	}
	return compPairIDs
}
