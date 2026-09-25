package league

import (
	"database/sql"
	"errors"
	"log/slog"
	"math"
	"math/rand/v2"
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
	A, B string
	Slot int // 0 for round-robin; 1+ for leveled-league display grouping
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

type candidate struct {
	id       string
	dist     float64
	load     int
	position int
}

// chooseOpponentForRound returns the best eligible opponent for p within
// round k, or "" if none exists. Candidates inside the comfort zone are
// shuffled (random order); candidates outside are sorted nearest-first, then
// by lower load, then by better position. The first candidate that passes
// the completion check wins.
func (svc *Service) chooseOpponentForRound(st *leveledState, p string, k int) string {
	inside, outside := svc.collectCandidatesForRound(st, p, k)

	need := make(map[string]int, len(st.pairs))
	for _, id := range st.pairs {
		need[id] = st.target - st.load(id)
	}
	ctx := completionCtx{need: need, met: st.met, slack: minSlack(need, st.met)}

	if id := firstCompletable(p, inside, ctx); id != "" {
		return id
	}
	return firstCompletable(p, outside, ctx)
}

// collectCandidatesForRound builds the inside-zone (shuffled) and
// outside-zone (sorted) candidate lists for p, restricted to opponents that
// have no existing match in round k. Below the last Jornada (k < target) a
// pair with a match already in round k is excluded up front, before the
// comfort-zone split — otherwise an occupied pair could dominate the zone
// order and starve an available one. Distance is the absolute Elo gap
// (rather than a rank-position difference), so pairs tied on Elo — most
// visibly the four unranked pairs, all seeded at 0 — are genuinely
// equidistant instead of ordered apart by name.
func (svc *Service) collectCandidatesForRound(st *leveledState, p string, k int) (inside, outside []candidate) {
	eloP := st.elo[p]
	for _, q := range st.pairs {
		if !st.eligible(p, q) {
			continue
		}
		if k < st.target && st.occupied[q][k] {
			continue
		}
		d := math.Abs(st.elo[q] - eloP)
		c := candidate{id: q, dist: d, load: st.load(q), position: st.position[q]}
		if d <= float64(st.comfort) {
			inside = append(inside, c)
		} else {
			outside = append(outside, c)
		}
	}
	svc.shuffle(len(inside), func(i, j int) { inside[i], inside[j] = inside[j], inside[i] })
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
	return inside, outside
}

// completionCtx holds the per-candidate completion check inputs.
type completionCtx struct {
	need  map[string]int
	met   map[string]map[string]struct{}
	slack int
}

// firstCompletable returns the id of the first candidate that keeps the
// schedule completable after adding the p–candidate pairing, or "" if none do.
func firstCompletable(p string, candidates []candidate, ctx completionCtx) string {
	for _, c := range candidates {
		needAfter := make(map[string]int, len(ctx.need))
		for k, v := range ctx.need {
			needAfter[k] = v
		}
		needAfter[p]--
		needAfter[c.id]--
		metAfter := cloneMet(ctx.met)
		addMet(metAfter, p, c.id)
		if completable(needAfter, metAfter, ctx.slack, candidateTries) {
			return c.id
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
	rng := rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	for range tries {
		if tryGreedy(need, met, slack, rng) {
			return true
		}
	}
	return false
}

// tryGreedy attempts one greedy pass: serve the most-constrained pair first.
// rng provides per-pass randomness so repeated calls explore different orderings.
func tryGreedy(origNeed map[string]int, origMet map[string]map[string]struct{}, slack int, rng *rand.Rand) bool {
	need := make(map[string]int, len(origNeed))
	for k, v := range origNeed {
		need[k] = v
	}
	met := cloneMet(origMet)

	missed := 0
	for {
		pairs := pairsWithNeed(need)
		if len(pairs) == 0 {
			break
		}
		sortByConstraint(pairs, need, met, rng)
		p := pairs[0]

		candidates := eligibleCandidates(p, need, met)
		if len(candidates) == 0 {
			missed += need[p]
			need[p] = 0
			if missed > slack {
				return false
			}
			continue
		}
		sortByConstraint(candidates, need, met, rng)
		missed += fillNeed(p, candidates, need, met)
		if missed > slack {
			return false
		}
	}
	return missed <= slack
}

// sortByConstraint sorts ids by fewest (options − need) first; ties broken randomly.
func sortByConstraint(ids []string, need map[string]int, met map[string]map[string]struct{}, rng *rand.Rand) {
	sort.Slice(ids, func(i, j int) bool {
		oi := countEligible(ids[i], need, met) - need[ids[i]]
		oj := countEligible(ids[j], need, met) - need[ids[j]]
		if oi != oj {
			return oi < oj
		}
		return rng.Float64() < 0.5
	})
}

// fillNeed fills all of p's remaining need slots from candidates and returns
// the number of slots that could not be filled (need[p] > len(candidates)).
func fillNeed(p string, candidates []string, need map[string]int, met map[string]map[string]struct{}) int {
	fill := need[p]
	missed := 0
	if fill > len(candidates) {
		missed = fill - len(candidates)
		fill = len(candidates)
	}
	for _, q := range candidates[:fill] {
		need[q]--
		addMet(met, p, q)
	}
	need[p] = 0
	return missed
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
			result = append(result, Pairing{A: home, B: away, Slot: k})
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
	return currentWindowFor(st.start, st.end, st.target, st.now)
}

// CurrentWindow returns the calendar Jornada a leveled competition is in
// right now: floor((now−start)/u)+1, clamped to [1, target_matches]. Returns
// 1 when there is no usable window (missing dates, zero target, or now
// before start) — the ordinary case where the season hasn't started yet.
func CurrentWindow(comp *core.Record, now time.Time) int {
	return currentWindowFor(
		comp.GetDateTime("start_date").Time(),
		comp.GetDateTime("end_date").Time(),
		comp.GetInt("target_matches"),
		now,
	)
}

// currentWindowFor computes floor((now−start)/u)+1, clamped to [1, target].
// Returns 1 when there is no usable window (missing dates, zero target/unit,
// or now before start).
func currentWindowFor(start, end time.Time, target int, now time.Time) int {
	if start.IsZero() || end.IsZero() || target <= 0 || !now.After(start) {
		return 1
	}
	u := end.Sub(start) / time.Duration(target)
	if u <= 0 {
		return 1
	}
	cur := int(math.Floor(float64(now.Sub(start))/float64(u))) + 1
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
// daily cron. avoid marks pairs as met for this run only (so a just-deleted
// pairing is not immediately recreated). Returns nil, nil for a missing
// competition (not an error — the competition may have been deleted).
func (svc *Service) TopUpAssignments(compID string, now time.Time, avoid ...Pairing) ([]*core.Record, error) {
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

	return &leveledState{
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
	}, nil
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
		if withdrawn[p1] || withdrawn[p2] {
			continue
		}
		addMet(t.met, p1, p2)
		t.home[p1]++
		if slot := m.GetInt("slot"); slot > 0 {
			t.occupied[p1][slot] = true
			t.occupied[p2][slot] = true
		}
		if m.GetString("status") == "final" {
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

// clampInt restricts v to [lo, hi].
func clampInt(v, lo, hi int) int {
	return max(lo, min(v, hi))
}
