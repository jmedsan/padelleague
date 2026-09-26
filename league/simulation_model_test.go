package league

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
)

// simSkills returns the fixed 16-pair true skill values used in the simulation.
// Scale 400: 0 vs -650 → 97.7%, -90 vs -560 → 93.7%, 40 apart → 55.7%.
func simSkills() []float64 {
	return []float64{0, -30, -60, -90, -200, -240, -280, -320, -360, -400, -440, -480, -560, -590, -620, -650}
}

// matchModel pre-computes match and set win probabilities for all 16×16 pairs.
type matchModel struct {
	p        [simPairs][simPairs]float64
	q        [simPairs][simPairs]float64
	loserCum [simPairs][simPairs][7]float64
}

func newMatchModel(skills []float64) *matchModel {
	m := &matchModel{}
	for i := range simPairs {
		for j := range simPairs {
			if i == j {
				continue
			}
			p := 1.0 / (1.0 + math.Pow(10, (skills[j]-skills[i])/simScale))
			m.p[i][j] = p
			m.q[i][j] = setProb(p)
		}
	}
	for w := range simPairs {
		for l := range simPairs {
			if w == l {
				continue
			}
			d := m.q[w][l]
			mu := 4.2 - 5.5*(d-0.5)
			var acc float64
			for g := range 7 {
				diff := float64(g) - mu
				acc += math.Exp(-(diff * diff) / (2 * 1.8 * 1.8))
				m.loserCum[w][l][g] = acc
			}
		}
	}
	return m
}

// play simulates a best-of-3 match between pairs i and j (i is pair1).
// Returns a score string like "6-3 4-6 7-5".
func (m *matchModel) play(i, j int, rng *rand.Rand) string {
	q := m.q[i][j]
	si, sj := 0, 0
	var sets []string
	for si < 2 && sj < 2 {
		r := rng.Float64()
		if r < q {
			lg := loserGames(m.loserCum[i][j], rng)
			wg := 6
			if lg > 4 {
				wg = 7
			}
			sets = append(sets, fmt.Sprintf("%d-%d", wg, lg))
			si++
		} else {
			lg := loserGames(m.loserCum[j][i], rng)
			wg := 6
			if lg > 4 {
				wg = 7
			}
			sets = append(sets, fmt.Sprintf("%d-%d", lg, wg))
			sj++
		}
	}
	score := sets[0]
	for _, s := range sets[1:] {
		score += " " + s
	}
	return score
}

// favorite returns the match-win probability of the stronger pair.
func (m *matchModel) favorite(i, j int) float64 {
	if m.p[i][j] >= m.p[j][i] {
		return m.p[i][j]
	}
	return m.p[j][i]
}

// setProb solves q²(3−2q) = p via 60-step bisection.
func setProb(p float64) float64 {
	lo, hi := 0.0, 1.0
	for range 60 {
		mid := (lo + hi) / 2
		if mid*mid*(3-2*mid) < p {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}

// loserGames samples loser games using the pre-computed cumulative weights.
func loserGames(cum [7]float64, rng *rand.Rand) int {
	total := cum[6]
	r := rng.Float64() * total
	for g, c := range cum {
		if r <= c {
			return g
		}
	}
	return 6
}

// simMetrics accumulates season-level statistics across many seasons.
type simMetrics struct {
	seasons    int
	matches    int
	exact      int // seasons where min==max==target
	over       int // seasons where max > target
	maxPending int // max pending per pair seen across all steps
	evenSum    float64
	blowouts   int
	topVBottom int
	rhoSum     float64 // plain-points ranking (the public table)
	wrongCat   int
	rhoSumOld  float64 // ranking by the removed schedule-strength adjustment, for reference
	wrongOld   int
	rhoSumElo  float64 // plain points, ties broken by the hidden Elo (internal-only option)
	wrongElo   int
	eloSeasons int
	pendHist   [6]int // pair-seasons whose max pending was ≤3, 4, 5, 6, 7, 8+
	pendSteps  int    // pair-steps observed (steps × pairs)
	pendHigh   int    // pair-steps with ≥5 pending
	balanced   int    // seasons where every pair ended on the exact home/away split
	balSeasons int
}

// addPending records the pending-load distribution and home/away outcome of
// one DB-backed season.
func (a *simMetrics) addPending(pairMax map[string]int, steps, highSum int, balanced bool) {
	for _, v := range pairMax {
		switch {
		case v <= 3:
			a.pendHist[0]++
		case v >= 8:
			a.pendHist[5]++
		default:
			a.pendHist[v-3]++
		}
	}
	a.pendSteps += steps * simPairs
	a.pendHigh += highSum
	a.balSeasons++
	if balanced {
		a.balanced++
	}
}

func (a *simMetrics) addMatch(m *matchModel, i, j int) {
	a.matches++
	f := m.favorite(i, j)
	a.evenSum += 1 - 2*(f-0.5)
	if f > simBlowout {
		a.blowouts++
	}
	ti, tj := i < 4, j < 4
	bi, bj := i >= 12, j >= 12
	if (ti && bj) || (tj && bi) {
		a.topVBottom++
	}
}

func (a *simMetrics) addSeason(played []int, maxPending int) {
	a.seasons++
	mn, mx := played[0], played[0]
	for _, v := range played[1:] {
		if v < mn {
			mn = v
		}
		if v > mx {
			mx = v
		}
	}
	if mn == simTarget && mx == simTarget {
		a.exact++
	}
	if mx > simTarget {
		a.over++
	}
	if maxPending > a.maxPending {
		a.maxPending = maxPending
	}
}

func (a *simMetrics) addRanking(order []int) {
	rho, wrong := rankingQuality(order)
	a.rhoSum += rho
	a.wrongCat += wrong
}

// addRankingOld records the ranking produced by the removed
// schedule-strength adjustment (recipe §4 before 2026-09-26), kept only to
// compare the public plain-points table against it.
func (a *simMetrics) addRankingOld(order []int) {
	rho, wrong := rankingQuality(order)
	a.rhoSumOld += rho
	a.wrongOld += wrong
}

// addRankingElo records the plain-points ranking with equal points broken
// by the hidden Elo — a candidate internal-only tiebreak.
func (a *simMetrics) addRankingElo(order []int) {
	rho, wrong := rankingQuality(order)
	a.rhoSumElo += rho
	a.wrongElo += wrong
	a.eloSeasons++
}

// rankingQuality returns the Spearman rho of order against the true order
// and twice the number of bottom-half pairs ranked in the top half.
func rankingQuality(order []int) (rho float64, wrongCat int) {
	wrong := 0
	for k, trueIdx := range order {
		if k < 8 && trueIdx >= 8 {
			wrong++
		}
	}
	return spearman(order), 2 * wrong
}

// oldAdjustedOrder ranks pairs by the removed formula: points + 3 × played ×
// 1.5 × (SOS − 0.5), SOS = mean win rate of the opponents faced; ties keep
// the plain-points order. wins/played are per pair; opps lists each pair's
// opponents, one entry per match; plain is the plain-points ranking.
func oldAdjustedOrder(wins, played []int, opps [][]int, plain []int) []int {
	n := len(wins)
	score := make([]float64, n)
	for p := range n {
		var sos float64
		for _, o := range opps[p] {
			if played[o] > 0 {
				sos += float64(wins[o]) / float64(played[o])
			}
		}
		if len(opps[p]) > 0 {
			sos /= float64(len(opps[p]))
		}
		adj := 3.0 * float64(played[p]) * 1.5 * (sos - 0.5)
		score[p] = float64(3*wins[p]) + math.Round(adj*10)/10
	}
	order := append([]int(nil), plain...)
	sort.SliceStable(order, func(i, j int) bool { return score[order[i]] > score[order[j]] })
	return order
}

// row returns a markdown table row for the given variant name.
func (a *simMetrics) row(name string) string {
	s := a.seasons
	if s == 0 {
		return fmt.Sprintf("| %s | — | — | — | — | — | — | — | — |", name)
	}
	m := a.matches
	exactPct := 100.0 * float64(a.exact) / float64(s)
	evenPct := 0.0
	if m > 0 {
		evenPct = 100.0 * a.evenSum / float64(m)
	}
	blowPerSeason := float64(a.blowouts) / float64(s)
	tvbPerSeason := float64(a.topVBottom) / float64(s)
	reliability := 100.0 * (a.rhoSum/float64(s) + 1) / 2
	wrongPerSeason := float64(a.wrongCat) / float64(s)
	reliabilityOld := 100.0 * (a.rhoSumOld/float64(s) + 1) / 2
	wrongOldPerSeason := float64(a.wrongOld) / float64(s)
	eloCols := "— | —"
	if a.eloSeasons > 0 {
		es := float64(a.eloSeasons)
		eloCols = fmt.Sprintf("%.1f%% | %.2f", 100.0*(a.rhoSumElo/es+1)/2, float64(a.wrongElo)/es)
	}
	pendCols := "— | — | —"
	if a.balSeasons > 0 {
		h := a.pendHist
		pendCols = fmt.Sprintf("%d/%d/%d/%d/%d/%d | %.1f%% | %.0f%%",
			h[0], h[1], h[2], h[3], h[4], h[5],
			100.0*float64(a.pendHigh)/float64(max(a.pendSteps, 1)),
			100.0*float64(a.balanced)/float64(a.balSeasons))
	}
	return fmt.Sprintf("| %s | %.2f%% | %d | %d | %.1f%% | %.1f | %.1f | %.1f%% | %.2f | %.1f%% | %.2f | %s | %s |",
		name, exactPct, a.over, a.maxPending, evenPct, blowPerSeason, tvbPerSeason,
		reliability, wrongPerSeason, reliabilityOld, wrongOldPerSeason, eloCols, pendCols)
}

// spearman computes the Spearman rank correlation between the observed ranking
// and the true order. order[k] is the true index of the pair ranked k-th.
func spearman(order []int) float64 {
	n := len(order)
	sumSq := 0.0
	for k, trueIdx := range order {
		d := float64(k - trueIdx)
		sumSq += d * d
	}
	return 1 - 6*sumSq/float64(n*(n*n-1))
}

// noisySeedLevels buckets pairs into the 8 skill levels by noisy rank order
// (strongest first), for setting pairs.level directly — the production
// mechanism that replaced seed_pairs ordering.
func noisySeedLevels(pairs []string, trueIdx map[string]int, sd float64, rng *rand.Rand) map[string]string {
	ranked := noisySeed(pairs, trueIdx, sd, rng)
	levels := make(map[string]string, len(ranked))
	n := len(Levels) - 1 // exclude "unranked" from the bucket cycle
	for i, id := range ranked {
		bucket := i * n / len(ranked)
		levels[id] = Levels[bucket].Key
	}
	return levels
}

// noisySeed returns pair IDs sorted by trueIndex + gauss(0, sd).
func noisySeed(pairs []string, trueIdx map[string]int, sd float64, rng *rand.Rand) []string {
	type entry struct {
		id    string
		score float64
	}
	entries := make([]entry, len(pairs))
	for i, id := range pairs {
		entries[i] = entry{id: id, score: float64(trueIdx[id]) + rng.NormFloat64()*sd}
	}
	// sort ascending by score (lower score = better rank = first in seed)
	for i := range entries {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].score < entries[i].score {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
	result := make([]string, len(pairs))
	for i, e := range entries {
		result[i] = e.id
	}
	return result
}

// runRandomBaseline simulates target rounds of circle-method round-robin in
// memory (no DB), keeping simTarget rounds chosen at random. Results are
// drawn from the match model and ranked by plain points (ties random, since
// no sets are simulated), plus the removed adjustment for reference.
func runRandomBaseline(acc *simMetrics, model *matchModel, rng *rand.Rand) {
	// Build 15 rounds via circle method.
	teams := make([]int, simPairs)
	for i := range teams {
		teams[i] = i
	}
	allRounds := circleRounds(teams)

	// Keep simTarget rounds chosen at random (without replacement).
	perm := rng.Perm(len(allRounds))
	played := make([]int, simPairs)
	wins := make([]int, simPairs)
	opps := make([][]int, simPairs)
	maxPend := 0

	for _, ri := range perm[:simTarget] {
		round := allRounds[ri]
		maxPend = max(maxPend, 1) // each pair has 1 pending per round
		for _, pair := range round {
			i, j := pair[0], pair[1]
			acc.addMatch(model, i, j)
			played[i]++
			played[j]++
			opps[i] = append(opps[i], j)
			opps[j] = append(opps[j], i)
			if rng.Float64() < model.p[i][j] {
				wins[i]++
			} else {
				wins[j]++
			}
		}
	}
	acc.addSeason(played, maxPend)

	plain := rng.Perm(simPairs) // random tiebreak among equal points
	sort.SliceStable(plain, func(a, b int) bool { return wins[plain[a]] > wins[plain[b]] })
	acc.addRanking(plain)
	acc.addRankingOld(oldAdjustedOrder(wins, played, opps, plain))
}

// circleRounds returns the 15 rounds of a circle-method round-robin for 16 teams.
func circleRounds(teams []int) [][8][2]int {
	t := make([]int, len(teams))
	copy(t, teams)
	n := len(t)
	rounds := make([][8][2]int, n-1)
	for r := range n - 1 {
		for k := range n / 2 {
			rounds[r][k] = [2]int{t[k], t[n-1-k]}
		}
		// rotate: fix t[0], rotate the rest
		last := t[n-1]
		copy(t[2:], t[1:n-1])
		t[1] = last
	}
	return rounds
}
