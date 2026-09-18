package league

import (
	"math"
	"sort"

	"github.com/pocketbase/pocketbase/core"
)

// Pairing is an ordered pair of pair IDs assigned to play each other.
type Pairing struct {
	A, B string
}

// IsLeveled reports whether the competition assigns opponents by rating
// proximity instead of generating a full round-robin.
func IsLeveled(comp *core.Record) bool {
	target := comp.GetInt("target_matches")
	pairs := comp.GetStringSlice("pairs")
	return !IsPlayoff(comp) && target > 0 && target < len(pairs)-1
}

// OpenAssignments returns the number of pending matches the system keeps each
// pair on, defaulting to 3 when unset or zero.
func OpenAssignments(comp *core.Record) int {
	if v := comp.GetInt("open_assignments"); v > 0 {
		return v
	}
	return 3
}

// leveledState holds the in-memory snapshot used during one matchmaking run.
type leveledState struct {
	target   int
	open     int
	comfort  int // ceil(target/2)
	pairs    []string
	played   map[string]int
	pending  map[string]int
	met      map[string]map[string]struct{}
	position map[string]int // 0-based index in rating order
}

func (st *leveledState) load(p string) int {
	return st.played[p] + st.pending[p]
}

// wants reports whether p needs a new assignment now.
func (st *leveledState) wants(p string) bool {
	return st.pending[p] < st.open && st.load(p) < st.target
}

// eligible reports whether q is a valid opponent candidate for p.
func (st *leveledState) eligible(p, q string) bool {
	if q == p {
		return false
	}
	if _, met := st.met[p][q]; met {
		return false
	}
	if st.load(q) >= st.target {
		return false
	}
	if st.pending[q] > st.open {
		return false
	}
	return true
}

// chooseOpponent returns the best eligible opponent for p, or "" if none exists.
// Candidates inside the comfort zone are shuffled (random order); candidates
// outside are sorted nearest-first, then by lower load, then by better position.
// The first candidate that passes the completion check wins.
func (svc *Service) chooseOpponent(st *leveledState, p string) string {
	posP := st.position[p]

	type candidate struct {
		id       string
		dist     int
		load     int
		position int
	}

	var inside, outside []candidate
	for _, q := range st.pairs {
		if !st.eligible(p, q) {
			continue
		}
		d := st.position[q] - posP
		if d < 0 {
			d = -d
		}
		c := candidate{id: q, dist: d, load: st.load(q), position: st.position[q]}
		if d <= st.comfort {
			inside = append(inside, c)
		} else {
			outside = append(outside, c)
		}
	}

	// Shuffle inside zone for randomness.
	svc.shuffle(len(inside), func(i, j int) { inside[i], inside[j] = inside[j], inside[i] })

	// Sort outside zone: nearest first, then lower load, then better (lower) position.
	sort.Slice(outside, func(i, j int) bool {
		a, b := outside[i], outside[j]
		if a.dist != b.dist {
			return a.dist < b.dist
		}
		if a.load != b.load {
			return a.load < b.load
		}
		return a.position < b.position
	})

	// Build need map for completion check (current state before adding p–q).
	need := make(map[string]int, len(st.pairs))
	for _, id := range st.pairs {
		need[id] = st.target - st.load(id)
	}
	slack := minSlack(need, st.met)

	// Try inside candidates first, then outside.
	for _, group := range [][]candidate{inside, outside} {
		for _, c := range group {
			// Simulate adding p–q.
			needAfter := make(map[string]int, len(need))
			for k, v := range need {
				needAfter[k] = v
			}
			needAfter[p]--
			needAfter[c.id]--

			metAfter := cloneMet(st.met)
			addMet(metAfter, p, c.id)

			if completable(needAfter, metAfter, slack, candidateTries) {
				return c.id
			}
		}
	}
	return ""
}

const (
	baselineTries  = 100
	candidateTries = 30
)

// completable does a randomized greedy check: can we build a valid schedule
// for the remaining need, given met, allowing up to slack unfilled slots?
func completable(need map[string]int, met map[string]map[string]struct{}, slack, tries int) bool {
	for pass := range tries {
		_ = pass
		if tryGreedy(need, met, slack) {
			return true
		}
	}
	return false
}

// tryGreedy attempts one greedy pass: serve the most-constrained pair first.
func tryGreedy(origNeed map[string]int, origMet map[string]map[string]struct{}, slack int) bool {
	need := make(map[string]int, len(origNeed))
	for k, v := range origNeed {
		need[k] = v
	}
	met := cloneMet(origMet)

	missed := 0
	for {
		// Find pair with positive need.
		pairs := pairsWithNeed(need)
		if len(pairs) == 0 {
			break
		}
		// Pick the one with fewest eligible opponents (most constrained).
		sort.Slice(pairs, func(i, j int) bool {
			oi := countEligible(pairs[i], need, met)
			oj := countEligible(pairs[j], need, met)
			if oi != oj {
				return oi < oj
			}
			return pairs[i] < pairs[j]
		})
		p := pairs[0]

		// Find eligible opponents for p, sorted by fewest options.
		candidates := eligibleCandidates(p, need, met)
		if len(candidates) == 0 {
			missed++
			// p can't be filled; count as 1 missed slot (pair can't complete need[p]).
			need[p] = 0
			if missed > slack {
				return false
			}
			continue
		}
		sort.Slice(candidates, func(i, j int) bool {
			oi := countEligible(candidates[i], need, met)
			oj := countEligible(candidates[j], need, met)
			if oi != oj {
				return oi < oj
			}
			return candidates[i] < candidates[j]
		})
		q := candidates[0]
		need[p]--
		need[q]--
		addMet(met, p, q)
	}
	return missed <= slack
}

// minSlack finds the minimum slack for which completable returns true.
// Recipe §3.4: try 0 (or 1 when total need is odd), then +2.
func minSlack(need map[string]int, met map[string]map[string]struct{}) int {
	max := totalNeed(need)
	start := 0
	if max%2 != 0 {
		start = 1
	}
	for s := start; s <= max; s += 2 {
		if completable(need, met, s, baselineTries) {
			return s
		}
	}
	return max
}

// plan runs the fill loop (recipe §3.5) and returns the pairings to create.
func plan(svc *Service, st *leveledState) []Pairing {
	var result []Pairing
	skipped := map[string]bool{}

	for {
		// Find the pair with wants() not yet skipped; lowest load, then best position.
		p := nextRequester(st, skipped)
		if p == "" {
			break
		}
		q := svc.chooseOpponent(st, p)
		if q == "" {
			skipped[p] = true
			continue
		}
		result = append(result, Pairing{A: p, B: q})
		st.pending[p]++
		st.pending[q]++
		addMet(st.met, p, q)
	}
	return result
}

// nextRequester returns the pair that wants an assignment and has the lowest
// load, then the best (lowest) position. Returns "" when none wants one.
func nextRequester(st *leveledState, skipped map[string]bool) string {
	best := ""
	for _, p := range st.pairs {
		if skipped[p] || !st.wants(p) {
			continue
		}
		if best == "" {
			best = p
			continue
		}
		lp, lb := st.load(p), st.load(best)
		if lp < lb || (lp == lb && st.position[p] < st.position[best]) {
			best = p
		}
	}
	return best
}

// -- helpers ----------------------------------------------------------------

func cloneMet(met map[string]map[string]struct{}) map[string]map[string]struct{} {
	out := make(map[string]map[string]struct{}, len(met))
	for k, v := range met {
		inner := make(map[string]struct{}, len(v))
		for id := range v {
			inner[id] = struct{}{}
		}
		out[k] = inner
	}
	return out
}

func addMet(met map[string]map[string]struct{}, p, q string) {
	if met[p] == nil {
		met[p] = map[string]struct{}{}
	}
	if met[q] == nil {
		met[q] = map[string]struct{}{}
	}
	met[p][q] = struct{}{}
	met[q][p] = struct{}{}
}

func pairsWithNeed(need map[string]int) []string {
	var out []string
	for k, v := range need {
		if v > 0 {
			out = append(out, k)
		}
	}
	return out
}

func countEligible(p string, need map[string]int, met map[string]map[string]struct{}) int {
	count := 0
	for q, n := range need {
		if q == p || n <= 0 {
			continue
		}
		if _, ok := met[p][q]; ok {
			continue
		}
		count++
	}
	return count
}

func eligibleCandidates(p string, need map[string]int, met map[string]map[string]struct{}) []string {
	var out []string
	for q, n := range need {
		if q == p || n <= 0 {
			continue
		}
		if _, ok := met[p][q]; ok {
			continue
		}
		out = append(out, q)
	}
	return out
}

func totalNeed(need map[string]int) int {
	s := 0
	for _, v := range need {
		if v > 0 {
			s += v
		}
	}
	return s
}

// ceilDiv returns ceil(a/b).
func ceilDiv(a, b int) int {
	return int(math.Ceil(float64(a) / float64(b)))
}
