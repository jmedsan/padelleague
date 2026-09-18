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

// leveledGroups partitions match cards into:
//   - "Por jugar": non-final matches sorted by arrange_by (empty deadline last)
//   - "Jugados — <mes año>": finalized matches grouped by calendar month of
//     finalized_at in the given timezone, newest month first.
func leveledGroups(cards []MatchCard, tz *time.Location) []LeveledGroup {
	type monthKey struct {
		year  int
		month time.Month
	}

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

	sort.SliceStable(pending, func(i, j int) bool {
		ai := pending[i].Match.GetDateTime("arrange_by").Time()
		aj := pending[j].Match.GetDateTime("arrange_by").Time()
		if ai.IsZero() && aj.IsZero() {
			return false
		}
		if ai.IsZero() {
			return false
		}
		if aj.IsZero() {
			return true
		}
		return ai.Before(aj)
	})

	sort.Slice(monthOrder, func(i, j int) bool {
		if monthOrder[i].year != monthOrder[j].year {
			return monthOrder[i].year > monthOrder[j].year
		}
		return monthOrder[i].month > monthOrder[j].month
	})

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

// buildLeveledRounds converts matches into leveled RoundView groups for the
// public competition page.
func buildLeveledRounds(matches []*core.Record, pairNames map[string]string, playerPairIDs map[string]struct{}, pairFilter string, tz *time.Location) []RoundView {
	var cards []MatchCard
	for _, m := range matches {
		if pairFilter != "" && pairFilter != "all" &&
			m.GetString("pair1") != pairFilter && m.GetString("pair2") != pairFilter {
			continue
		}
		cards = append(cards, NewMatchRow(m, pairNames, playerPairIDs))
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
