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
// (still pending) float to the top of "Por jugar".
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

// pendingByArrangeBy sorts "Por jugar" matches by scheduling state first
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
//   - "Por jugar": non-final matches sorted by arrange_by (empty deadline last)
//   - "Jugados — <mes año>": finalized matches grouped by calendar month of
//     finalized_at in the given timezone, newest month first.
func leveledGroups(cards []MatchCard, tz *time.Location) []LeveledGroup {
	var pending []MatchCard
	byMonth := map[monthKey][]MatchCard{}
	var monthOrder []monthKey
	seen := map[monthKey]bool{}

	for _, mc := range cards {
		if mc.Match.GetString("status") != league.StatusFinal {
			pending = append(pending, mc)
			continue
		}
		ft := mc.Match.GetDateTime("finalized_at").Time()
		if ft.IsZero() {
			ft = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		}
		local := ft.In(tz)
		mk := monthKey{year: local.Year(), month: local.Month()}
		byMonth[mk] = append(byMonth[mk], mc)
		if !seen[mk] {
			seen[mk] = true
			monthOrder = append(monthOrder, mk)
		}
	}

	sort.SliceStable(pending, pendingByArrangeBy(pending))
	sort.Slice(monthOrder, newerMonthFirst(monthOrder))

	var groups []LeveledGroup

	if len(pending) > 0 {
		groups = append(groups, LeveledGroup{
			Key:     "pending",
			Title:   "Por jugar",
			Matches: pending,
		})
	}

	for _, mk := range monthOrder {
		title := fmt.Sprintf("Jugados — %s %d", spanishMonths[mk.month-1], mk.year)
		key := fmt.Sprintf("played-%d-%02d", mk.year, mk.month)
		groups = append(groups, LeveledGroup{
			Key:     key,
			Title:   title,
			Matches: byMonth[mk],
		})
	}

	return groups
}

// leveledRoundsCtx holds the display context for buildLeveledRounds.
type leveledRoundsCtx struct {
	PairNames     map[string]string
	PlayerPairIDs map[string]struct{}
	PairFilter    string
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

	groups := leveledGroups(cards, tz)
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
