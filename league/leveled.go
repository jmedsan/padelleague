package league

import (
	"database/sql"
	"errors"
	"log/slog"
	"math"
	"sort"
	"time"

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

type candidate struct {
	id       string
	dist     int
	load     int
	position int
}

// chooseOpponent returns the best eligible opponent for p, or "" if none exists.
// Candidates inside the comfort zone are shuffled (random order); candidates
// outside are sorted nearest-first, then by lower load, then by better position.
// The first candidate that passes the completion check wins.
func (svc *Service) chooseOpponent(st *leveledState, p string) string {
	inside, outside := svc.collectCandidates(st, p)

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

// collectCandidates builds the inside-zone (shuffled) and outside-zone (sorted)
// candidate lists for p.
func (svc *Service) collectCandidates(st *leveledState, p string) (inside, outside []candidate) {
	posP := st.position[p]
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

// GenerateInitialAssignments creates the initial pending matches for a leveled
// league. Called inside a transaction by the fixture handler. Returns the count
// of created matches. Does NOT notify — PublishCalendar handles that.
func (svc *Service) GenerateInitialAssignments(txApp core.App, comp *core.Record, now time.Time) (int, error) {
	st, err := buildLeveledState(txApp, comp, nil)
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
		// PocketBase wraps not-found in a different error; treat any lookup
		// failure for a known ID pattern as "not found".
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

	st, err := buildLeveledState(svc.app, comp, avoid)
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
		allPlayers := append(PlayersForPair(svc.app, p1ID), PlayersForPair(svc.app, p2ID)...)
		svc.notifier.NotifyPlayers(allPlayers, NotifMatchAssigned(m.Id, pairNames[p2ID], compName))
	}
}

// buildLeveledState constructs the in-memory state from the competition's
// current matches and ratings, marking avoid pairs as met.
func buildLeveledState(app core.App, comp *core.Record, avoid []Pairing) (*leveledState, error) {
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

	seedIDs := comp.GetStringSlice("seed_pairs")
	pairNames := PairNames(app, activePairIDs)
	sortedPairs := sortPairsByRating(activePairIDs, ratings, seedIDs, pairNames)

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

	played, pending, met := tallyMatchState(sortedPairs, matches, withdrawnSet)

	for _, av := range avoid {
		addMet(met, av.A, av.B)
	}

	return &leveledState{
		target:   target,
		open:     open,
		comfort:  ceilDiv(target, 2),
		pairs:    sortedPairs,
		played:   played,
		pending:  pending,
		met:      met,
		position: position,
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
	if deadline, ok := assignmentDeadline(comp, now); ok {
		rec.Set("arrange_by", deadline.Format("2006-01-02"))
	}
}

var _ = slog.Debug // keep slog import used

// sortPairsByRating returns a copy of pairs sorted by rating desc, then seed
// index asc, then pair name asc.
func sortPairsByRating(pairs []string, ratings map[string]float64, seedIDs []string, pairNames map[string]string) []string {
	seedPos := make(map[string]int, len(seedIDs))
	for i, id := range seedIDs {
		seedPos[id] = i
	}
	sorted := make([]string, len(pairs))
	copy(sorted, pairs)
	sort.SliceStable(sorted, func(i, j int) bool {
		ri, rj := ratings[sorted[i]], ratings[sorted[j]]
		if ri != rj {
			return ri > rj
		}
		si, hasI := seedPos[sorted[i]]
		sj, hasJ := seedPos[sorted[j]]
		if hasI != hasJ {
			return hasI
		}
		if hasI && si != sj {
			return si < sj
		}
		return pairNames[sorted[i]] < pairNames[sorted[j]]
	})
	return sorted
}

// tallyMatchState classifies existing matches into played/pending counts and
// the met-pairs set, skipping matches involving withdrawn pairs.
func tallyMatchState(pairs []string, matches []*core.Record, withdrawn map[string]bool) (played, pending map[string]int, met map[string]map[string]struct{}) {
	played = make(map[string]int, len(pairs))
	pending = make(map[string]int, len(pairs))
	met = make(map[string]map[string]struct{}, len(pairs))
	for _, id := range pairs {
		met[id] = map[string]struct{}{}
	}
	for _, m := range matches {
		p1, p2 := m.GetString("pair1"), m.GetString("pair2")
		if withdrawn[p1] || withdrawn[p2] {
			continue
		}
		addMet(met, p1, p2)
		if m.GetString("status") == "final" {
			played[p1]++
			played[p2]++
		} else {
			pending[p1]++
			pending[p2]++
		}
	}
	return played, pending, met
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
