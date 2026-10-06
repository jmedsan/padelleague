package handlers

import (
	"errors"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
	"padelleague/render"
)

var (
	errOverrideNoChanges  = errors.New("override: no changes")
	errOverrideNoWinner   = errors.New("override: winner cannot be determined")
	errOverridePlayoffOrd = errors.New("override: playoff dates out of bracket order")
)

// overrideForm is what the admin override form submitted; empty means "leave".
type overrideForm struct {
	scores, date, time, venueID, court string
}

// readOverrideForm reads and validates the form. ok is false when it already
// answered the request with an alert.
func readOverrideForm(e *core.RequestEvent) (f overrideForm, ok bool) {
	f = overrideForm{
		date:    e.Request.FormValue("date"),
		time:    e.Request.FormValue("time"),
		venueID: e.Request.FormValue("venue_id"),
		court:   e.Request.FormValue("court_number"),
	}
	if f.time != "" {
		if _, err := time.Parse("15:04", f.time); err != nil {
			_ = alertError(e, "Formato de hora no válido: usa HH:MM")
			return f, false
		}
	}
	if e.Request.FormValue("scores") != "" {
		if f.scores, _ = readScoreForm(e, "", "scores"); f.scores == "" {
			return f, false
		}
	}
	return f, true
}

// AdminOverride lets an admin set the final score, the date, the venue or the
// court, bypassing the normal flow. Pending proposals the change settles are
// rejected by the admin (see writeAsAdmin).
func (h *MatchHandler) AdminOverride(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	if _, err := h.app.FindRecordById("matches", id); err != nil {
		return alertError(e, "Partido no encontrado")
	}
	if !render.AdminView(e) {
		return alertError(e, "Solo administradores")
	}
	form, ok := readOverrideForm(e)
	if !ok {
		return nil
	}

	return h.adminOverrideWrite(e, id, form)
}

// adminOverrideWrite is the ONE path by which an admin sets a result, a date, a
// venue or a court on a match (AdminOverride and the admin's /correct).
func (h *MatchHandler) adminOverrideWrite(e *core.RequestEvent, id string, form overrideForm) error {
	match, change, err := writeAsAdmin(h.app, adminWrite{
		matchID: id, adminID: e.Auth.Id,
		apply: func(txApp core.App, m *core.Record) (adminChange, error) {
			return applyOverride(txApp, m, form)
		},
	})
	switch {
	case errors.Is(err, errOverrideNoChanges):
		return alertWarning(e, "No se detectaron cambios")
	case errors.Is(err, errOverrideNoWinner):
		return alertError(e, "No se pudo determinar ganador")
	case errors.Is(err, errOverridePlayoffOrd):
		return alertError(e, "Las fechas de playoff deben respetar el orden del cuadro (una ronda posterior no puede ir antes que una previa)")
	case err != nil:
		return alertError(e, "Error al guardar")
	}

	compName := league.CompetitionName(h.app, match.GetString("competition"))
	allPlayers := league.MatchPlayersExcluding(h.app, match, "")
	h.notifier.NotifyPlayers(allPlayers, league.NotifAdminCorrection(id, strings.Split(change.detail, "; "), compName))

	return redirectHX(e, "/match/"+id)
}

// applyOverride sets the submitted fields on match and says what changed and
// which pending proposals that settles: a new score finalizes the match, so it
// settles every proposal; a new date or time settles the date proposals.
func applyOverride(app core.App, match *core.Record, f overrideForm) (adminChange, error) {
	oldScores := match.GetString("scores")
	var changes []string
	if f.scores != "" && f.scores != oldScores {
		winner, err := league.DetermineWinner(match, f.scores)
		if err != nil {
			return adminChange{}, errOverrideNoWinner
		}
		match.Set("scores", f.scores)
		match.Set("winner", winner)
		match.Set("carried_sets", "")
		match.Set("status", league.StatusFinal)
		if oldScores == "" {
			changes = append(changes, "Resultado establecido: "+f.scores)
		} else {
			changes = append(changes, "Resultado corregido: "+oldScores+" → "+f.scores)
		}
	}
	scoreChanged := len(changes) > 0
	dateChange := detectFieldChange(match, "date", f.date, "Fecha")
	timeChange := detectFieldChange(match, "time", f.time, "Hora")
	changes = append(changes, dateChange...)
	changes = append(changes, timeChange...)
	changes = append(changes, detectVenueChange(app, match, f.venueID)...)
	changes = append(changes, detectFieldChange(match, "court_number", f.court, "Pista")...)
	if len(changes) == 0 {
		return adminChange{}, errOverrideNoChanges
	}
	if len(dateChange) > 0 || len(timeChange) > 0 {
		league.ClearMatchReminders(app, match.Id)
	}
	if f.date != "" {
		if err := validatePlayoffDates(app, match); err != nil {
			return adminChange{}, errOverridePlayoffOrd
		}
	}

	var decides []string
	switch {
	case scoreChanged:
		decides = []string{proposalResult, proposalScheduling}
	case len(dateChange) > 0 || len(timeChange) > 0:
		decides = []string{proposalScheduling}
	}
	return adminChange{kind: "admin_action", detail: strings.Join(changes, "; "), decides: decides}, nil
}

func validatePlayoffDates(app core.App, match *core.Record) error {
	comp, err := app.FindRecordById("competitions", match.GetString("competition"))
	if err != nil || !league.IsPlayoff(comp) {
		return nil
	}
	allMatches, err := app.FindRecordsByFilter("matches",
		"competition = {:comp}", "", 0, 0,
		map[string]any{"comp": comp.Id})
	if err != nil {
		return nil
	}
	for i, m := range allMatches {
		if m.Id == match.Id {
			allMatches[i] = match
			break
		}
	}
	return league.ValidatePlayoffDates(allMatches)
}

func detectVenueChange(app core.App, match *core.Record, venueID string) []string {
	if venueID == "" {
		return nil
	}
	venueName := venueID
	if v, err := app.FindRecordById("venues", venueID); err == nil {
		venueName = v.GetString("name")
	}
	old := match.GetString("club")
	if venueName == old {
		return nil
	}
	match.Set("club", venueName)
	if old == "" {
		return []string{"Club establecido: " + venueName}
	}
	return []string{"Club cambiado: " + old + " → " + venueName}
}
