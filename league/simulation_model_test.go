package league

import (
	"fmt"
	"math"
	"math/rand/v2"
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
	rhoSum     float64
	wrongCat   int
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
	rho := spearman(order)
	a.rhoSum += rho
	wrong := 0
	for k, trueIdx := range order {
		if k < 8 && trueIdx >= 8 {
			wrong++
		}
	}
	a.wrongCat += 2 * wrong
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
	return fmt.Sprintf("| %s | %.2f%% | %d | %d | %.1f%% | %.1f | %.1f | %.1f%% | %.2f |",
		name, exactPct, a.over, a.maxPending, evenPct, blowPerSeason, tvbPerSeason, reliability, wrongPerSeason)
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
// memory (no DB), keeping simTarget rounds chosen at random.
// Ranking columns are unavailable (random tiebreak) so only pairing metrics are collected.
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
	maxPend := 0

	for _, ri := range perm[:simTarget] {
		round := allRounds[ri]
		maxPend = max(maxPend, 1) // each pair has 1 pending per round
		for _, pair := range round {
			i, j := pair[0], pair[1]
			acc.addMatch(model, i, j)
			played[i]++
			played[j]++
		}
	}
	acc.addSeason(played, maxPend)
	// No ranking data for random baseline — wrongCat and rhoSum not updated.
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
