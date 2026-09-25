package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
)

type roundDate struct {
	Number int
	Date   string // "YYYY-MM-DD" for the input value
}

type roundGroup struct {
	Number  int
	Key     string // unique key for auto-expand matching
	Title   string // display title
	Matches []MatchCard
	Played  int
	Total   int
	Warning league.Warning
}

func (h *CompetitionHandler) buildRoundDates(comp *core.Record) []roundDate {
	rounds := comp.GetInt("rounds")
	if rounds == 0 {
		return nil
	}
	var dates []roundDate
	for r := 1; r <= rounds; r++ {
		d := roundDate{Number: r}
		if t, ok := league.RoundArrangeDate(comp, r); ok {
			d.Date = t.Format("2006-01-02")
		}
		dates = append(dates, d)
	}
	return dates
}

func (h *CompetitionHandler) buildRoundGroups(comp *core.Record, matches []*core.Record, pairNames map[string]string) []roundGroup {
	noPairs := map[string]struct{}{}
	var allCards []MatchCard
	roundMap := map[int][]int{}
	for _, m := range matches {
		rn := int(m.GetFloat("round_number"))
		roundMap[rn] = append(roundMap[rn], len(allCards))
		allCards = append(allCards, NewMatchRow(m, pairNames, noPairs))
	}
	enrichWithPendingResults(h.app, allCards)
	var rounds []roundGroup
	for rn, idxs := range roundMap {
		ms := make([]MatchCard, len(idxs))
		for i, idx := range idxs {
			ms[i] = allCards[idx]
		}
		key := fmt.Sprintf("round-%d", rn)
		rounds = append(rounds, roundGroup{Number: rn, Key: key, Title: fmt.Sprintf("Jornada %d", rn), Matches: ms})
	}
	sort.Slice(rounds, func(i, j int) bool {
		return rounds[i].Number < rounds[j].Number
	})
	populateRoundProgress(comp, rounds)
	return rounds
}

// buildLeveledRoundGroups builds admin round groups for a leveled league,
// grouping matches into Jornada and monthly "Jugados — <mes año>" groups.
// pairFilter, when non-empty, restricts the list to matches involving that
// pair (mirrors the public competition page's team filter).
func (h *CompetitionHandler) buildLeveledRoundGroups(comp *core.Record, matches []*core.Record, pairNames map[string]string, pairFilter string) []roundGroup {
	noPairs := map[string]struct{}{}
	var allCards []MatchCard
	for _, m := range matches {
		if pairFilter != "" && m.GetString("pair1") != pairFilter && m.GetString("pair2") != pairFilter {
			continue
		}
		allCards = append(allCards, NewMatchRow(m, pairNames, noPairs))
	}
	enrichWithPendingResults(h.app, allCards)

	tz := league.Timezone(h.app)
	groups := leveledGroups(allCards, tz, leveledWindowFor(comp))
	result := make([]roundGroup, len(groups))
	for i, g := range groups {
		played, total := 0, len(g.Matches)
		for _, mc := range g.Matches {
			if mc.Match.GetString("status") == league.StatusFinal {
				played++
			}
		}
		result[i] = roundGroup{Key: g.Key, Title: g.Title, Matches: g.Matches, Played: played, Total: total}
	}
	return result
}

// firstIncompleteRoundGroup returns the key of the first round with an
// unplayed match, or "" if every round is complete (or there are none).
func firstIncompleteRoundGroup(rounds []roundGroup) string {
	for _, r := range rounds {
		if r.Played < r.Total {
			return r.Key
		}
	}
	return ""
}

// populateRoundProgress fills each round's Played/Total/Warning in place.
// Warning is skipped for playoffs, which have admin-fixed dates instead of
// the recommended-arrange-by deadlines this warning is based on.
func populateRoundProgress(comp *core.Record, rounds []roundGroup) {
	isPlayoff := league.IsPlayoff(comp)
	graceDays := comp.GetInt("arrange_grace_days")
	now := time.Now()
	for i := range rounds {
		rounds[i].Total = len(rounds[i].Matches)
		for _, m := range rounds[i].Matches {
			if m.Match.GetString("status") == league.StatusFinal {
				rounds[i].Played++
			}
		}
		if isPlayoff || rounds[i].Played == rounds[i].Total {
			continue
		}
		if deadline, ok := league.RoundArrangeDate(comp, rounds[i].Number); ok {
			rounds[i].Warning = league.WarningLevel(deadline, graceDays, now)
		}
	}
}

func resetWarnLevels(app core.App, compID string) {
	matches, err := app.FindRecordsByFilter("matches",
		"competition = {:comp}", "", 0, 0, map[string]any{"comp": compID})
	if err != nil {
		slog.Error("reset warn levels: list matches", "comp", compID, "err", err)
		return
	}
	for _, m := range matches {
		if m.GetInt("last_warn_level") == 0 {
			continue
		}
		m.Set("last_warn_level", 0)
		if err := app.Save(m); err != nil {
			slog.Error("reset warn level", "match", m.Id, "err", err)
		}
	}
}

func (h *CompetitionHandler) refreshRoundSchedule(comp *core.Record) {
	if league.IsPlayoff(comp) {
		return
	}
	if comp.GetString("round_arrange_dates") != "" {
		return
	}
	rounds := comp.GetInt("rounds")
	if rounds == 0 {
		matches := findRecordsLogged(h.app, "refreshRoundSchedule: find matches", RecordQuery{
			Collection: "matches", Filter: "competition = {:cid}", Params: map[string]any{"cid": comp.Id},
		})
		for _, m := range matches {
			if rn := m.GetInt("round_number"); rn > rounds {
				rounds = rn
			}
		}
		if rounds > 0 {
			comp.Set("rounds", rounds)
		}
	}
	if rounds == 0 {
		return
	}
	start := comp.GetDateTime("start_date").Time()
	end := comp.GetDateTime("end_date").Time()
	comp.Set("round_arrange_dates", league.StoreRoundSchedule(start, end, rounds))
	if err := h.app.Save(comp); err != nil {
		slog.Error("refresh round schedule failed", "competition", comp.Id, "err", err)
	}
}

// refreshLeveledArrangeBy recomputes arrange_by for every non-final leveled
// match using its stored slot, after start_date/end_date changed. Matches
// with slot=0 (round-robin or pre-migration) are left untouched.
func (h *CompetitionHandler) refreshLeveledArrangeBy(comp *core.Record) {
	matches := findRecordsLogged(h.app, "refreshLeveledArrangeBy: find matches", RecordQuery{
		Collection: "matches",
		Filter:     "competition = {:cid} && status != 'final'",
		Params:     map[string]any{"cid": comp.Id},
	})
	for _, m := range matches {
		slot := m.GetInt("slot")
		if slot <= 0 {
			continue
		}
		deadline, ok := league.SlotDeadline(comp, slot)
		if !ok {
			continue
		}
		m.Set("arrange_by", deadline.Format("2006-01-02"))
		if err := h.app.Save(m); err != nil {
			slog.Error("refresh leveled arrange_by failed", "match", m.Id, "err", err)
		}
	}
}

// UpdateRoundDates saves admin-edited per-round arrange-by dates.
func (h *CompetitionHandler) UpdateRoundDates(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	comp, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}

	rounds := comp.GetInt("rounds")
	if rounds == 0 {
		return alertError(e, "No hay jornadas generadas")
	}

	schedule := make(map[int]time.Time, rounds)
	for r := 1; r <= rounds; r++ {
		v := e.Request.FormValue("round_date_" + strconv.Itoa(r))
		if v == "" {
			continue
		}
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			return alertError(e, "Fecha inválida en jornada "+strconv.Itoa(r))
		}
		schedule[r] = t
	}

	b, _ := json.Marshal(schedule)
	comp.Set("round_arrange_dates", string(b))
	if err := h.app.Save(comp); err != nil {
		slog.Error("update round dates failed", "competition", id, "err", err)
		return alertError(e, "Error al guardar las fechas")
	}
	league.LogCompetitionEvent(h.app, league.CompetitionEvent{CompetitionID: id, ActorID: e.Auth.Id, Kind: "settings_changed", Detail: "cambió las fechas de jornada"})

	resetWarnLevels(h.app, id)
	return redirectHX(e, "/admin/competitions/"+id)
}

// RegenerateRoundDates overwrites stored dates from the current start/end/rounds.
func (h *CompetitionHandler) RegenerateRoundDates(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	comp, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}

	rounds := comp.GetInt("rounds")
	start := comp.GetDateTime("start_date").Time()
	end := comp.GetDateTime("end_date").Time()
	comp.Set("round_arrange_dates", league.StoreRoundSchedule(start, end, rounds))
	if err := h.app.Save(comp); err != nil {
		slog.Error("regenerate round dates failed", "competition", id, "err", err)
		return alertError(e, "Error al regenerar las fechas")
	}
	league.LogCompetitionEvent(h.app, league.CompetitionEvent{CompetitionID: id, ActorID: e.Auth.Id, Kind: "settings_changed", Detail: "cambió las fechas de jornada"})

	resetWarnLevels(h.app, id)
	return redirectHX(e, "/admin/competitions/"+id)
}
