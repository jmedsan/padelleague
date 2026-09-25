package league

import (
	"flag"
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fairnessRuns is the opt-in flag gating TestFairness_InitialAssignments —
// 0 (the default) skips the test, matching simulation_test.go's pattern.
var fairnessRuns = flag.Int("fairness.runs", 0, "GenerateInitialAssignments draws per variant; 0 skips the fairness test")

// fairnessSeasonRuns is the opt-in flag gating TestFairness_FullSeason.
var fairnessSeasonRuns = flag.Int("fairness.seasons", 0, "full-season draws per variant; 0 skips the full-season fairness test")

const (
	fairnessPairs  = 16
	fairnessTarget = 10
	fairnessOpen   = 3

	// regressionRuns/regressionMeanGapMax/regressionSDWinMax gate
	// TestFairness_Regression, which runs by default in make test (unlike
	// the opt-in tests above) to catch a fairness regression in
	// GenerateInitialAssignments. Thresholds sit between the pre-elo100
	// rank-distance baseline (mean gap ~106, SD win% ~12.4) and elo100's
	// measured mean (~65, ~9.0) — see the commit message for the full
	// before/after table.
	regressionRuns       = 20
	regressionMeanGapMax = 85.0
	regressionSDWinMax   = 10.5
)

// fairnessDates mirrors e2e/scenario-helpers.ts competitionDates().
var (
	fairnessStart = time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	fairnessEnd   = time.Date(2026, 12, 6, 12, 0, 0, 0, time.UTC)
)

// scenarioLevels mirrors e2e/scenario-helpers.ts PAIR_LEVELS exactly: 2
// advanced, 4 intermediate variants, 6 beginner variants, 4 left unranked
// (empty string, same as a pair the admin never classified).
var scenarioLevels = []string{
	"advanced", "advanced",
	"intermediate_high", "intermediate", "intermediate", "intermediate_low",
	"beginner_high", "beginner_high", "beginner", "beginner", "beginner", "beginner",
	"", "", "", "",
}

// uniformLevels spreads 16 pairs evenly over the 7 ranked levels (unranked
// excluded — it exists to model "never classified", not a rung of the
// skill ladder), roughly 2-3 pairs per level.
func uniformLevels() []string {
	ranked := make([]string, 0, len(Levels))
	for _, l := range Levels {
		if l.Key != "unranked" {
			ranked = append(ranked, l.Key)
		}
	}
	levels := make([]string, fairnessPairs)
	for i := range levels {
		levels[i] = ranked[i%len(ranked)]
	}
	return levels
}

// fairnessRunMetrics holds the metrics computed from one GenerateInitial
// Assignments draw.
type fairnessRunMetrics struct {
	maxWinPct  float64
	minWinPct  float64
	sdWinPct   float64
	meanGap    float64
	gap200Plus int
	matchCount int
}

// fairnessDist summarizes one metric's distribution across every run.
type fairnessDist struct {
	mean, p50, p95, worst float64
}

func summarize(values []float64, higherIsWorse bool) fairnessDist {
	if len(values) == 0 {
		return fairnessDist{}
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)

	sum := 0.0
	for _, v := range sorted {
		sum += v
	}
	mean := sum / float64(len(sorted))
	p50 := percentile(sorted, 0.50)
	p95 := percentile(sorted, 0.95)
	worst := sorted[len(sorted)-1]
	if !higherIsWorse {
		worst = sorted[0]
	}
	return fairnessDist{mean: mean, p50: p50, p95: p95, worst: worst}
}

// percentile does a simple linear-interpolation percentile over an
// already-sorted slice.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 1 {
		return sorted[0]
	}
	idx := p * float64(len(sorted)-1)
	lo := int(math.Floor(idx))
	hi := int(math.Ceil(idx))
	if lo == hi {
		return sorted[lo]
	}
	frac := idx - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

// expectedWinPct is the standard Elo win-probability formula.
func expectedWinPct(ownElo, oppElo float64) float64 {
	return 1 / (1 + math.Pow(10, (oppElo-ownElo)/400))
}

// pairMatch is the (pair1, pair2) shape measureAssignments needs — either
// read off a real match record or synthesized for the random baseline.
type pairMatch struct{ pair1, pair2 string }

// measureAssignments computes fairnessRunMetrics from one batch of matches,
// given each pair's Elo (by level).
func measureAssignments(matches []pairMatch, elo map[string]float64) fairnessRunMetrics {
	type acc struct {
		winPcts []float64
	}
	perPair := make(map[string]*acc)
	for id := range elo {
		perPair[id] = &acc{}
	}

	var gapSum float64
	var gap200 int
	for _, m := range matches {
		p1, p2 := m.pair1, m.pair2
		e1, e2 := elo[p1], elo[p2]
		gap := math.Abs(e1 - e2)
		gapSum += gap
		if gap >= 200 {
			gap200++
		}

		w1 := expectedWinPct(e1, e2)
		w2 := 1 - w1
		perPair[p1].winPcts = append(perPair[p1].winPcts, w1)
		perPair[p2].winPcts = append(perPair[p2].winPcts, w2)
	}

	var pairWinPcts []float64
	for _, a := range perPair {
		if len(a.winPcts) == 0 {
			continue
		}
		sum := 0.0
		for _, w := range a.winPcts {
			sum += w
		}
		pairWinPcts = append(pairWinPcts, sum/float64(len(a.winPcts))*100)
	}

	if len(pairWinPcts) == 0 || len(matches) == 0 {
		return fairnessRunMetrics{}
	}

	maxW, minW := pairWinPcts[0], pairWinPcts[0]
	sum := 0.0
	for _, w := range pairWinPcts {
		if w > maxW {
			maxW = w
		}
		if w < minW {
			minW = w
		}
		sum += w
	}
	mean := sum / float64(len(pairWinPcts))
	var sqDiff float64
	for _, w := range pairWinPcts {
		d := w - mean
		sqDiff += d * d
	}
	sd := math.Sqrt(sqDiff / float64(len(pairWinPcts)))

	return fairnessRunMetrics{
		maxWinPct:  maxW,
		minWinPct:  minW,
		sdWinPct:   sd,
		meanGap:    gapSum / float64(len(matches)),
		gap200Plus: gap200,
		matchCount: len(matches),
	}
}

// setupFairnessCompetition creates a 16-pair leveled competition with the
// given level assignment and the scenario's dates, ready for repeated
// GenerateInitialAssignments draws.
func setupFairnessCompetition(t *testing.T, app core.App, levels []string) (*core.Record, map[string]float64) {
	t.Helper()
	require.Len(t, levels, fairnessPairs, "levels must cover all pairs")

	pairs := make([]*core.Record, fairnessPairs)
	elo := make(map[string]float64, fairnessPairs)
	for i := range pairs {
		p := makePair(t, app, fmt.Sprintf("Fair %02d", i+1))
		if levels[i] != "" {
			p.Set("level", levels[i])
			require.NoError(t, app.Save(p))
		}
		pairs[i] = p
		elo[p.Id] = LevelElo(levels[i])
	}

	comp := makeLeveledCompetition(t, app, pairs, fairnessTarget, fairnessOpen)
	comp.Set("start_date", fairnessStart)
	comp.Set("end_date", fairnessEnd)
	require.NoError(t, app.Save(comp))

	return comp, elo
}

// deleteMatches removes every match for comp, so the next
// GenerateInitialAssignments draw starts from a clean slate.
func deleteMatches(t *testing.T, app core.App, compID string) {
	t.Helper()
	matches, err := app.FindRecordsByFilter("matches", "competition = {:c}", "", 0, 0, map[string]any{"c": compID})
	require.NoError(t, err)
	for _, m := range matches {
		require.NoError(t, app.Delete(m))
	}
}

// runFairnessVariant draws *fairnessRuns initial batches (the real
// GenerateInitialAssignments) against a fresh competition built from
// levels, deleting matches between draws, and returns the per-run metrics.
func runFairnessVariant(t *testing.T, levels []string) []fairnessRunMetrics {
	t.Helper()
	app := newTestApp(t)
	svc := New(app, nil)
	comp, elo := setupFairnessCompetition(t, app, levels)

	// now is before start_date, matching the real pregen flow (admin
	// generates the calendar before the season begins).
	now := fairnessStart.Add(-24 * time.Hour)

	results := make([]fairnessRunMetrics, *fairnessRuns)
	for i := range *fairnessRuns {
		n, err := svc.GenerateInitialAssignments(app, comp, now)
		require.NoError(t, err)
		require.Greater(t, n, 0, "run %d produced no matches", i)
		records, err := app.FindRecordsByFilter("matches", "competition = {:c}", "", 0, 0, map[string]any{"c": comp.Id})
		require.NoError(t, err)

		matches := make([]pairMatch, len(records))
		for j, m := range records {
			matches[j] = pairMatch{pair1: m.GetString("pair1"), pair2: m.GetString("pair2")}
		}

		results[i] = measureAssignments(matches, elo)
		deleteMatches(t, app, comp.Id)
	}
	return results
}

// randomBaselineMetrics simulates a pure-random valid pairing (respecting
// each pair's open_assignments=3 slot count) under the same 16-pair
// scenario level mix, with no rating-proximity logic at all — the
// worst-case fairness baseline GenerateInitialAssignments is compared
// against.
func randomBaselineMetrics(levels []string, rng *rand.Rand) fairnessRunMetrics {
	elo := make(map[string]float64, len(levels))
	ids := make([]string, len(levels))
	slots := make(map[string]int, len(levels))
	for i, lvl := range levels {
		id := fmt.Sprintf("pair-%d", i)
		ids[i] = id
		elo[id] = LevelElo(lvl)
		slots[id] = fairnessOpen
	}

	var pairings []pairMatch
	met := make(map[string]map[string]struct{}, len(ids))
	for _, id := range ids {
		met[id] = map[string]struct{}{}
	}

	for {
		var wanting []string
		for _, id := range ids {
			if slots[id] > 0 {
				wanting = append(wanting, id)
			}
		}
		if len(wanting) < 2 {
			break
		}
		rng.Shuffle(len(wanting), func(i, j int) { wanting[i], wanting[j] = wanting[j], wanting[i] })

		p := wanting[0]
		var q string
		for _, cand := range wanting[1:] {
			if _, already := met[p][cand]; !already {
				q = cand
				break
			}
		}
		if q == "" {
			// p has met every remaining candidate — give up on p this pass.
			slots[p] = 0
			continue
		}
		pairings = append(pairings, pairMatch{pair1: p, pair2: q})
		met[p][q] = struct{}{}
		met[q][p] = struct{}{}
		slots[p]--
		slots[q]--
	}

	return measureAssignments(pairings, elo)
}

// fmtDist renders a fairnessDist as a table row segment.
func fmtDist(name string, d fairnessDist) string {
	return fmt.Sprintf("%-14s mean=%7.2f  p50=%7.2f  p95=%7.2f  worst=%7.2f", name, d.mean, d.p50, d.p95, d.worst)
}

// TestFairness_InitialAssignments measures how uneven GenerateInitial
// Assignments' opening batch of matches is across many independent draws,
// for the scenario's exact level mix, a uniform level mix, and a
// pure-random baseline. Opt-in and skipped by default — run with
// -fairness.runs=N.
func TestFairness_InitialAssignments(t *testing.T) {
	if *fairnessRuns == 0 {
		t.Skip("opt-in: run with -fairness.runs=N")
	}

	mixes := []struct {
		name   string
		levels []string
	}{
		{"scenario mix", scenarioLevels},
		{"uniform mix", uniformLevels()},
	}

	t.Logf("fairness harness: %d runs per mix, %d pairs, target=%d, open=%d",
		*fairnessRuns, fairnessPairs, fairnessTarget, fairnessOpen)

	for _, mix := range mixes {
		t.Run(mix.name, func(t *testing.T) {
			t.Parallel()
			runs := runFairnessVariant(t, mix.levels)
			logInitialBatchTable(t, mix.name, runs)
		})
		t.Run(mix.name+" / random baseline", func(t *testing.T) {
			t.Parallel()
			rng := rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
			runs := make([]fairnessRunMetrics, *fairnessRuns)
			for i := range *fairnessRuns {
				runs[i] = randomBaselineMetrics(mix.levels, rng)
			}
			logInitialBatchTable(t, mix.name+" / random baseline", runs)
		})
	}
}

// logInitialBatchTable logs the distribution summary for one mix's
// initial-batch run set.
func logInitialBatchTable(t *testing.T, name string, runs []fairnessRunMetrics) {
	t.Helper()
	maxW := make([]float64, len(runs))
	minW := make([]float64, len(runs))
	sdW := make([]float64, len(runs))
	meanGap := make([]float64, len(runs))
	gap200 := make([]float64, len(runs))
	for i, r := range runs {
		maxW[i] = r.maxWinPct
		minW[i] = r.minWinPct
		sdW[i] = r.sdWinPct
		meanGap[i] = r.meanGap
		gap200[i] = float64(r.gap200Plus)
	}

	t.Logf("\n=== %s (%d runs) ===\n%s\n%s\n%s\n%s\n%s",
		name,
		len(runs),
		fmtDist("max win%", summarize(maxW, true)),
		fmtDist("min win%", summarize(minW, false)),
		fmtDist("SD win%", summarize(sdW, true)),
		fmtDist("mean |gap|", summarize(meanGap, true)),
		fmtDist("gap>=200 ct", summarize(gap200, true)),
	)
}

// TestFairness_Regression is a cheap, non-opt-in guard against a fairness
// regression in GenerateInitialAssignments: it runs regressionRuns draws on
// the scenario level mix and asserts the mean gap and SD(win%) stay below
// thresholds set between the pre-elo100 rank-distance baseline and
// elo100's measured mean — see the commit that introduced this test for
// the full before/after table. Runs by default in make test.
func TestFairness_Regression(t *testing.T) {
	*fairnessRuns = regressionRuns
	runs := runFairnessVariant(t, scenarioLevels)

	var gapSum, sdSum float64
	for _, r := range runs {
		gapSum += r.meanGap
		sdSum += r.sdWinPct
	}
	meanGap := gapSum / float64(len(runs))
	meanSD := sdSum / float64(len(runs))

	assert.Less(t, meanGap, regressionMeanGapMax,
		"mean |gap| %.2f >= %.2f — GenerateInitialAssignments fairness regressed", meanGap, regressionMeanGapMax)
	assert.Less(t, meanSD, regressionSDWinMax,
		"SD(win%%) %.2f >= %.2f — GenerateInitialAssignments fairness regressed", meanSD, regressionSDWinMax)
}

// -- Full-season simulation --------------------------------------------

// seasonRunMetrics holds the whole-season per-pair metrics for one
// completed leveled-league draw (target_matches matches per pair, reached
// via TopUpAssignments after each match is finalized).
type seasonRunMetrics struct {
	maxAbsBias   float64
	sdWinPct     float64
	meanGap      float64
	gap200Plus   int
	matchCount   int
	jornadaGaps  map[int]float64 // slot -> mean gap for matches assigned at that slot
	jornadaCount map[int]int
}

// assignedMatch is one pairing with the Elo of each side captured AT
// ASSIGNMENT TIME (ratings drift as results come in, so this must be
// snapshotted right when GenerateInitialAssignments/TopUpAssignments
// return it — never recomputed later from the final Ratings()).
type assignedMatch struct {
	pair1, pair2 string
	elo1, elo2   float64
	slot         int
}

// measureSeason computes seasonRunMetrics from every assignedMatch in a
// completed season.
func measureSeason(assigned []assignedMatch) seasonRunMetrics {
	type acc struct {
		winPcts []float64
		oppElo  []float64
		ownElo  float64
	}
	perPair := make(map[string]*acc)

	var gapSum float64
	var gap200 int
	jornadaGapSum := make(map[int]float64)
	jornadaCount := make(map[int]int)

	for _, m := range assigned {
		gap := math.Abs(m.elo1 - m.elo2)
		gapSum += gap
		if gap >= 200 {
			gap200++
		}
		jornadaGapSum[m.slot] += gap
		jornadaCount[m.slot]++

		w1 := expectedWinPct(m.elo1, m.elo2)
		w2 := 1 - w1

		if perPair[m.pair1] == nil {
			perPair[m.pair1] = &acc{ownElo: m.elo1}
		}
		if perPair[m.pair2] == nil {
			perPair[m.pair2] = &acc{ownElo: m.elo2}
		}
		perPair[m.pair1].winPcts = append(perPair[m.pair1].winPcts, w1)
		perPair[m.pair1].oppElo = append(perPair[m.pair1].oppElo, m.elo2)
		perPair[m.pair2].winPcts = append(perPair[m.pair2].winPcts, w2)
		perPair[m.pair2].oppElo = append(perPair[m.pair2].oppElo, m.elo1)
	}

	jornadaGaps := make(map[int]float64, len(jornadaGapSum))
	for slot, sum := range jornadaGapSum {
		jornadaGaps[slot] = sum / float64(jornadaCount[slot])
	}

	if len(assigned) == 0 {
		return seasonRunMetrics{jornadaGaps: jornadaGaps, jornadaCount: jornadaCount}
	}

	var maxAbsBias float64
	var winPcts []float64
	for _, a := range perPair {
		if len(a.oppElo) == 0 {
			continue
		}
		oppSum := 0.0
		for _, e := range a.oppElo {
			oppSum += e
		}
		bias := oppSum/float64(len(a.oppElo)) - a.ownElo
		if abs := math.Abs(bias); abs > maxAbsBias {
			maxAbsBias = abs
		}

		wSum := 0.0
		for _, w := range a.winPcts {
			wSum += w
		}
		winPcts = append(winPcts, wSum/float64(len(a.winPcts))*100)
	}

	mean := 0.0
	for _, w := range winPcts {
		mean += w
	}
	mean /= float64(len(winPcts))
	var sqDiff float64
	for _, w := range winPcts {
		d := w - mean
		sqDiff += d * d
	}
	sd := math.Sqrt(sqDiff / float64(len(winPcts)))

	return seasonRunMetrics{
		maxAbsBias:   maxAbsBias,
		sdWinPct:     sd,
		meanGap:      gapSum / float64(len(assigned)),
		gap200Plus:   gap200,
		matchCount:   len(assigned),
		jornadaGaps:  jornadaGaps,
		jornadaCount: jornadaCount,
	}
}

// runFairnessSeason draws *fairnessSeasonRuns full seasons for the given
// level mix, each reaching target_matches per pair via GenerateInitial
// Assignments + TopUpAssignments, with outcomes drawn from the Elo win
// probability at assignment time (a Bernoulli coin flip, not a separate
// true-skill model — measures the algorithm against the same Elo it
// optimizes on).
func runFairnessSeason(t *testing.T, levels []string) []seasonRunMetrics {
	t.Helper()
	app := newTestApp(t)
	svc := New(app, nil)
	comp, staticElo := setupFairnessCompetition(t, app, levels)
	rng := rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))

	results := make([]seasonRunMetrics, *fairnessSeasonRuns)
	for i := range *fairnessSeasonRuns {
		results[i] = playOneSeason(t, app, svc, comp, rng)
		deleteMatches(t, app, comp.Id)
		require.Len(t, staticElo, fairnessPairs) // sanity: setup didn't mutate
	}
	return results
}

// playOneSeason drives one leveled competition from GenerateInitial
// Assignments through TopUpAssignments until every pair reaches
// target_matches, finalizing each match from a Bernoulli draw on the Elo
// win probability at assignment time, and returns the season's metrics.
func playOneSeason(t *testing.T, app core.App, svc *Service, comp *core.Record, rng *rand.Rand) seasonRunMetrics {
	t.Helper()
	now := fairnessStart.Add(-24 * time.Hour)

	n, err := svc.GenerateInitialAssignments(app, comp, now)
	require.NoError(t, err)
	require.Greater(t, n, 0)

	var assigned []assignedMatch
	clock := fairnessStart

	recordAssigned := func(matches []*core.Record) {
		ratings, err := Ratings(app, comp)
		require.NoError(t, err)
		for _, m := range matches {
			p1, p2 := m.GetString("pair1"), m.GetString("pair2")
			assigned = append(assigned, assignedMatch{
				pair1: p1, pair2: p2,
				elo1: ratings[p1], elo2: ratings[p2],
				slot: m.GetInt("slot"),
			})
		}
	}

	pending, err := app.FindRecordsByFilter("matches", "competition = {:c} && status = 'pending'", "", 0, 0, map[string]any{"c": comp.Id})
	require.NoError(t, err)
	recordAssigned(pending)

	for {
		pend, err := app.FindRecordsByFilter("matches", "competition = {:c} && status = 'pending'", "", 0, 0, map[string]any{"c": comp.Id})
		require.NoError(t, err)
		if len(pend) == 0 {
			break
		}

		m := pend[rng.IntN(len(pend))]
		ratings, err := Ratings(app, comp)
		require.NoError(t, err)
		p1, p2 := m.GetString("pair1"), m.GetString("pair2")
		w1 := expectedWinPct(ratings[p1], ratings[p2])

		clock = clock.Add(time.Minute)
		score := "6-0 6-0"
		if rng.Float64() >= w1 {
			score = "0-6 0-6"
		}
		winner, err := DetermineWinner(m, score)
		require.NoError(t, err)
		m.Set("scores", score)
		m.Set("winner", winner)
		m.Set("status", "final")
		m.Set("finalized_at", clock.Format("2006-01-02 15:04:05.000Z"))
		require.NoError(t, app.Save(m))

		created, err := svc.TopUpAssignments(comp.Id, clock, Pairing{A: p1, B: p2})
		require.NoError(t, err)
		recordAssigned(created)
	}

	return measureSeason(assigned)
}

// fmtSeasonDist renders a fairnessDist as a table row segment, matching
// fmtDist's layout for the initial-batch table.
func fmtSeasonDist(name string, d fairnessDist) string {
	return fmt.Sprintf("%-14s mean=%7.2f  p50=%7.2f  p95=%7.2f  worst=%7.2f", name, d.mean, d.p50, d.p95, d.worst)
}

// TestFairness_FullSeason measures the same seeded-Elo fairness over a
// complete season (every pair reaches target_matches, not just the
// opening batch), plus per-Jornada mean gap to detect front-loading —
// whether early rounds are systematically less even than later ones.
// Opt-in and skipped by default — run with -fairness.seasons=N.
func TestFairness_FullSeason(t *testing.T) {
	if *fairnessSeasonRuns == 0 {
		t.Skip("opt-in: run with -fairness.seasons=N")
	}

	t.Logf("full-season fairness harness: %d seasons, scenario mix, %d pairs, target=%d, open=%d",
		*fairnessSeasonRuns, fairnessPairs, fairnessTarget, fairnessOpen)

	runs := runFairnessSeason(t, scenarioLevels)

	maxBias := make([]float64, len(runs))
	sdW := make([]float64, len(runs))
	meanGap := make([]float64, len(runs))
	gap200 := make([]float64, len(runs))
	for i, r := range runs {
		maxBias[i] = r.maxAbsBias
		sdW[i] = r.sdWinPct
		meanGap[i] = r.meanGap
		gap200[i] = float64(r.gap200Plus)
	}

	t.Logf("\n=== scenario mix — full season (%d runs) ===\n%s\n%s\n%s\n%s",
		len(runs),
		fmtSeasonDist("max |bias|", summarize(maxBias, true)),
		fmtSeasonDist("SD win%", summarize(sdW, true)),
		fmtSeasonDist("mean |gap|", summarize(meanGap, true)),
		fmtSeasonDist("gap>=200 ct", summarize(gap200, true)),
	)

	// Per-Jornada mean gap, averaged across runs, to detect front-loading
	// (early rounds systematically less even than later ones).
	jornadaSums := make(map[int]float64)
	jornadaCounts := make(map[int]int)
	for _, r := range runs {
		for slot, gap := range r.jornadaGaps {
			jornadaSums[slot] += gap
			jornadaCounts[slot]++
		}
	}
	slots := make([]int, 0, len(jornadaSums))
	for slot := range jornadaSums {
		slots = append(slots, slot)
	}
	sort.Ints(slots)
	var jornadaReport string
	for _, slot := range slots {
		jornadaReport += fmt.Sprintf("  Jornada %2d: mean gap %7.2f (n=%d runs)\n",
			slot, jornadaSums[slot]/float64(jornadaCounts[slot]), jornadaCounts[slot])
	}
	t.Logf("per-Jornada mean gap (scenario mix):\n%s", jornadaReport)
}
