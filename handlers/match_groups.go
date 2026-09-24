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

type weekKey struct {
	year int
	week int
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

// leveledGroups partitions match cards into:
//   - "Jornada N" (N=1..open): pending/scheduled/confirmed/disputed matches
//     at that per-pair ordinal slot, titled with the date range spanning
//     their arrange_by values (or just "Jornada N" without dates)
//   - "Bloque N": slot > open matches with no arrange_by (dateless fallback),
//     or slot = 0 matches under the defensive "Sin asignar" title
//   - "Semana del D al D de <mes>": slot > open matches with arrange_by,
//     grouped by ISO week
//   - "Jugados — <mes año>": finalized matches grouped by calendar month of
//     finalized_at in the given timezone, newest month first
func leveledGroups(cards []MatchCard, tz *time.Location, open int) []LeveledGroup {
	jornadas := map[int][]MatchCard{}
	weekBuckets := map[weekKey][]MatchCard{}
	bloques := map[int][]MatchCard{}
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
		switch {
		case slot > 0 && slot <= open:
			jornadas[slot] = append(jornadas[slot], mc)
		case slot > open:
			ab := mc.Match.GetDateTime("arrange_by").Time()
			if ab.IsZero() {
				bloques[slot] = append(bloques[slot], mc)
				continue
			}
			local := ab.In(tz)
			y, w := local.ISOWeek()
			weekBuckets[weekKey{year: y, week: w}] = append(weekBuckets[weekKey{year: y, week: w}], mc)
		default:
			bloques[0] = append(bloques[0], mc)
		}
	}

	var groups []LeveledGroup
	groups = append(groups, jornadaGroups(jornadas, open, tz)...)
	groups = append(groups, bloqueGroups(bloques)...)
	groups = append(groups, weekGroups(weekBuckets, tz)...)
	groups = append(groups, playedGroups(byMonth, monthOrder)...)
	return groups
}

// jornadaGroups builds the "Jornada N" groups (slot 1..open), ascending.
func jornadaGroups(jornadas map[int][]MatchCard, open int, tz *time.Location) []LeveledGroup {
	var groups []LeveledGroup
	for s := 1; s <= open; s++ {
		ms := jornadas[s]
		if len(ms) == 0 {
			continue
		}
		sort.SliceStable(ms, pendingByArrangeBy(ms))
		title := fmt.Sprintf("Jornada %d", s)
		if lo, hi, ok := arrangeByRange(ms); ok {
			title = fmt.Sprintf("Jornada %d — %s al %s", s, fmtShortDate(lo, tz), fmtShortDate(hi, tz))
		}
		groups = append(groups, LeveledGroup{Key: fmt.Sprintf("jornada-%d", s), Title: title, Matches: ms})
	}
	return groups
}

// bloqueGroups builds the dateless-fallback groups: slot > open with no
// arrange_by, and the defensive slot=0 "Sin asignar" group.
func bloqueGroups(bloques map[int][]MatchCard) []LeveledGroup {
	slots := make([]int, 0, len(bloques))
	for s := range bloques {
		slots = append(slots, s)
	}
	sort.Ints(slots)

	var groups []LeveledGroup
	for _, s := range slots {
		ms := bloques[s]
		sort.SliceStable(ms, pendingByArrangeBy(ms))
		title := fmt.Sprintf("Bloque %d", s)
		if s == 0 {
			title = "Sin asignar"
		}
		groups = append(groups, LeveledGroup{Key: fmt.Sprintf("bloque-%d", s), Title: title, Matches: ms})
	}
	return groups
}

// weekGroups builds the "Semana del D al D de <mes>" groups for top-up
// matches with a known arrange_by, in chronological order.
func weekGroups(weekBuckets map[weekKey][]MatchCard, tz *time.Location) []LeveledGroup {
	keys := make([]weekKey, 0, len(weekBuckets))
	for wk := range weekBuckets {
		keys = append(keys, wk)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].year != keys[j].year {
			return keys[i].year < keys[j].year
		}
		return keys[i].week < keys[j].week
	})

	var groups []LeveledGroup
	for _, wk := range keys {
		ms := weekBuckets[wk]
		sort.SliceStable(ms, pendingByArrangeBy(ms))
		mon := isoWeekMonday(wk.year, wk.week, tz)
		sun := mon.AddDate(0, 0, 6)
		title := fmt.Sprintf("Semana del %s al %s", fmtShortDate(mon, tz), fmtShortDate(sun, tz))
		groups = append(groups, LeveledGroup{
			Key: fmt.Sprintf("week-%d-%02d", wk.year, wk.week), Title: title, Matches: ms,
		})
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

// arrangeByRange returns the min and max arrange_by across ms, ok=false when
// none have one set.
func arrangeByRange(ms []MatchCard) (lo, hi time.Time, ok bool) {
	for _, mc := range ms {
		ab := mc.Match.GetDateTime("arrange_by").Time()
		if ab.IsZero() {
			continue
		}
		if !ok || ab.Before(lo) {
			lo = ab
		}
		if !ok || ab.After(hi) {
			hi = ab
		}
		ok = true
	}
	return lo, hi, ok
}

// fmtShortDate formats t as "D de <mes>" in the given timezone (e.g. "15 de enero").
func fmtShortDate(t time.Time, tz *time.Location) string {
	local := t.In(tz)
	return fmt.Sprintf("%d de %s", local.Day(), spanishMonths[local.Month()-1])
}

// isoWeekMonday returns the Monday of the given ISO week/year at midnight in tz.
func isoWeekMonday(year, week int, tz *time.Location) time.Time {
	// Jan 4 is always in ISO week 1; walk to that week's Monday, then add
	// (week-1) weeks.
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, tz)
	offset := int(jan4.Weekday())
	if offset == 0 {
		offset = 7 // Sunday → ISO weekday 7
	}
	week1Monday := jan4.AddDate(0, 0, -(offset - 1))
	return week1Monday.AddDate(0, 0, (week-1)*7)
}

// leveledRoundsCtx holds the display context for buildLeveledRounds.
type leveledRoundsCtx struct {
	PairNames     map[string]string
	PlayerPairIDs map[string]struct{}
	PairFilter    string
	Open          int
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

	groups := leveledGroups(cards, tz, ctx.Open)
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
