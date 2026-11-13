package league

import (
	"math/rand"
	"testing"
	"time"
)

// brutePairings is the reference: try every subset of edges.
func brutePairings(need []int, avail []uint32) int {
	type e struct{ u, v int }
	var edges []e
	for u := range need {
		for v := u + 1; v < len(need); v++ {
			if avail[u]&(1<<uint(v)) != 0 && avail[v]&(1<<uint(u)) != 0 {
				edges = append(edges, e{u, v})
			}
		}
	}
	best := 0
	for mask := 0; mask < 1<<len(edges); mask++ {
		deg := make([]int, len(need))
		cnt, ok := 0, true
		for i, ed := range edges {
			if mask&(1<<i) != 0 {
				deg[ed.u]++
				deg[ed.v]++
				cnt++
			}
		}
		for v := range need {
			if deg[v] > need[v] {
				ok = false
			}
		}
		if ok && cnt > best {
			best = cnt
		}
	}
	return best
}

func TestMaxPairings_MatchesBruteForce(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for it := 0; it < 1500; it++ {
		n := 2 + rng.Intn(6)
		need := make([]int, n)
		avail := make([]uint32, n)
		for v := range need {
			need[v] = rng.Intn(4)
		}
		randomEdges(rng, avail)
		if got, want := maxPairings(need, avail), brutePairings(need, avail); got != want {
			t.Fatalf("need=%v avail=%v: got %d want %d", need, avail, got, want)
		}
	}
}

// TestMaxPairings_Blossoms pins graphs whose augmenting paths need blossom
// contraction: an odd cycle with a tail, and two triangles joined by a bridge.
func TestMaxPairings_Blossoms(t *testing.T) {
	link := func(n int, edges [][2]int) []uint32 {
		avail := make([]uint32, n)
		for _, e := range edges {
			avail[e[0]] |= 1 << uint(e[1])
			avail[e[1]] |= 1 << uint(e[0])
		}
		return avail
	}
	cases := []struct {
		name  string
		need  []int
		edges [][2]int
		want  int
	}{
		{"5-cycle with tail", []int{1, 1, 1, 1, 1, 1}, [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 4}, {4, 0}, {4, 5}}, 3},
		{"two triangles and a bridge", []int{1, 1, 1, 1, 1, 1}, [][2]int{{0, 1}, {1, 2}, {2, 0}, {3, 4}, {4, 5}, {5, 3}, {2, 3}}, 3},
		{"triangle, need 2 each", []int{2, 2, 2}, [][2]int{{0, 1}, {1, 2}, {2, 0}}, 3},
		{"7-cycle", []int{1, 1, 1, 1, 1, 1, 1}, [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 4}, {4, 5}, {5, 6}, {6, 0}}, 3},
	}
	for _, c := range cases {
		avail := link(len(c.need), c.edges)
		if got := maxPairings(c.need, avail); got != c.want || got != brutePairings(c.need, avail) {
			t.Errorf("%s: got %d want %d (brute %d)", c.name, got, c.want, brutePairings(c.need, avail))
		}
	}
}

func TestMaxPairings_LargeIsFast(t *testing.T) {
	n := 20
	need := make([]int, n)
	avail := make([]uint32, n)
	for u := 0; u < n; u++ {
		need[u] = 3
		for v := 0; v < n; v++ {
			if v != u {
				avail[u] |= 1 << uint(v)
			}
		}
	}
	start := time.Now()
	if got := maxPairings(need, avail); got != 30 {
		t.Fatalf("got %d want 30", got)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("too slow: %v", d)
	}
}

// randomEdges links each pair of vertices with probability 2/3.
func randomEdges(rng *rand.Rand, avail []uint32) {
	for u := range avail {
		for v := u + 1; v < len(avail); v++ {
			if rng.Intn(3) > 0 {
				avail[u] |= 1 << uint(v)
				avail[v] |= 1 << uint(u)
			}
		}
	}
}
