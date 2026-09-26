package league

import (
	"flag"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	simPairs   = 16
	simTarget  = 10
	simOpen    = 3
	simScale   = 400.0
	simBlowout = 0.85
)

var (
	simSeasons = flag.Int("simulation.seasons", 0, "seasons per variant; 0 skips the simulation test")
	simSeed    = flag.Uint64("simulation.seed", 0, "outcome RNG seed; 0 = time-based, logged")
)

// simVariant describes one row in the comparison table.
type simVariant struct {
	name    string
	leveled bool      // false = random-pairing baseline, no DB
	seedSDs []float64 // nil = no seed; {2} = good seed; {6} = poor; {2,4,6} = mixed
}

func simVariants() []simVariant {
	return []simVariant{
		{"Random pairing (no leveling)", false, nil},
		{"Recipe, no admin seed", true, nil},
		{"Recipe, seed off by 6 places (poor)", true, []float64{6}},
		{"Recipe, seed off by 2, 4 or 6 places (mixed)", true, []float64{2, 4, 6}},
		{"Recipe, seed off by 2 places (good)", true, []float64{2}},
	}
}

// simEnv holds the shared app and model reused across all seasons of one variant.
type simEnv struct {
	app   core.App
	svc   *Service
	pairs []*core.Record
	model *matchModel
}

// simSeason owns per-season state for one leveled-league simulation season.
type simSeason struct {
	t       *testing.T
	app     core.App
	svc     *Service
	comp    *core.Record
	trueIdx map[string]int
	model   *matchModel
	rng     *rand.Rand
	clock   time.Time
	maxPend int
}

func newSimSeason(t *testing.T, env simEnv, v simVariant, rng *rand.Rand, seasonIdx int) *simSeason {
	t.Helper()

	// Per-season random permutation of true skill indices (F2 fix).
	perm := rng.Perm(simPairs)
	trueIdx := make(map[string]int, simPairs)
	for k, pair := range env.pairs {
		trueIdx[pair.Id] = perm[k]
	}

	comp := makeLeveledCompetition(t, env.app, env.pairs, simTarget, simOpen)

	if v.seedSDs != nil {
		sd := v.seedSDs[seasonIdx%len(v.seedSDs)]
		pairIDs := make([]string, simPairs)
		for i, p := range env.pairs {
			pairIDs[i] = p.Id
		}
		levels := noisySeedLevels(pairIDs, trueIdx, sd, rng)
		for _, p := range env.pairs {
			p.Set("level", levels[p.Id])
			require.NoError(t, env.app.Save(p))
		}
	}

	_, err := env.svc.GenerateInitialAssignments(env.app, comp, time.Now())
	require.NoError(t, err)

	return &simSeason{
		t:       t,
		app:     env.app,
		svc:     env.svc,
		comp:    comp,
		trueIdx: trueIdx,
		model:   env.model,
		rng:     rng,
		clock:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func (s *simSeason) run(acc *simMetrics) {
	s.t.Helper()
	for {
		pend := s.pending()
		s.trackMaxPending(pend)
		if len(pend) == 0 {
			break
		}
		m := pend[s.rng.IntN(len(pend))]
		p1 := m.GetString("pair1")
		p2 := m.GetString("pair2")
		i := s.trueIdx[p1]
		j := s.trueIdx[p2]
		score := s.model.play(i, j, s.rng)
		s.finalize(m, score)
		acc.addMatch(s.model, i, j)
		_, err := s.svc.TopUpAssignments(s.comp.Id, s.clock, Pairing{A: p1, B: p2})
		require.NoError(s.t, err)
		s.assertInvariants()
	}

	rows, err := s.svc.ComputeStandings(s.comp.Id)
	require.NoError(s.t, err)

	played := make([]int, simPairs)
	order := make([]int, len(rows))
	for k, row := range rows {
		played[k] = row.Played
		order[k] = s.trueIdx[row.PairID]
	}
	acc.addRanking(order)
	acc.addSeason(played, s.maxPend)
}

func (s *simSeason) pending() []*core.Record {
	s.t.Helper()
	recs, err := s.app.FindRecordsByFilter("matches",
		"competition = {:c} && status = 'pending'",
		"", 0, 0, map[string]any{"c": s.comp.Id})
	require.NoError(s.t, err)
	return recs
}

func (s *simSeason) trackMaxPending(pend []*core.Record) {
	pendPerPair := make(map[string]int, simPairs)
	for _, m := range pend {
		pendPerPair[m.GetString("pair1")]++
		pendPerPair[m.GetString("pair2")]++
	}
	for _, v := range pendPerPair {
		if v > s.maxPend {
			s.maxPend = v
		}
	}
}

func (s *simSeason) finalize(m *core.Record, score string) {
	s.t.Helper()
	s.clock = s.clock.Add(time.Minute)
	winner, err := DetermineWinner(m, score)
	require.NoError(s.t, err)
	m.Set("scores", score)
	m.Set("winner", winner)
	m.Set("status", "final")
	m.Set("finalized_at", s.clock.Format("2006-01-02 15:04:05.000Z"))
	require.NoError(s.t, s.app.Save(m))
}

func (s *simSeason) assertInvariants() {
	s.t.Helper()
	assert.False(s.t, hasDuplicatePairing(s.t, s.app, s.comp.Id), "P1: duplicate pairing")
	for _, p := range s.comp.GetStringSlice("pairs") {
		all := allMatchesFor(s.t, s.app, s.comp.Id, p)
		pend := pendingMatchesFor(s.t, s.app, s.comp.Id, p)
		played := 0
		for _, m := range all {
			if m.GetString("status") == "final" {
				played++
			}
		}
		load := played + len(pend)
		assert.LessOrEqual(s.t, load, simTarget, "P2: pair %s load %d > target", p, load)
		assert.LessOrEqual(s.t, len(pend), simOpen+1, "P3: pair %s pending %d > open+1", p, len(pend))
	}
}

func (s *simSeason) cleanup() {
	s.t.Helper()
	matches, err := s.app.FindRecordsByFilter("matches",
		"competition = {:c}", "", 0, 0, map[string]any{"c": s.comp.Id})
	require.NoError(s.t, err)
	for _, m := range matches {
		require.NoError(s.t, s.app.Delete(m))
	}
	require.NoError(s.t, s.app.Delete(s.comp))
}

// TestSimulation_LeveledLeague runs a multi-season comparison of leveled-league
// variants against a random-pairing baseline.
// Opt-in: -simulation.seasons=N (0 = skip, the default so make ci stays fast).
func TestSimulation_LeveledLeague(t *testing.T) {
	if *simSeasons == 0 {
		t.Skip("opt-in: run with -simulation.seasons=N")
	}

	seed := *simSeed
	if seed == 0 {
		seed = uint64(time.Now().UnixNano())
	}
	t.Logf("simulation seed: %d", seed)

	variants := simVariants()
	model := newMatchModel(simSkills())
	rows := make([]string, len(variants))

	t.Run("variants", func(t *testing.T) {
		for i, v := range variants {
			t.Run(v.name, func(t *testing.T) {
				t.Parallel()
				rng := rand.New(rand.NewPCG(seed, uint64(i)))
				acc := &simMetrics{}

				if !v.leveled {
					for range *simSeasons {
						runRandomBaseline(acc, model, rng)
					}
					rows[i] = acc.row(v.name)
					return
				}

				// Create app and 16 pairs once — bcrypt is expensive.
				app := newTestApp(t)
				svc := New(app, nil)
				pairs := make([]*core.Record, simPairs)
				for k := range pairs {
					pairs[k] = makePair(t, app, fmt.Sprintf("Sim %02d", k+1))
				}
				env := simEnv{app: app, svc: svc, pairs: pairs, model: model}

				for si := range *simSeasons {
					ss := newSimSeason(t, env, v, rng, si)
					ss.run(acc)
					ss.cleanup()
				}

				if *simSeasons >= 100 {
					assertStatistical(t, v.name, acc)
				}
				rows[i] = acc.row(v.name)
			})
		}
	})

	header := "| Variant | Every pair exactly 10 | Anyone above 10 | Max pending | Evenness raw | Blowouts/season | Top-4 v bottom-4/season | Reliability | Wrong pairs |\n" +
		"|---|---|---|---|---|---|---|---|---|\n"
	t.Logf("\n%s%s", header, strings.Join(rows, "\n"))
}

// assertStatistical checks wide-margin statistical assertions (only at ≥100 seasons).
func assertStatistical(t *testing.T, name string, acc *simMetrics) {
	t.Helper()
	assert.Equal(t, acc.seasons, acc.exact,
		"%s: exact-target rate must be 100%% under ffactor's exact completability check (got %d/%d)",
		name, acc.exact, acc.seasons)
}
