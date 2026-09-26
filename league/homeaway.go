package league

import (
	"math/bits"
	"strconv"
	"strings"
)

// homeQuota returns the home-match range every pair must end the season
// inside: exactly target/2 for an even target, floor or ceil for an odd one.
func homeQuota(target int) (lo, hi int) {
	return target / 2, (target + 1) / 2
}

// hfactor reports whether the remaining schedule can be completed exactly
// (every vertex reaches need == 0 over its avail candidates, no repeat) AND
// oriented so that every vertex takes between homeMin and homeMax of its
// remaining matches at home. Exact backtracking with degree, parity and
// quota pruning, memoized per call; the inputs are not mutated.
func hfactor(need []int, avail []uint32, homeMin, homeMax []int) bool {
	n := len(need)
	st := oriented{
		need:    append([]int(nil), need...),
		avail:   append([]uint32(nil), avail...),
		homeMin: append([]int(nil), homeMin...),
		homeMax: append([]int(nil), homeMax...),
		memo:    map[string]bool{},
		n:       n,
	}
	return st.solve()
}

type oriented struct {
	need, homeMin, homeMax []int
	avail                  []uint32
	memo                   map[string]bool
	stack                  []orientedSnapshot
	n                      int
}

func (o *oriented) solve() bool {
	live, ok := o.normalize()
	if !ok {
		return false
	}
	if live == 0 {
		return true
	}
	for v := range o.need {
		if o.need[v] > 0 && !o.vertexFeasible(v, live) {
			return false
		}
	}
	key := o.key()
	if cached, ok := o.memo[key]; ok {
		return cached
	}
	v := o.pick(live)
	result := o.branch(v)
	o.memo[key] = result
	return result
}

// normalize clamps every live vertex's home range to [0, need], returns the
// live mask, and rejects states whose totals cannot be oriented: odd total
// need, or a home quota sum that the total match count cannot meet.
func (o *oriented) normalize() (live uint32, ok bool) {
	total, sumMin, sumMax := 0, 0, 0
	for v, nd := range o.need {
		// A vertex over its home quota, or one that still owes homes it has
		// no matches left for, sinks the state even when its need is 0.
		if o.homeMax[v] < 0 || o.homeMin[v] > max(nd, 0) {
			return 0, false
		}
		if nd <= 0 {
			continue
		}
		live |= 1 << uint(v)
		total += nd
		o.homeMin[v] = max(o.homeMin[v], 0)
		o.homeMax[v] = min(o.homeMax[v], nd)
		sumMin += o.homeMin[v]
		sumMax += o.homeMax[v]
	}
	if live == 0 {
		return 0, true
	}
	if total%2 != 0 || sumMin > total/2 || total/2 > sumMax {
		return 0, false
	}
	return live, true
}

// vertexFeasible restricts v's avail to live vertices and checks it has
// enough partners: at least need of them, homeMin able to play away and
// need−homeMax able to play home.
func (o *oriented) vertexFeasible(v int, live uint32) bool {
	a := o.avail[v] & live
	o.avail[v] = a
	if bits.OnesCount32(a) < o.need[v] {
		return false
	}
	canAway, canHome := 0, 0
	for u := range o.need {
		if a&(1<<uint(u)) == 0 {
			continue
		}
		if o.need[u]-o.homeMin[u] > 0 {
			canAway++
		}
		if o.homeMax[u] > 0 {
			canHome++
		}
	}
	return canAway >= o.homeMin[v] && canHome >= o.need[v]-o.homeMax[v]
}

// pick returns the live vertex with the least slack (avail − need), ties to
// the larger need.
func (o *oriented) pick(live uint32) int {
	best, bestSlack := -1, 0
	for v := range o.need {
		if live&(1<<uint(v)) == 0 {
			continue
		}
		slack := bits.OnesCount32(o.avail[v]) - o.need[v]
		if best == -1 || slack < bestSlack || (slack == bestSlack && o.need[v] > o.need[best]) {
			best, bestSlack = v, slack
		}
	}
	return best
}

// branch tries every partner subset of v and, within it, every split into
// home and away partners allowed by the quotas.
func (o *oriented) branch(v int) bool {
	nd := o.need[v]
	for _, subset := range subsetsOfSize(o.avail[v], nd) {
		for h := o.homeMin[v]; h <= o.homeMax[v]; h++ {
			for _, homeSet := range subsetsOfSize(subset, h) {
				if o.apply(v, subset, homeSet) {
					if o.solve() {
						o.restore()
						return true
					}
				}
				o.restore()
			}
		}
	}
	return false
}

// apply pairs v with every vertex in subset, v at home against homeSet and
// away against the rest; returns false (leaving state to be restored) when a
// partner cannot take that side.
func (o *oriented) apply(v int, subset, homeSet uint32) bool {
	o.save()
	for u := range o.need {
		if subset&(1<<uint(u)) == 0 {
			continue
		}
		if homeSet&(1<<uint(u)) != 0 { // u plays away
			if o.need[u]-o.homeMin[u] <= 0 {
				return false
			}
		} else { // u plays home
			if o.homeMax[u] <= 0 {
				return false
			}
			o.homeMin[u]--
			o.homeMax[u]--
		}
		o.need[u]--
		o.avail[u] &^= 1 << uint(v)
	}
	o.need[v] = 0
	o.homeMin[v], o.homeMax[v] = 0, 0
	o.avail[v] = 0
	return true
}

type orientedSnapshot struct {
	need, homeMin, homeMax []int
	avail                  []uint32
}

func (o *oriented) save() {
	o.stack = append(o.stack, orientedSnapshot{
		need:    append([]int(nil), o.need...),
		homeMin: append([]int(nil), o.homeMin...),
		homeMax: append([]int(nil), o.homeMax...),
		avail:   append([]uint32(nil), o.avail...),
	})
}

func (o *oriented) restore() {
	s := o.stack[len(o.stack)-1]
	o.stack = o.stack[:len(o.stack)-1]
	copy(o.need, s.need)
	copy(o.homeMin, s.homeMin)
	copy(o.homeMax, s.homeMax)
	copy(o.avail, s.avail)
}

func (o *oriented) key() string {
	var b strings.Builder
	for v := range o.need {
		b.WriteString(strconv.Itoa(o.need[v]))
		b.WriteByte(',')
		b.WriteString(strconv.Itoa(o.homeMin[v]))
		b.WriteByte(',')
		b.WriteString(strconv.Itoa(o.homeMax[v]))
		b.WriteByte(',')
		b.WriteString(strconv.FormatUint(uint64(o.avail[v]), 16))
		b.WriteByte(';')
	}
	return b.String()
}
