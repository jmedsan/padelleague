package league

import (
	"flag"
	"fmt"
	"math/rand/v2"
	"sort"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/require"
)

// Calendar simulation: the season runs on real dates (the scenario window
// 29/09–10/12/2036), Jornadas and deadlines come from JornadaWindow /
// SlotDeadline through the production code, and each pair plays at its own
// pace. Every day, the pending matches whose two pairs are both free are
// played in order of the earliest arrange_by; finalization and a daily
// pass both trigger TopUpAssignments with that day as now, exactly as the
// hooks and the cron do. Opt-in: -calendar.seasons=N.

var (
	calSeasons = flag.Int("calendar.seasons", 0, "seasons for the calendar simulation; 0 skips it")
	calSeed    = flag.Uint64("calendar.seed", 0, "RNG seed; 0 = time-based, logged")
	calAdmin   = flag.Bool("calendar.admin", false, "model an admin using the existing tools: release after 14 overdue days, 7-day recovery week, walkovers at close")
	calRules   = flag.Bool("calendar.rulebook", false, "with -calendar.admin: close per the rulebook instead of walkovers: -1 per pending match above 2 at end_date (oldest first), -1 to each pair per match unplayed after the extra week")
)

const (
	adminReleaseAfterDays = 14 // a match overdue this long is released and re-paired
	adminRecoveryDays     = 7  // rulebook: "7 días extraordinarios" after the regular league
)

// Behavior profiles, drawn per pair and season (assumed shares of an amateur
// league; every profile still needs both pairs free on the day):
//   - normal (60%): pace 3 days (a third of them) or 7 days;
//   - lazy (20%): pace 7 days, and only plays a match within 3 days of its
//     deadline or once it is overdue;
//   - gap (15%): normal pace, but unavailable for one block of 14-35 days
//     (injury, trip) starting at a random point of the season;
//   - abandon (5%): normal pace until a random day in the middle third, then
//     withdraws: its pending matches become walkovers and it leaves the roster.
type profile int

const (
	profNormal profile = iota
	profLazy
	profGap
	profAbandon
)

var profileNames = []string{"normal", "lazy", "gap", "abandon"}

type pairBehavior struct {
	prof      profile
	pace      int
	gapFrom   time.Time
	gapTo     time.Time
	quitDay   time.Time
	withdrawn bool
}

func drawBehavior(rng *rand.Rand, start, end time.Time) pairBehavior {
	b := pairBehavior{pace: 7}
	if rng.Float64() < 1.0/3 {
		b.pace = 3
	}
	days := SeasonDays(start, end)
	switch r := rng.Float64(); {
	case r < 0.60:
		b.prof = profNormal
	case r < 0.80:
		b.prof = profLazy
		b.pace = 7
	case r < 0.95:
		b.prof = profGap
		from := start.AddDate(0, 0, rng.IntN(days-14))
		b.gapFrom = from
		b.gapTo = from.AddDate(0, 0, 14+rng.IntN(22))
	default:
		b.prof = profAbandon
		b.quitDay = start.AddDate(0, 0, days/3+rng.IntN(days/3))
	}
	return b
}

// available reports whether the pair can play a match with the given
// deadline on day.
func (b *pairBehavior) available(day, nextFree, deadline time.Time) bool {
	if b.withdrawn || nextFree.After(day) {
		return false
	}
	if b.prof == profGap && !day.Before(b.gapFrom) && day.Before(b.gapTo) {
		return false
	}
	if b.prof == profLazy && daysBetween(day, deadline) > 3 {
		return false
	}
	return true
}

const (
	calStart = "2036-09-29"
	calEnd   = "2036-12-10"
)

type calSeasonResult struct {
	played        int
	overdue       int // played after arrange_by
	unplayed      int // still pending after end_date
	shortPairs    int // active pairs below target at the end
	walkovers     int
	withdrawals   int
	peakPending   map[string]int
	overdueByPair map[string]int
	maxLate       int // worst lateness in days
	prof          map[string]profile
	short         map[string]bool
	// per opponent profile: matches played against it and how many were overdue
	vsPlayed  [4]int
	vsOverdue [4]int
	// admin actions
	releases     int
	closeWalkers int // walkovers at close with a clear responsible pair
	closeAnnul   int // unplayed at close with no responsible pair (rulebook: -1 each; no tool)
	// rulebook close: penalty points per pair
	penalty map[string]int
}

func TestCalendarSimulation(t *testing.T) {
	if *calSeasons == 0 {
		t.Skip("opt-in: run with -calendar.seasons=N")
	}
	seed := *calSeed
	if seed == 0 {
		seed = uint64(time.Now().UnixNano())
	}
	t.Logf("calendar simulation seed: %d, window %s..%s, target %d, open %d", seed, calStart, calEnd, simTarget, simOpen)
	rng := rand.New(rand.NewPCG(seed, 1))
	model := newMatchModel(simSkills())

	app := newTestApp(t)
	svc := New(app, nil)
	pairs := make([]*core.Record, simPairs)
	for k := range pairs {
		pairs[k] = makePair(t, app, fmt.Sprintf("Cal %02d", k+1))
	}

	var results []calSeasonResult
	for range *calSeasons {
		results = append(results, runCalendarSeason(t, app, svc, pairs, model, rng))
	}
	t.Log(calendarReport(results))
}

func runCalendarSeason(t *testing.T, app core.App, svc *Service, pairs []*core.Record, model *matchModel, rng *rand.Rand) calSeasonResult {
	t.Helper()
	// Levels: 80% levelled with a mixed-quality seed, 3 unranked (the owner's assumption).
	perm := rng.Perm(simPairs)
	trueIdx := make(map[string]int, simPairs)
	pairIDs := make([]string, simPairs)
	for k, p := range pairs {
		trueIdx[p.Id] = perm[k]
		pairIDs[k] = p.Id
	}
	levels := noisySeedLevels(pairIDs, trueIdx, []float64{2, 4, 6}[rng.IntN(3)], rng)
	for _, k := range rng.Perm(simPairs)[:3] {
		levels[pairIDs[k]] = "unranked"
	}
	for _, p := range pairs {
		p.Set("level", levels[p.Id])
		require.NoError(t, app.Save(p))
	}
	comp := makeLeveledCompetition(t, app, pairs, simTarget, simOpen)
	comp.Set("start_date", calStart)
	comp.Set("end_date", calEnd)
	require.NoError(t, app.Save(comp))

	start, _ := time.Parse("2006-01-02", calStart)
	end, _ := time.Parse("2006-01-02", calEnd)
	_, err := svc.GenerateInitialAssignments(app, comp, start.AddDate(0, 0, -7))
	require.NoError(t, err)

	behavior := map[string]*pairBehavior{}
	nextFree := map[string]time.Time{}
	res := calSeasonResult{peakPending: map[string]int{}, overdueByPair: map[string]int{}, prof: map[string]profile{}, short: map[string]bool{}}
	for _, p := range pairs {
		b := drawBehavior(rng, start, end)
		behavior[p.Id] = &b
		nextFree[p.Id] = start
		res.prof[p.Id] = b.prof
	}

	last := end
	if *calAdmin {
		last = end.AddDate(0, 0, adminRecoveryDays)
		comp.Set("recovery_days", adminRecoveryDays)
		require.NoError(t, app.Save(comp))
	}
	blocked := map[string][2]int{} // match id -> days each side alone blocked it
	res.penalty = map[string]int{}
	now := start
	pendingAtEnd := map[string]int{}
	for day := start; !day.After(last); day = day.AddDate(0, 0, 1) {
		now = day.Add(20 * time.Hour)
		if day.Equal(end.AddDate(0, 0, 1)) {
			for _, m := range pendingCalendarMatches(t, app, comp.Id) {
				pendingAtEnd[m.GetString("pair1")]++
				pendingAtEnd[m.GetString("pair2")]++
			}
		}
		for _, p := range pairs {
			b := behavior[p.Id]
			if b.prof == profAbandon && !b.withdrawn && !day.Before(b.quitDay) {
				b.withdrawn = true
				res.withdrawals++
				res.walkovers += withdrawCalendarPair(t, app, svc, comp, p.Id, now)
			}
		}
		pend := pendingCalendarMatches(t, app, comp.Id)
		trackPeak(res.peakPending, pend)
		sort.SliceStable(pend, func(i, j int) bool {
			ai, aj := pend[i].GetDateTime("arrange_by").Time(), pend[j].GetDateTime("arrange_by").Time()
			if !ai.Equal(aj) {
				return ai.Before(aj)
			}
			return pend[i].GetString("created") < pend[j].GetString("created")
		})
		busy := map[string]bool{}
		for _, m := range pend {
			p1, p2 := m.GetString("pair1"), m.GetString("pair2")
			deadline := m.GetDateTime("arrange_by").Time()
			a1 := behavior[p1].available(day, nextFree[p1], deadline)
			a2 := behavior[p2].available(day, nextFree[p2], deadline)
			if a1 != a2 {
				b := blocked[m.Id]
				if a1 {
					b[1]++
				} else {
					b[0]++
				}
				blocked[m.Id] = b
			}
			if *calAdmin && day.Before(end) && daysBetween(deadline, day) > adminReleaseAfterDays {
				// Admin release: the match is deleted and both pairs re-paired (delete hook path).
				require.NoError(t, app.Delete(m))
				res.releases++
				_, err := svc.TopUpAssignments(comp.Id, now, Pairing{A: p1, B: p2})
				require.NoError(t, err)
				continue
			}
			if busy[p1] || busy[p2] || !a1 || !a2 {
				continue
			}
			busy[p1], busy[p2] = true, true
			nextFree[p1] = day.AddDate(0, 0, behavior[p1].pace)
			nextFree[p2] = day.AddDate(0, 0, behavior[p2].pace)
			late := daysBetween(deadline, day)
			res.vsPlayed[behavior[p2].prof]++
			res.vsPlayed[behavior[p1].prof]++
			if late > 0 {
				res.overdue++
				res.overdueByPair[p1]++
				res.overdueByPair[p2]++
				res.vsOverdue[behavior[p2].prof]++
				res.vsOverdue[behavior[p1].prof]++
				res.maxLate = max(res.maxLate, late)
			}
			score := model.play(trueIdx[p1], trueIdx[p2], rng)
			finalizeCalendarMatch(t, app, m, score, now)
			res.played++
			_, err := svc.TopUpAssignments(comp.Id, now, Pairing{A: p1, B: p2})
			require.NoError(t, err)
		}
		// Daily cron pass.
		_, err := svc.TopUpAssignments(comp.Id, now)
		require.NoError(t, err)
	}
	res.unplayed = len(pendingCalendarMatches(t, app, comp.Id))
	if *calAdmin && *calRules {
		// Rulebook: at end_date, every pending match above 2 per pair costs -1
		// (oldest first); after the extra week, every unplayed match costs -1
		// to each pair. Unplayed matches stay unplayed (no result awarded).
		for _, m := range pendingCalendarMatches(t, app, comp.Id) {
			res.penalty[m.GetString("pair1")]++
			res.penalty[m.GetString("pair2")]++
		}
		for p, n := range pendingAtEnd {
			if n > 2 {
				res.penalty[p] += n - 2
			}
		}
	} else if *calAdmin {
		// Close: report-unplayed → walkover with penalty against the side that
		// blocked the match more days; no responsible side → rulebook -1 each.
		for _, m := range pendingCalendarMatches(t, app, comp.Id) {
			b := blocked[m.Id]
			p1, p2 := m.GetString("pair1"), m.GetString("pair2")
			switch {
			case b[0] > b[1]:
				finalizeWalkover(t, app, m, p2, now)
				res.closeWalkers++
			case b[1] > b[0]:
				finalizeWalkover(t, app, m, p1, now)
				res.closeWalkers++
			default:
				res.closeAnnul++
			}
		}
	}
	for _, p := range pairs {
		if behavior[p.Id].withdrawn {
			continue
		}
		played := 0
		for _, m := range allMatchesFor(t, app, comp.Id, p.Id) {
			if m.GetString("status") == "final" {
				played++
			}
		}
		if played < simTarget {
			res.shortPairs++
			res.short[p.Id] = true
		}
	}
	cleanupCalendarSeason(t, app, comp)
	return res
}

// withdrawCalendarPair mirrors WithdrawPair: pending matches become walkovers
// for the opponent, the pair joins withdrawn_pairs, and the top-up runs.
func withdrawCalendarPair(t *testing.T, app core.App, svc *Service, comp *core.Record, pairID string, now time.Time) int {
	t.Helper()
	pend, err := app.FindRecordsByFilter("matches",
		"competition = {:c} && status = 'pending' && (pair1 = {:p} || pair2 = {:p})", "", 0, 0,
		map[string]any{"c": comp.Id, "p": pairID})
	require.NoError(t, err)
	for _, m := range pend {
		opp := m.GetString("pair2")
		if opp == pairID {
			opp = m.GetString("pair1")
		}
		m.Set("scores", "6-0 6-0")
		m.Set("winner", opp)
		m.Set("status", "final")
		m.Set("review_type", "walkover")
		m.Set("finalized_at", now.UTC().Format("2006-01-02 15:04:05.000Z"))
		require.NoError(t, app.Save(m))
	}
	fresh, err := app.FindRecordById("competitions", comp.Id)
	require.NoError(t, err)
	fresh.Set("withdrawn_pairs", append(fresh.GetStringSlice("withdrawn_pairs"), pairID))
	require.NoError(t, app.Save(fresh))
	_, err = svc.TopUpAssignments(comp.Id, now)
	require.NoError(t, err)
	return len(pend)
}

func pendingCalendarMatches(t *testing.T, app core.App, compID string) []*core.Record {
	t.Helper()
	recs, err := app.FindRecordsByFilter("matches",
		"competition = {:c} && status = 'pending'", "", 0, 0, map[string]any{"c": compID})
	require.NoError(t, err)
	return recs
}

func trackPeak(peak map[string]int, pend []*core.Record) {
	count := map[string]int{}
	for _, m := range pend {
		count[m.GetString("pair1")]++
		count[m.GetString("pair2")]++
	}
	for p, c := range count {
		peak[p] = max(peak[p], c)
	}
}

func finalizeCalendarMatch(t *testing.T, app core.App, m *core.Record, score string, at time.Time) {
	t.Helper()
	winner, err := DetermineWinner(m, score)
	require.NoError(t, err)
	m.Set("scores", score)
	m.Set("winner", winner)
	m.Set("status", "final")
	m.Set("finalized_at", at.UTC().Format("2006-01-02 15:04:05.000Z"))
	require.NoError(t, app.Save(m))
}

func finalizeWalkover(t *testing.T, app core.App, m *core.Record, winner string, at time.Time) {
	t.Helper()
	m.Set("scores", "6-0 6-0")
	m.Set("winner", winner)
	m.Set("status", "final")
	m.Set("review_type", "walkover")
	m.Set("finalized_at", at.UTC().Format("2006-01-02 15:04:05.000Z"))
	require.NoError(t, app.Save(m))
}

func cleanupCalendarSeason(t *testing.T, app core.App, comp *core.Record) {
	t.Helper()
	matches, err := app.FindRecordsByFilter("matches",
		"competition = {:c}", "", 0, 0, map[string]any{"c": comp.Id})
	require.NoError(t, err)
	for _, m := range matches {
		require.NoError(t, app.Delete(m))
	}
	require.NoError(t, app.Delete(comp))
}

// calendarReport aggregates the seasons into one plain table.
func calendarReport(results []calSeasonResult) string {
	n := float64(len(results))
	played, overdue, unplayed, short, exact, maxLate := 0, 0, 0, 0, 0, 0
	var peakHist [6]int
	// overdue per pair-season grouped by that pair's peak pending
	group := map[string][2]int{} // bucket -> {pairSeasons, overdueSum}
	for _, r := range results {
		played += r.played
		overdue += r.overdue
		unplayed += r.unplayed
		short += r.shortPairs
		maxLate = max(maxLate, r.maxLate)
		if r.shortPairs == 0 {
			exact++
		}
		for p, peak := range r.peakPending {
			switch {
			case peak <= 3:
				peakHist[0]++
			case peak >= 8:
				peakHist[5]++
			default:
				peakHist[peak-3]++
			}
			b := "<=4"
			if peak == 5 {
				b = "5"
			} else if peak >= 6 {
				b = "6+"
			}
			g := group[b]
			g[0]++
			g[1] += r.overdueByPair[p]
			group[b] = g
		}
	}
	var profN, profOver, profShort, profPen [4]int
	var vsPlayed, vsOverdue [4]int
	walk, withd, rel, cw, ca := 0, 0, 0, 0, 0
	for _, r := range results {
		walk += r.walkovers
		withd += r.withdrawals
		rel += r.releases
		cw += r.closeWalkers
		ca += r.closeAnnul
		for p, pr := range r.prof {
			profN[pr]++
			profOver[pr] += r.overdueByPair[p]
			if r.short[p] {
				profShort[pr]++
			}
			profPen[pr] += r.penalty[p]
		}
		for i := range 4 {
			vsPlayed[i] += r.vsPlayed[i]
			vsOverdue[i] += r.vsOverdue[i]
		}
	}
	profRows := ""
	for i, name := range profileNames {
		if profN[i] == 0 {
			continue
		}
		profRows += fmt.Sprintf("| %s | %d | %.2f | %.0f%% | %.1f%% (n=%d) | %.2f |\n", name, profN[i],
			float64(profOver[i])/float64(profN[i]), 100*float64(profShort[i])/float64(profN[i]),
			100*float64(vsOverdue[i])/float64(max(vsPlayed[i], 1)), vsPlayed[i], float64(profPen[i])/float64(profN[i]))
	}
	perPair := func(b string) string {
		g := group[b]
		if g[0] == 0 {
			return "—"
		}
		return fmt.Sprintf("%.2f (n=%d)", float64(g[1])/float64(g[0]), g[0])
	}
	return fmt.Sprintf("\n| Seasons | Matches played/season | Overdue/season (after arrange_by) | Unplayed at end_date/season | Pairs short/season | Seasons with every pair at target | Worst lateness (days) | Peak pending per pair-season (≤3/4/5/6/7/8+) | Overdue per pair-season by peak pending ≤4 / 5 / 6+ |\n|---|---|---|---|---|---|---|---|---|\n| %d | %.1f | %.1f (%.1f%%) | %.2f | %.2f | %.0f%% | %d | %d/%d/%d/%d/%d/%d | %s / %s / %s |",
		len(results), float64(played)/n, float64(overdue)/n, 100*float64(overdue)/float64(max(played, 1)),
		float64(unplayed)/n, float64(short)/n, 100*float64(exact)/n, maxLate,
		peakHist[0], peakHist[1], peakHist[2], peakHist[3], peakHist[4], peakHist[5],
		perPair("<=4"), perPair("5"), perPair("6+")) +
		fmt.Sprintf("\n\nWithdrawals: %.2f/season, withdrawal walkovers: %.2f/season, admin releases: %.2f/season, close walkovers (responsible side): %.2f/season, close with no responsible side (rulebook -1 each, no tool): %.2f/season\n\n| Profile | Pair-seasons | Overdue per pair-season | Ends short | Overdue share of matches played AGAINST this profile | Rulebook penalty points per pair-season |\n|---|---|---|---|---|---|\n%s", float64(withd)/n, float64(walk)/n, float64(rel)/n, float64(cw)/n, float64(ca)/n, profRows)
}
