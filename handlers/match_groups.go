package handlers

import (
	"fmt"
	"sort"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
)

var spanishMonths = []string{
	"enero", "febrero", "marzo", "abril", "mayo", "junio",
	"julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre",
}

// LeveledGroup is a named group of matches for the leveled-league display.
type LeveledGroup struct {
	Key     string
	Title   string
	Matches []MatchCard
}

type monthKey struct {
	year  int
	month time.Month
}

// schedulingRank orders match statuses the way an admin triages them: no
// proposal yet, then proposed, then confirmed/disputed/final — stuck matches
// (still pending) float to the top of their group.
func schedulingRank(status string) int {
	switch status {
	case league.StatusPending:
		return 0
	case league.StatusScheduled:
		return 1
	case league.StatusConfirmed:
		return 2
	case league.StatusDisputed:
		return 3
	default:
		return 4
	}
}

// pendingSortKey returns the deterministic tiebreak pair name for a match —
// the lower of its two pair names, matching the standings tiebreaker
// convention (pair name as the stable last resort).
func pendingSortKey(mc MatchCard) string {
	if mc.Pair1Name <= mc.Pair2Name {
		return mc.Pair1Name
	}
	return mc.Pair2Name
}

// pendingByArrangeBy sorts non-final matches by scheduling state first
// (unscheduled matches float to the top), then arrange-by deadline, then
// pair name as a stable tiebreaker.
func pendingByArrangeBy(pending []MatchCard) func(i, j int) bool {
	return func(i, j int) bool {
		ri := schedulingRank(pending[i].Match.GetString("status"))
		rj := schedulingRank(pending[j].Match.GetString("status"))
		if ri != rj {
			return ri < rj
		}
		ai := pending[i].Match.GetDateTime("arrange_by").Time()
		aj := pending[j].Match.GetDateTime("arrange_by").Time()
		if ai.IsZero() != aj.IsZero() {
			return aj.IsZero()
		}
		if !ai.Equal(aj) {
			return ai.Before(aj)
		}
		return pendingSortKey(pending[i]) < pendingSortKey(pending[j])
	}
}

func newerMonthFirst(order []monthKey) func(i, j int) bool {
	return func(i, j int) bool {
		if order[i].year != order[j].year {
			return order[i].year > order[j].year
		}
		return order[i].month > order[j].month
	}
}

// leveledWindow holds the competition's play window, used to compute each
// Jornada's date-range title. Zero target or dates means no dates are shown.
type leveledWindow struct {
	start  time.Time
	end    time.Time
	target int
}

// hasDates reports whether w carries a usable competition window.
func (w leveledWindow) hasDates() bool {
	return w.target > 0 && !w.start.IsZero() && !w.end.IsZero()
}

// jornadaRange returns the [lo, hi] date range for Jornada n: the window is
// divided into target equal-length units, n's range is [start+(n-1)*u,
// start+n*u-1day], capped at end for the last Jornada.
func (w leveledWindow) jornadaRange(n int) (lo, hi time.Time) {
	u := w.end.Sub(w.start) / time.Duration(w.target)
	lo = w.start.Add(time.Duration(n-1) * u)
	hi = w.start.Add(time.Duration(n)*u).AddDate(0, 0, -1)
	if n >= w.target || hi.After(w.end) {
		hi = w.end
	}
	return lo, hi
}

// leveledGroups partitions match cards into:
//   - "Jornada N" (N=1..target): every non-final match, grouped by its
//     per-pair ordinal slot, titled with the competition-window date range
//     for that slot (or just "Jornada N" without a window)
//   - "Sin asignar": defensive fallback for slot=0 (pre-migration data)
//   - "Jugados — <mes año>": finalized matches grouped by calendar month of
//     finalized_at in the given timezone, newest month first
func leveledGroups(cards []MatchCard, tz *time.Location, w leveledWindow) []LeveledGroup {
	jornadas := map[int][]MatchCard{}
	unassigned := []MatchCard{}
	byMonth := map[monthKey][]MatchCard{}
	var monthOrder []monthKey
	seenMonth := map[monthKey]bool{}

	for _, mc := range cards {
		if mc.Match.GetString("status") == league.StatusFinal {
			ft := mc.Match.GetDateTime("finalized_at").Time()
			if ft.IsZero() {
				ft = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
			}
			local := ft.In(tz)
			mk := monthKey{year: local.Year(), month: local.Month()}
			byMonth[mk] = append(byMonth[mk], mc)
			if !seenMonth[mk] {
				seenMonth[mk] = true
				monthOrder = append(monthOrder, mk)
			}
			continue
		}

		slot := mc.Match.GetInt("slot")
		if slot <= 0 {
			unassigned = append(unassigned, mc)
			continue
		}
		jornadas[slot] = append(jornadas[slot], mc)
	}

	var groups []LeveledGroup
	groups = append(groups, jornadaGroups(jornadas, w, tz)...)
	if len(unassigned) > 0 {
		sort.SliceStable(unassigned, pendingByArrangeBy(unassigned))
		groups = append(groups, LeveledGroup{Key: "bloque-0", Title: "Sin asignar", Matches: unassigned})
	}
	groups = append(groups, playedGroups(byMonth, monthOrder)...)
	return groups
}

// jornadaGroups builds the "Jornada N" groups (slot 1..target), ascending.
// Empty Jornadas are skipped.
func jornadaGroups(jornadas map[int][]MatchCard, w leveledWindow, tz *time.Location) []LeveledGroup {
	var groups []LeveledGroup
	for s := 1; s <= w.target; s++ {
		ms := jornadas[s]
		if len(ms) == 0 {
			continue
		}
		sort.SliceStable(ms, pendingByArrangeBy(ms))
		title := fmt.Sprintf("Jornada %d", s)
		if w.hasDates() {
			lo, hi := w.jornadaRange(s)
			title = fmt.Sprintf("Jornada %d · %s – %s", s, fmtShortDate(lo, tz), fmtShortDate(hi, tz))
		}
		groups = append(groups, LeveledGroup{Key: fmt.Sprintf("jornada-%d", s), Title: title, Matches: ms})
	}
	return groups
}

// playedGroups builds the "Jugados — <mes año>" groups, newest month first.
func playedGroups(byMonth map[monthKey][]MatchCard, monthOrder []monthKey) []LeveledGroup {
	sort.Slice(monthOrder, newerMonthFirst(monthOrder))
	groups := make([]LeveledGroup, 0, len(monthOrder))
	for _, mk := range monthOrder {
		title := fmt.Sprintf("Jugados — %s %d", spanishMonths[mk.month-1], mk.year)
		key := fmt.Sprintf("played-%d-%02d", mk.year, mk.month)
		groups = append(groups, LeveledGroup{Key: key, Title: title, Matches: byMonth[mk]})
	}
	return groups
}

// leveledWindowFor builds the leveledWindow from a leveled competition's
// stored start/end dates and target_matches.
func leveledWindowFor(comp *core.Record) leveledWindow {
	return leveledWindow{
		start:  comp.GetDateTime("start_date").Time(),
		end:    comp.GetDateTime("end_date").Time(),
		target: comp.GetInt("target_matches"),
	}
}

// fmtShortDate formats t as "D mes" in the given timezone (e.g. "15 enero").
func fmtShortDate(t time.Time, tz *time.Location) string {
	local := t.In(tz)
	return fmt.Sprintf("%d %s", local.Day(), spanishMonths[local.Month()-1])
}

// leveledRoundsCtx holds the display context for buildLeveledRounds.
type leveledRoundsCtx struct {
	PairNames     map[string]string
	PlayerPairIDs map[string]struct{}
	PairFilter    string
	Window        leveledWindow
}

// buildLeveledRounds converts matches into leveled RoundView groups for the
// public competition page.
func buildLeveledRounds(matches []*core.Record, ctx leveledRoundsCtx, tz *time.Location) []RoundView {
	var cards []MatchCard
	for _, m := range matches {
		if ctx.PairFilter != "" && ctx.PairFilter != "all" &&
			m.GetString("pair1") != ctx.PairFilter && m.GetString("pair2") != ctx.PairFilter {
			continue
		}
		cards = append(cards, NewMatchRow(m, ctx.PairNames, ctx.PlayerPairIDs))
	}

	groups := leveledGroups(cards, tz, ctx.Window)
	rounds := make([]RoundView, 0, len(groups))
	for _, g := range groups {
		rounds = append(rounds, RoundView{
			Key:     g.Key,
			Title:   g.Title,
			Matches: g.Matches,
		})
	}
	return rounds
}
