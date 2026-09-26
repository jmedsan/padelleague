package league

import (
	"database/sql"
	"errors"
	"log/slog"
	"math"
	"math/bits"
	"sort"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// eloComfortZone is the Elo-point width of a candidate's "comfort zone" in
// collectCandidatesForRound — one skill level (see league/rating.go's
// Levels, spaced 100 points apart). A candidate within this gap is in the
// shuffled inside zone; farther candidates are sorted nearest-first outside.
const eloComfortZone = 100

// Pairing is an ordered pair of pair IDs assigned to play each other.
type Pairing struct {
	A, B    string
	Slot    int  // 0 for round-robin; 1+ for leveled-league display grouping
	Rematch bool // the pairing repeats an already-met opponent (rematch mode)
}

// IsLeveled reports whether the competition assigns opponents by rating
// proximity instead of generating a full round-robin. Deliberately computed
// against the competition's full (configured) pairs list, not the active
// (non-withdrawn) count: this classification drives Jornada grouping, the
// Aj. standings column, and top-up eligibility for the competition's whole
// lifetime, and must never flip mid-season just because a pair withdraws —
// a withdrawal shrinking the active pair count below the leveled threshold
// is a completability problem for the assignment engine (ffactor operates
// on buildLeveledState's active-only pairs list), not a reason to
// reclassify the competition as round-robin.
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
	comfort  int // Elo points; a candidate within this gap is in the shuffled "comfort zone"
	pairs    []string
	elo      map[string]float64 // pair id -> hidden Elo rating at assignment time
	played   map[string]int
	pending  map[string]int
	met      map[string]map[string]struct{}
	position map[string]int          // 0-based index in rating order
	occupied map[string]map[int]bool // pair id -> set of slots it already has a match at
	home     map[string]int          // pair id -> number of matches played/pending as home (pair1)
	tieCoin  int                     // flips on every home/away tiebreak, so symmetric ties alternate
	start    time.Time               // competition start_date, zero if unset
	end      time.Time               // competition end_date, zero if unset
	now      time.Time
	loc      *time.Location // league display timezone; today's Jornada is computed in this zone
	rematch  bool           // no exact completion exists (after withdrawals): met pairs may play again
}

func (st *leveledState) load(p string) int {
	return st.played[p] + st.pending[p]
}

// wants reports whether p needs a new assignment now.
func (st *leveledState) wants(p string) bool {
	return st.pending[p] < st.open && st.load(p) < st.target
}

// balance returns p's home/away balance: 2*home − load, positive when p has
// played more matches at home than away.
func (st *leveledState) balance(p string) int {
	return 2*st.home[p] - st.load(p)
}

// homeTeam decides which of p, q plays at home for a new pairing, picking
// whichever has the lower home/away balance. A balance tie (including the
// common 0-0 case for two pairs with no prior matches) alternates via
// tieCoin: p and q are symmetric at a tie, so a value derived from either
// pair's own state would resolve the same way every time a similarly
// balanced pair meets another — tieCoin flips on every tie regardless of
// who's involved, so repeated ties still alternate.
func (st *leveledState) homeTeam(p, q string) (home, away string) {
	bp, bq := st.balance(p), st.balance(q)
	switch {
	case bq < bp:
		return q, p
	case bp < bq:
		return p, q
	default:
		st.tieCoin++
		if st.tieCoin%2 == 0 {
			return p, q
		}
		return q, p
	}
}

// eligible reports whether q is a valid opponent candidate for p. In rematch
// mode an already-met q stays eligible; relaxed lifts the pending ≤ open cap
// (the target cap always holds).
func (st *leveledState) eligible(p, q string, relaxed bool) bool {
	if q == p {
		return false
	}
	if _, met := st.met[p][q]; met && !st.rematch {
		return false
	}
	if st.load(q) >= st.target {
		return false
	}
	if !relaxed && st.pending[q] > st.open {
		return false
	}
	return true
}

// candidateFor reports whether q can be offered to p in round k: eligible,
// free in round k (below the last Jornada), and — in the relaxed pass — not
// already tried in the normal pass.
func (st *leveledState) candidateFor(p, q string, k int, relaxed bool) bool {
	if !st.eligible(p, q, relaxed) {
		return false
	}
	if k < st.target && st.occupied[q][k] {
		return false
	}
	return !relaxed || st.pending[q] > st.open
}

// hasMet reports whether p and q already have a match together.
func (st *leveledState) hasMet(p, q string) bool {
	_, met := st.met[p][q]
	return met
}

type candidate struct {
	id       string
	dist     float64
	load     int
	position int
	met      bool
}

// chooseOpponentForRound returns the best eligible opponent for p within
// round k, or "" if none exists. Candidates inside the comfort zone are
// shuffled for tie-breaking, then stably sorted by lower load first, so an
// equal-Elo pair with fewer pending matches is preferred and ties are still
// random; candidates outside are sorted nearest-first, then by lower load,
// then by better position. The first candidate that passes the completion
// check wins.
func (svc *Service) chooseOpponentForRound(st *leveledState, p string, k int) string {
	fs := buildFactorState(st)
	inside, outside := svc.collectCandidatesForRound(st, p, k, false)
	if id := firstFactorable(p, inside, fs); id != "" {
		return id
	}
	if id := firstFactorable(p, outside, fs); id != "" {
		return id
	}
	// Fallback: no opponent at or below open+1 — never leave p waiting while
	// a completable assignment exists. Least-loaded first, target cap kept.
	inside, outside = svc.collectCandidatesForRound(st, p, k, true)
	relaxed := append(inside, outside...)
	sort.SliceStable(relaxed, func(i, j int) bool { return relaxed[i].load < relaxed[j].load })
	return firstFactorable(p, relaxed, fs)
}

// collectCandidatesForRound builds the inside-zone (shuffled, then sorted by
// load) and outside-zone (sorted) candidate lists for p, restricted to
// opponents that have no existing match in round k. Below the last Jornada
// (k < target) a pair with a match already in round k is excluded up front,
// before the comfort-zone split — otherwise an occupied pair could dominate
// the zone order and starve an available one. Distance is the absolute Elo
// gap (rather than a rank-position difference), so pairs tied on Elo — most
// visibly the four unranked pairs, all seeded at 0 — are genuinely
// equidistant instead of ordered apart by name.
func (svc *Service) collectCandidatesForRound(st *leveledState, p string, k int, relaxed bool) (inside, outside []candidate) {
	eloP := st.elo[p]
	for _, q := range st.pairs {
		if !st.candidateFor(p, q, k, relaxed) {
			continue
		}
		d := math.Abs(st.elo[q] - eloP)
		c := candidate{id: q, dist: d, load: st.load(q), position: st.position[q], met: st.hasMet(p, q)}
		if d <= float64(st.comfort) && !st.rematch {
			inside = append(inside, c)
		} else {
			outside = append(outside, c)
		}
	}
	svc.shuffle(len(inside), func(i, j int) { inside[i], inside[j] = inside[j], inside[i] })
	sort.SliceStable(inside, func(i, j int) bool { return inside[i].load < inside[j].load })
	sort.Slice(outside, func(i, j int) bool {
		a, b := outside[i], outside[j]
		if a.met != b.met {
			return !a.met // rematch mode: unmet opponents first
		}
		if a.dist != b.dist {
			return a.dist < b.dist
		}
		if a.load != b.load {
			return a.load < b.load
		}
		return a.position < b.position
	})
	return inside, outside
}

// factorState is the index-based (bitmask) view of a leveledState's
// remaining need and eligibility, the input shape ffactor operates on.
// Vertex i corresponds to st.pairs[i]; index maps a pair id back to its
// vertex for callers working with string ids.
type factorState struct {
	need      []int
	avail     []uint32
	index     map[string]int
	rematch   bool // completion test is the multigraph condition, avail ignores met
	shortfall int  // matches already unschedulable in rematch mode; a pairing may not add to it
}

// buildFactorState derives a factorState from st: need[v] is how many more
// matches st.pairs[v] must play to reach target; avail[v] is the bitmask of
// vertices v could still be paired with (excludes v itself, anyone already
// met, and anyone with no remaining need). Rebuilt fresh on every call since
// plan() mutates st.pending/st.met between requesters.
func buildFactorState(st *leveledState) factorState {
	n := len(st.pairs)
	index := make(map[string]int, n)
	for i, id := range st.pairs {
		index[id] = i
	}
	need := make([]int, n)
	for i, id := range st.pairs {
		need[i] = st.target - st.load(id)
	}
	avail := make([]uint32, n)
	for i, id := range st.pairs {
		var mask uint32
		for j, other := range st.pairs {
			if j == i || need[j] <= 0 {
				continue
			}
			if _, met := st.met[id][other]; met && !st.rematch {
				continue
			}
			mask |= 1 << uint(j)
		}
		avail[i] = mask
	}
	fs := factorState{need: need, avail: avail, index: index, rematch: st.rematch}
	if st.rematch {
		fs.shortfall = shortfallMatches(need)
	}
	return fs
}

// completable reports whether the remaining need can still be met: exactly
// (ffactor) in normal mode; in rematch mode the pairing must not add to the
// unavoidable shortfall (loopless-multigraph realizability, Hakimi 1962).
func (fs factorState) completable(need []int, avail []uint32) bool {
	if !fs.rematch {
		return ffactor(need, avail)
	}
	return shortfallMatches(need) <= fs.shortfall
}

// firstFactorable returns the id of the first candidate for which pairing p
// with it still leaves an exactly completable schedule (ffactor), or "" if
// none do. fs is never mutated — each candidate is tried against a fresh
// copy of fs.need/fs.avail.
func firstFactorable(p string, candidates []candidate, fs factorState) string {
	pv := fs.index[p]
	for _, c := range candidates {
		qv := fs.index[c.id]
		need := append([]int(nil), fs.need...)
		avail := append([]uint32(nil), fs.avail...)
		need[pv]--
		need[qv]--
		avail[pv] &^= 1 << uint(qv)
		avail[qv] &^= 1 << uint(pv)
		if fs.completable(need, avail) {
			return c.id
		}
	}
	return ""
}

// ffactor reports whether the remaining schedule is EXACTLY completable:
// every vertex with need > 0 can be paired down to need == 0 using only its
// avail candidates, with no slack. Vertices are 0..len(need)-1; avail[v] is
// the bitmask of vertices v may still be paired with. Deterministic;
// memoized within this call's own search tree only (see ffactorMemoized) —
// a package-level cache would grow unbounded over a long-running process
// life, and a fresh call tree gains nothing from a prior competition's
// (need, avail) states.
func ffactor(need []int, avail []uint32) bool {
	return ffactorMemoized(need, avail, make(map[string]bool))
}

// ffactorMemoized is ffactor's recursive core, threading one memo map
// through the whole search so the identical (need, avail) state reached via
// different branches is solved once.
func ffactorMemoized(need []int, avail []uint32, memo map[string]bool) bool {
	var live uint32
	total := 0
	for v, n := range need {
		if n > 0 {
			live |= 1 << uint(v)
			total += n
		}
	}
	if live == 0 {
		return true
	}
	if total%2 != 0 {
		return false
	}
	for v, n := range need {
		if n <= 0 {
			continue
		}
		a := avail[v] & live
		if bits.OnesCount32(a) < n {
			return false
		}
		avail[v] = a
	}

	key := factorKey(need, avail)
	if cached, ok := memo[key]; ok {
		return cached
	}

	v := pickFactorVertex(need, avail, live)
	result := tryFactorSubsets(need, avail, v, memo)
	memo[key] = result
	return result
}

// pickFactorVertex returns the live vertex minimizing
// popcount(avail[v]) - need[v] (most constrained first), breaking ties
// toward larger need — the same "hardest first" heuristic a greedy solver
// uses, kept here to bound the branching factor of the exact search.
func pickFactorVertex(need []int, avail []uint32, live uint32) int {
	best := -1
	var bestSlack int
	for v := 0; v < len(need); v++ {
		if live&(1<<uint(v)) == 0 {
			continue
		}
		slack := bits.OnesCount32(avail[v]) - need[v]
		if best == -1 || slack < bestSlack || (slack == bestSlack && need[v] > need[best]) {
			best, bestSlack = v, slack
		}
	}
	return best
}

// tryFactorSubsets enumerates every size-need[v] subset of avail[v], applies
// it (v and each chosen partner's need drop by one, mutual availability
// closes), and recurses. Restores need/avail after each attempt so sibling
// subsets see the original state. Returns true on the first subset that
// leads to a fully completable schedule.
func tryFactorSubsets(need []int, avail []uint32, v int, memo map[string]bool) bool {
	if need[v] == 0 {
		return ffactorMemoized(need, avail, memo)
	}
	for _, subset := range subsetsOfSize(avail[v], need[v]) {
		origNeed := append([]int(nil), need...)
		origAvail := append([]uint32(nil), avail...)

		need[v] = 0
		for u := 0; u < len(need); u++ {
			if subset&(1<<uint(u)) == 0 {
				continue
			}
			need[u]--
			avail[v] &^= 1 << uint(u)
			avail[u] &^= 1 << uint(v)
		}
		if ffactorMemoized(need, avail, memo) {
			return true
		}
		copy(need, origNeed)
		copy(avail, origAvail)
	}
	return false
}

// subsetsOfSize returns every subset of mask with exactly size bits set.
func subsetsOfSize(mask uint32, size int) []uint32 {
	var members []int
	for v := 0; v < 32; v++ {
		if mask&(1<<uint(v)) != 0 {
			members = append(members, v)
		}
	}
	if size <= 0 || size > len(members) {
		return nil
	}
	var out []uint32
	combo := make([]int, size)
	var rec func(start, depth int)
	rec = func(start, depth int) {
		if depth == size {
			var s uint32
			for _, b := range combo {
				s |= 1 << uint(b)
			}
			out = append(out, s)
			return
		}
		for i := start; i < len(members); i++ {
			combo[depth] = members[i]
			rec(i+1, depth+1)
		}
	}
	rec(0, 0)
	return out
}

// factorKey builds a memoization key from need/avail — a length-prefixed
// byte encoding is enough since both slices share a fixed size per ffactor
// call tree and never grow.
func factorKey(need []int, avail []uint32) string {
	buf := make([]byte, 0, len(need)*4+len(avail)*4)
	for _, n := range need {
		buf = append(buf, byte(n), byte(n>>8), byte(n>>16), byte(n>>24))
	}
	for _, a := range avail {
		buf = append(buf, byte(a), byte(a>>8), byte(a>>16), byte(a>>24))
	}
	return string(buf)
}

// plan fills one Jornada at a time, starting from the competition's current
// calendar window, so every pair plays at most once per round (recipe §3.5).
// Each round repeats the wants/choose/pair loop until no requester remains,
// then advances to the next Jornada. The loop stops early once no pair wants
// a new assignment — reached after `open` rounds in the initial batch, or
// immediately once a top-up's few requesters are satisfied.
func plan(svc *Service, st *leveledState) []Pairing {
	var result []Pairing

	for k := st.currentWindow(); k <= st.target; k++ {
		skipped := map[string]bool{}
		for {
			p := nextRequesterForRound(st, skipped, k)
			if p == "" {
				break
			}
			q := svc.chooseOpponentForRound(st, p, k)
			if q == "" {
				skipped[p] = true
				continue
			}
			home, away := st.homeTeam(p, q)
			result = append(result, Pairing{A: home, B: away, Slot: k, Rematch: st.hasMet(p, q)})
			st.pending[p]++
			st.pending[q]++
			st.markHome(home)
			addMet(st.met, p, q)
			st.occupy(p, k)
			st.occupy(q, k)
		}
		if !anyWants(st) {
			break
		}
	}
	return result
}

// anyWants reports whether any pair still wants a new assignment.
func anyWants(st *leveledState) bool {
	for _, p := range st.pairs {
		if st.wants(p) {
			return true
		}
	}
	return false
}

// occupy records that p already has a match at slot.
func (st *leveledState) occupy(p string, slot int) {
	if st.occupied[p] == nil {
		st.occupied[p] = map[int]bool{}
	}
	st.occupied[p][slot] = true
}

// markHome records that p plays at home for a new pairing.
func (st *leveledState) markHome(p string) {
	if st.home == nil {
		st.home = map[string]int{}
	}
	st.home[p]++
}

// currentWindow returns the calendar Jornada the competition is in right now.
func (st *leveledState) currentWindow() int {
	return currentWindowFor(season{st.start, st.end, st.target}, st.now, st.loc)
}

// CurrentWindow returns the calendar Jornada a leveled competition is in
// right now: floor(i·T/D)+1 where i is the day offset of today (in the
// league's display timezone) from start_date, clamped to [1,
// target_matches]. Returns 1 when there is no usable window (missing dates,
// zero target, or today before start) — the ordinary case where the season
// hasn't started yet.
func CurrentWindow(comp *core.Record, now time.Time, app core.App) int {
	s := season{
		start:  comp.GetDateTime("start_date").Time(),
		end:    comp.GetDateTime("end_date").Time(),
		target: comp.GetInt("target_matches"),
	}
	return currentWindowFor(s, now, Timezone(app))
}

// season is a leveled competition's play window and Jornada count — the
// three inputs JornadaWindow's day-offset partition is computed from.
type season struct {
	start, end time.Time
	target     int
}

// currentWindowFor returns the Jornada containing now, using the same
// whole-day partition as JornadaWindow: with i the calendar-day offset of
// today (now, read in loc) from s.start and D the inclusive day count,
// window = floor(i·T/D)+1. now is converted to loc's calendar date before
// any comparison, so a moment past midnight UTC but still "yesterday" in loc
// (e.g. 00:30 in a timezone east of UTC) is not miscounted into the next
// Jornada. Returns 1 when there is no usable window (missing dates, zero
// target, or today before start); clamps to target once today reaches or
// passes end.
func currentWindowFor(s season, now time.Time, loc *time.Location) int {
	start, end, target := s.start, s.end, s.target
	if start.IsZero() || end.IsZero() || target <= 0 {
		return 1
	}
	today := calendarDay(now, loc)
	if !today.After(start) {
		return 1
	}
	days := SeasonDays(start, end)
	if days <= 0 {
		return 1
	}
	i := clampInt(daysBetween(start, today), 0, days-1)
	cur := i*target/days + 1
	return clampInt(cur, 1, target)
}

// nextRequesterForRound returns the pair that wants an assignment, has no
// existing match in round k, and has the lowest load, then the best (lowest)
// position. Below the last Jornada (k < target) a pair already occupying
// round k is excluded; at k == target the occupancy check is skipped so a
// pair that still wants() can receive multiple matches in the final round.
// Returns "" when none wants one.
func nextRequesterForRound(st *leveledState, skipped map[string]bool, k int) string {
	best := ""
	for _, p := range st.pairs {
		if skipped[p] || !st.wants(p) {
			continue
		}
		if k < st.target && st.occupied[p][k] {
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

// GenerateInitialAssignments creates the initial pending matches for a leveled
// league. Called inside a transaction by the fixture handler. Returns the count
// of created matches. Does NOT notify — PublishCalendar handles that.
func (svc *Service) GenerateInitialAssignments(txApp core.App, comp *core.Record, now time.Time) (int, error) {
	st, err := buildLeveledState(txApp, comp, nil, now)
	if err != nil {
		return 0, err
	}

	pairings := plan(svc, st)
	if err := createMatchRecords(txApp, comp, pairings, now); err != nil {
		return 0, err
	}
	return len(pairings), nil
}

// TopUpAssignments creates new pending matches for pairs that need them. Called
// after a match becomes final, after a pending match is deleted, and by the
// daily cron — so concurrent calls for the same competition are expected
// (two matches finalizing at once each trigger their own call); the whole
// read-plan-save sequence is serialized per competition to prevent two
// calls from independently reading the same stale state and each topping a
// pair up past `open`, or creating the same pairing twice. avoid marks
// pairs as met for this run only (so a just-deleted pairing is not
// immediately recreated). Returns nil, nil for a missing competition (not
// an error — the competition may have been deleted).
func (svc *Service) TopUpAssignments(compID string, now time.Time, avoid ...Pairing) ([]*core.Record, error) {
	lock := svc.lockTopUp(compID)
	lock.Lock()
	defer lock.Unlock()

	comp, err := svc.app.FindRecordById("competitions", compID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		slog.Error("top-up: find competition", "id", compID, "err", err)
		return nil, nil
	}

	if !IsLeveled(comp) {
		return nil, nil
	}
	if comp.GetString("calendar_status") != "published" {
		return nil, nil
	}
	if CompetitionPhase(comp, now) != PhasePlaying {
		return nil, nil
	}

	st, err := buildLeveledState(svc.app, comp, avoid, now)
	if err != nil {
		return nil, err
	}

	pairings := plan(svc, st)
	if len(pairings) == 0 {
		return nil, nil
	}

	created, err := saveAssignments(svc.app, comp, pairings, now)
	if err != nil {
		return nil, err
	}
	notifyAssignments(svc, comp, created)
	return created, nil
}

func saveAssignments(app core.App, comp *core.Record, pairings []Pairing, now time.Time) ([]*core.Record, error) {
	var created []*core.Record
	if err := app.RunInTransaction(func(txApp core.App) error {
		col, err := txApp.FindCollectionByNameOrId("matches")
		if err != nil {
			return err
		}
		for _, p := range pairings {
			rec := core.NewRecord(col)
			setMatchFields(rec, comp, p, now)
			if err := txApp.Save(rec); err != nil {
				return err
			}
			created = append(created, rec)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return created, nil
}

func notifyAssignments(svc *Service, comp *core.Record, created []*core.Record) {
	if svc.notifier == nil || len(created) == 0 {
		return
	}
	compName := comp.GetString("name")
	pairNames := PairNames(svc.app, comp.GetStringSlice("pairs"))
	for _, m := range created {
		p1ID := m.GetString("pair1")
		p2ID := m.GetString("pair2")
		svc.notifier.NotifyPlayers(PlayersForPair(svc.app, p1ID), NotifMatchAssigned(m.Id, pairNames[p2ID], compName))
		svc.notifier.NotifyPlayers(PlayersForPair(svc.app, p2ID), NotifMatchAssigned(m.Id, pairNames[p1ID], compName))
	}
}

// buildLeveledState constructs the in-memory state from the competition's
// current matches and ratings, marking avoid pairs as met.
func buildLeveledState(app core.App, comp *core.Record, avoid []Pairing, now time.Time) (*leveledState, error) {
	target := comp.GetInt("target_matches")
	open := OpenAssignments(comp)

	allPairIDs := comp.GetStringSlice("pairs")
	withdrawnSet := make(map[string]bool)
	for _, id := range comp.GetStringSlice("withdrawn_pairs") {
		withdrawnSet[id] = true
	}
	var activePairIDs []string
	for _, id := range allPairIDs {
		if !withdrawnSet[id] {
			activePairIDs = append(activePairIDs, id)
		}
	}

	ratings, err := Ratings(app, comp)
	if err != nil {
		return nil, err
	}

	pairNames := PairNames(app, activePairIDs)
	sortedPairs := sortPairsByRating(activePairIDs, ratings, pairNames)

	position := make(map[string]int, len(sortedPairs))
	for i, id := range sortedPairs {
		position[id] = i
	}

	matches, err := app.FindRecordsByFilter("matches",
		"competition = {:c}",
		"", 0, 0, map[string]any{"c": comp.Id})
	if err != nil {
		return nil, err
	}

	tally := tallyMatchState(sortedPairs, matches, withdrawnSet)

	for _, av := range avoid {
		addMet(tally.met, av.A, av.B)
	}

	st := &leveledState{
		target:   target,
		open:     open,
		comfort:  eloComfortZone,
		pairs:    sortedPairs,
		elo:      ratings,
		played:   tally.played,
		pending:  tally.pending,
		met:      tally.met,
		position: position,
		occupied: tally.occupied,
		home:     tally.home,
		start:    comp.GetDateTime("start_date").Time(),
		end:      comp.GetDateTime("end_date").Time(),
		now:      now,
		loc:      Timezone(app),
	}
	st.detectRematch(comp)
	return st, nil
}

// detectRematch switches the run to rematch mode when no exact completion
// exists for the active roster.
func (st *leveledState) detectRematch(comp *core.Record) {
	fs := buildFactorState(st)
	if ffactor(fs.need, fs.avail) {
		return
	}
	st.rematch = true
	slog.Warn("leveled: no exact completion left, rematch mode",
		"competition", comp.Id, "name", comp.GetString("name"),
		"shortfall_matches", shortfallMatches(fs.need))
}

// Shortfall describes matches a leveled competition can no longer schedule
// under the target cap, even allowing rematches. Matches is how many are
// missing; PairIDs lists the pairs that may end below target.
type Shortfall struct {
	Matches int
	PairIDs []string
}

// LeveledShortfall reports the unavoidable shortfall for comp: zero Matches
// when every active pair can still reach target (with rematches if needed).
// With an odd total need any one of the listed pairs ends one match short;
// otherwise the listed pairs need more matches than the rest can supply.
func LeveledShortfall(app core.App, comp *core.Record, now time.Time) (Shortfall, error) {
	st, err := buildLeveledState(app, comp, nil, now)
	if err != nil {
		return Shortfall{}, err
	}
	fs := buildFactorState(st)
	missing := shortfallMatches(fs.need)
	if missing == 0 {
		return Shortfall{}, nil
	}
	total, maxNeed := 0, 0
	for _, n := range fs.need {
		if n > 0 {
			total += n
			maxNeed = max(maxNeed, n)
		}
	}
	var ids []string
	for v, n := range fs.need {
		if n > 0 && (total%2 != 0 || n == maxNeed) {
			ids = append(ids, st.pairs[v])
		}
	}
	return Shortfall{Matches: missing, PairIDs: ids}, nil
}

// shortfallMatches returns how many matches a need vector cannot realize
// even with rematches: the excess of the largest need over what the rest
// can supply, or one when the total is odd.
func shortfallMatches(need []int) int {
	total, maxNeed := 0, 0
	for _, n := range need {
		if n > 0 {
			total += n
			maxNeed = max(maxNeed, n)
		}
	}
	if total == 0 {
		return 0
	}
	if excess := maxNeed - (total - maxNeed); excess > 0 {
		return excess
	}
	return total % 2
}

// createMatchRecords writes one match record per pairing using txApp.
func createMatchRecords(txApp core.App, comp *core.Record, pairings []Pairing, now time.Time) error {
	if len(pairings) == 0 {
		return nil
	}
	col, err := txApp.FindCollectionByNameOrId("matches")
	if err != nil {
		return err
	}
	for _, p := range pairings {
		rec := core.NewRecord(col)
		setMatchFields(rec, comp, p, now)
		if err := txApp.Save(rec); err != nil {
			return err
		}
	}
	return nil
}

// setMatchFields populates a new match record for a leveled assignment.
func setMatchFields(rec *core.Record, comp *core.Record, p Pairing, now time.Time) {
	rec.Set("competition", comp.Id)
	rec.Set("round_number", 0)
	rec.Set("matches_to_win", 1)
	rec.Set("pair1", p.A)
	rec.Set("pair2", p.B)
	rec.Set("status", "pending")
	rec.Set("rematch", p.Rematch)

	slot := p.Slot
	if target := comp.GetInt("target_matches"); slot > target {
		slot = target
	}
	rec.Set("slot", slot)

	if slot > 0 {
		if deadline, ok := SlotDeadline(comp, slot); ok {
			rec.Set("arrange_by", deadline.Format("2006-01-02"))
		}
	} else if deadline, ok := assignmentDeadline(comp, now); ok {
		rec.Set("arrange_by", deadline.Format("2006-01-02"))
	}
}

// sortPairsByRating returns a copy of pairs sorted by rating desc, then pair
// name asc (deterministic tiebreak).
func sortPairsByRating(pairs []string, ratings map[string]float64, pairNames map[string]string) []string {
	sorted := make([]string, len(pairs))
	copy(sorted, pairs)
	sort.SliceStable(sorted, func(i, j int) bool {
		ri, rj := ratings[sorted[i]], ratings[sorted[j]]
		if ri != rj {
			return ri > rj
		}
		return pairNames[sorted[i]] < pairNames[sorted[j]]
	})
	return sorted
}

// matchTally holds the per-pair state derived from a competition's existing
// matches: played/pending counts, the met-pairs set, occupied slots, and
// home-match counts.
type matchTally struct {
	played   map[string]int
	pending  map[string]int
	met      map[string]map[string]struct{}
	occupied map[string]map[int]bool
	home     map[string]int
}

// tallyMatchState classifies existing matches into played/pending counts, the
// met-pairs set, each pair's occupied slots, and home-match counts, skipping
// matches involving withdrawn pairs.
func tallyMatchState(pairs []string, matches []*core.Record, withdrawn map[string]bool) matchTally {
	t := matchTally{
		played:   make(map[string]int, len(pairs)),
		pending:  make(map[string]int, len(pairs)),
		met:      make(map[string]map[string]struct{}, len(pairs)),
		occupied: make(map[string]map[int]bool, len(pairs)),
		home:     make(map[string]int, len(pairs)),
	}
	for _, id := range pairs {
		t.met[id] = map[string]struct{}{}
		t.occupied[id] = map[int]bool{}
	}
	for _, m := range matches {
		p1, p2 := m.GetString("pair1"), m.GetString("pair2")
		final := m.GetString("status") == "final"
		// A non-final match against a withdrawn pair never resolves — the
		// pending side skips it entirely. A final match still counts in
		// full for the non-withdrawn side: a real result played before the
		// withdrawal, or the walkover WithdrawPair records for the
		// opponent, is a genuine result the standings must keep.
		if (withdrawn[p1] || withdrawn[p2]) && !final {
			continue
		}
		addMet(t.met, p1, p2)
		t.home[p1]++
		if slot := m.GetInt("slot"); slot > 0 {
			occupySlot(t.occupied, p1, slot)
			occupySlot(t.occupied, p2, slot)
		}
		if final {
			t.played[p1]++
			t.played[p2]++
		} else {
			t.pending[p1]++
			t.pending[p2]++
		}
	}
	return t
}

// -- helpers ----------------------------------------------------------------

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

// occupySlot marks p as occupying slot, auto-vivifying p's entry — needed
// because a withdrawn pair (whose final matches still count, see
// tallyMatchState) was never pre-seeded into occupied like the active pairs.
func occupySlot(occupied map[string]map[int]bool, p string, slot int) {
	if occupied[p] == nil {
		occupied[p] = map[int]bool{}
	}
	occupied[p][slot] = true
}

// clampInt restricts v to [lo, hi].
func clampInt(v, lo, hi int) int {
	return max(lo, min(v, hi))
}
