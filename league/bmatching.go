package league

import "math/bits"

// maxPairings returns the largest number of new matches that can still be
// scheduled: the maximum b-matching where vertex v takes at most need[v]
// edges, all from avail (the unmet pairs), each edge used once. Unlike ffactor
// it allows slack, so it measures how many matches a stuck schedule can still
// place. It runs in polynomial time: the b-matching becomes a plain maximum
// matching on a gadget graph, solved with Edmonds' blossom algorithm.
//
// Gadget: each vertex v becomes min(need, free neighbors) copies; each usable
// edge (u,v) becomes two nodes eu, ev joined to each other, eu joined to every
// copy of u and ev to every copy of v. Matching eu-ev alone scores 1 per edge;
// using the edge in the b-matching scores 2 (eu and ev each take a copy), so
// the b-matching size is the maximum matching minus the edge count.
func maxPairings(need []int, avail []uint32) int {
	copies, nodes := vertexCopies(need, avail)
	edges := usableEdges(avail, copies)
	if len(edges) == 0 {
		return 0
	}
	g := newBlossom(nodes + 2*len(edges))
	for i, e := range edges {
		eu, ev := nodes+2*i, nodes+2*i+1
		g.addEdge(eu, ev)
		for _, c := range copies[e[0]] {
			g.addEdge(eu, c)
		}
		for _, c := range copies[e[1]] {
			g.addEdge(ev, c)
		}
	}
	return g.maxMatching() - len(edges)
}

// vertexCopies numbers the gadget copies of each vertex and returns the
// total node count.
func vertexCopies(need []int, avail []uint32) ([][]int, int) {
	copies := make([][]int, len(need))
	nodes := 0
	for v := range need {
		if need[v] <= 0 {
			continue
		}
		for range min(need[v], bits.OnesCount32(avail[v])) {
			copies[v] = append(copies[v], nodes)
			nodes++
		}
	}
	return copies, nodes
}

// usableEdges lists the unmet pairs whose two ends both have copies.
func usableEdges(avail []uint32, copies [][]int) [][2]int {
	var edges [][2]int
	for u := range avail {
		for v := u + 1; v < len(avail); v++ {
			both := avail[u]&(1<<uint(v)) != 0 && avail[v]&(1<<uint(u)) != 0
			if both && len(copies[u]) > 0 && len(copies[v]) > 0 {
				edges = append(edges, [2]int{u, v})
			}
		}
	}
	return edges
}

// blossom is Edmonds' maximum matching for general graphs, O(V^3).
type blossom struct {
	n      int
	adj    [][]int
	match  []int
	parent []int
	base   []int
	queue  []int
	used   []bool
	marked []bool
}

func newBlossom(n int) *blossom {
	return &blossom{n: n, adj: make([][]int, n), match: fill(n, -1), parent: make([]int, n), base: make([]int, n), used: make([]bool, n), marked: make([]bool, n)}
}

func fill(n, v int) []int {
	s := make([]int, n)
	for i := range s {
		s[i] = v
	}
	return s
}

func (b *blossom) addEdge(u, v int) {
	b.adj[u] = append(b.adj[u], v)
	b.adj[v] = append(b.adj[v], u)
}

func (b *blossom) lca(a, c int) int {
	seen := make([]bool, b.n)
	for {
		a = b.base[a]
		seen[a] = true
		if b.match[a] == -1 {
			break
		}
		a = b.parent[b.match[a]]
	}
	for {
		c = b.base[c]
		if seen[c] {
			return c
		}
		c = b.parent[b.match[c]]
	}
}

func (b *blossom) markPath(v, base, child int) {
	for b.base[v] != base {
		b.marked[b.base[v]] = true
		b.marked[b.base[b.match[v]]] = true
		b.parent[v] = child
		child = b.match[v]
		v = b.parent[b.match[v]]
	}
}

// contract merges the blossom closed by edge v-to into its base.
func (b *blossom) contract(v, to int) {
	cur := b.lca(v, to)
	for i := range b.marked {
		b.marked[i] = false
	}
	b.markPath(v, cur, to)
	b.markPath(to, cur, v)
	for i := 0; i < b.n; i++ {
		if !b.marked[b.base[i]] {
			continue
		}
		b.base[i] = cur
		if !b.used[i] {
			b.used[i] = true
			b.queue = append(b.queue, i)
		}
	}
}

// closesBlossom reports whether edge v-to joins two even vertices.
func (b *blossom) closesBlossom(to, root int) bool {
	return to == root || (b.match[to] != -1 && b.parent[b.match[to]] != -1)
}

// extend grows the alternating tree through the unvisited vertex to. It
// returns true when to is free, i.e. an augmenting path ends there.
func (b *blossom) extend(v, to int) bool {
	b.parent[to] = v
	if b.match[to] == -1 {
		return true
	}
	b.used[b.match[to]] = true
	b.queue = append(b.queue, b.match[to])
	return false
}

// findPath returns the end of an augmenting path from root, or -1.
func (b *blossom) findPath(root int) int {
	for i := 0; i < b.n; i++ {
		b.used[i] = false
		b.parent[i] = -1
		b.base[i] = i
	}
	b.used[root] = true
	b.queue = append(b.queue[:0], root)
	for h := 0; h < len(b.queue); h++ {
		v := b.queue[h]
		for _, to := range b.adj[v] {
			switch {
			case b.base[v] == b.base[to] || b.match[v] == to:
			case b.closesBlossom(to, root):
				b.contract(v, to)
			case b.parent[to] == -1 && b.extend(v, to):
				return to
			}
		}
	}
	return -1
}

func (b *blossom) maxMatching() int {
	size := 0
	for v := 0; v < b.n; v++ {
		if b.match[v] != -1 {
			continue
		}
		for end := b.findPath(v); end != -1; {
			pv := b.parent[end]
			next := b.match[pv]
			b.match[end], b.match[pv] = pv, end
			end = next
		}
	}
	for v := 0; v < b.n; v++ {
		if b.match[v] > v {
			size++
		}
	}
	return size
}
