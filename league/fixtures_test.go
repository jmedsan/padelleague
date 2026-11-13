package league

import (
	"fmt"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoundRobin_AllPairsPlayOnce(t *testing.T) {
	t.Parallel()
	for n := 2; n <= 8; n++ {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			pairIDs := make([]string, n)
			for i := range pairIDs {
				pairIDs[i] = fmt.Sprintf("p%d", i+1)
			}

			rounds := RoundRobin(pairIDs, false)
			require.NotEmpty(t, rounds)

			matchups := map[string]int{}
			for _, r := range rounds {
				for _, m := range r.Matches {
					key := m.Home + ":" + m.Away
					if m.Home > m.Away {
						key = m.Away + ":" + m.Home
					}
					matchups[key]++
				}
			}

			expectedPairs := n * (n - 1) / 2
			assert.Equal(t, expectedPairs, len(matchups), "should have all unique pairings")

			for key, count := range matchups {
				assert.Equal(t, 1, count, "pair %s should play exactly once", key)
			}
		})
	}
}

func TestRoundRobin_Double(t *testing.T) {
	t.Parallel()
	pairIDs := []string{"p1", "p2", "p3", "p4"}

	rounds := RoundRobin(pairIDs, true)
	require.NotEmpty(t, rounds)

	matchups := map[string]int{}
	for _, r := range rounds {
		for _, m := range r.Matches {
			key := m.Home + ":" + m.Away
			if m.Home > m.Away {
				key = m.Away + ":" + m.Home
			}
			matchups[key]++
		}
	}

	expectedPairs := 4 * 3 / 2
	assert.Equal(t, expectedPairs, len(matchups))

	for key, count := range matchups {
		assert.Equal(t, 2, count, "pair %s should play exactly twice", key)
	}

	// Round numbers must run 1..N with no gaps or repeats: the return leg
	// continues the numbering rather than restarting or colliding with the
	// first leg, which would scramble the fixture order.
	var numbers []int
	for _, r := range rounds {
		numbers = append(numbers, r.Number)
	}
	expected := make([]int, len(rounds))
	for i := range expected {
		expected[i] = i + 1
	}
	assert.Equal(t, expected, numbers)
}

func TestRoundRobin_NoPairTwicePerRound(t *testing.T) {
	t.Parallel()
	for n := 2; n <= 8; n++ {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			pairIDs := make([]string, n)
			for i := range pairIDs {
				pairIDs[i] = fmt.Sprintf("p%d", i+1)
			}

			rounds := RoundRobin(pairIDs, false)

			for _, r := range rounds {
				seen := map[string]bool{}
				for _, m := range r.Matches {
					assert.False(t, seen[m.Home], "round %d: %s appears twice", r.Number, m.Home)
					assert.False(t, seen[m.Away], "round %d: %s appears twice", r.Number, m.Away)
					seen[m.Home] = true
					seen[m.Away] = true
				}
			}
		})
	}
}

func TestRoundRobin_TwoPairs(t *testing.T) {
	t.Parallel()
	rounds := RoundRobin([]string{"a", "b"}, false)
	require.Len(t, rounds, 1)
	require.Len(t, rounds[0].Matches, 1)
	assert.Equal(t, 1, rounds[0].Number)
}

func TestRoundRobin_OddNumber(t *testing.T) {
	t.Parallel()
	rounds := RoundRobin([]string{"a", "b", "c"}, false)
	require.NotEmpty(t, rounds)

	matchups := map[string]int{}
	for _, r := range rounds {
		for _, m := range r.Matches {
			key := m.Home + ":" + m.Away
			if m.Home > m.Away {
				key = m.Away + ":" + m.Home
			}
			matchups[key]++
		}
	}
	assert.Equal(t, 3, len(matchups), "3 pairs = 3 unique matchups")
}

func genPairIDs(n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("p%d", i+1)
	}
	return ids
}

// maxStreak returns the longest run of consecutive rounds in which every
// pair played the same side (all-home or all-away), across the whole
// schedule in round order.
func maxStreak(rounds []Round, pairID string) int {
	var sides []bool // true = home
	for _, r := range rounds {
		for _, m := range r.Matches {
			switch pairID {
			case m.Home:
				sides = append(sides, true)
			case m.Away:
				sides = append(sides, false)
			}
		}
	}
	best, cur := 0, 0
	for i, side := range sides {
		if i == 0 || side == sides[i-1] {
			cur++
		} else {
			cur = 1
		}
		if cur > best {
			best = cur
		}
	}
	return best
}

// homeAwayImbalance returns |home count - away count| for one pair across
// the whole schedule.
func homeAwayImbalance(rounds []Round, pairID string) int {
	home, away := 0, 0
	for _, r := range rounds {
		for _, m := range r.Matches {
			switch pairID {
			case m.Home:
				home++
			case m.Away:
				away++
			}
		}
	}
	diff := home - away
	if diff < 0 {
		diff = -diff
	}
	return diff
}

func TestRoundRobin_MaxStreakAndImbalance(t *testing.T) {
	t.Parallel()
	for n := 2; n <= 16; n++ {
		for _, double := range []bool{false, true} {
			t.Run(fmt.Sprintf("n=%d/double=%v", n, double), func(t *testing.T) {
				ids := genPairIDs(n)
				rounds := RoundRobin(ids, double)
				require.NotEmpty(t, rounds)

				wantImbalance := 1
				if double {
					wantImbalance = 0
				}
				for _, id := range ids {
					assert.LessOrEqualf(t, maxStreak(rounds, id), 2,
						"pair %s: home/away streak must be at most 2 (n=%d, double=%v)", id, n, double)
					assert.LessOrEqualf(t, homeAwayImbalance(rounds, id), wantImbalance,
						"pair %s: home/away imbalance must be at most %d (n=%d, double=%v)", id, wantImbalance, n, double)
				}
			})
		}
	}
}

func TestRoundRobin_DoubleEveryOrderedPairOnce(t *testing.T) {
	t.Parallel()
	for n := 2; n <= 16; n++ {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			ids := genPairIDs(n)
			rounds := RoundRobin(ids, true)
			require.NotEmpty(t, rounds)

			ordered := map[string]int{}
			for _, r := range rounds {
				for _, m := range r.Matches {
					ordered[m.Home+">"+m.Away]++
				}
			}
			for _, home := range ids {
				for _, away := range ids {
					if home == away {
						continue
					}
					assert.Equal(t, 1, ordered[home+">"+away],
						"%s at home vs %s away must occur exactly once", home, away)
				}
			}
		})
	}
}

func TestRoundRobin_DoubleNoConsecutiveRepeat(t *testing.T) {
	t.Parallel()
	// n=2 has only one possible matchup and one match per round, so the
	// leg boundary is unavoidably a repeat — start at n=3, where there's
	// another pair the rotation could interleave with instead.
	for n := 3; n <= 16; n++ {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			ids := genPairIDs(n)
			rounds := RoundRobin(ids, true)
			require.NotEmpty(t, rounds)

			for i := 1; i < len(rounds); i++ {
				prevPairs := map[string]bool{}
				for _, m := range rounds[i-1].Matches {
					prevPairs[unorderedKey(m)] = true
				}
				for _, m := range rounds[i].Matches {
					assert.Falsef(t, prevPairs[unorderedKey(m)],
						"round %d repeats round %d's matchup %s vs %s (n=%d)",
						rounds[i].Number, rounds[i-1].Number, m.Home, m.Away, n)
				}
			}
		})
	}
}

func unorderedKey(m RoundMatch) string {
	if m.Home > m.Away {
		return m.Away + ":" + m.Home
	}
	return m.Home + ":" + m.Away
}

// A withdrawn seed is flag-only: it advances and the match stays pending, seeds untouched.
func TestSettleSeeds_WithdrawnSeedAdvances(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	w := makePair(t, app, "Retirada")
	p := makePair(t, app, "Viva")
	comp := makeCompetition(t, app, []*core.Record{w, p})
	comp.Set("withdrawn_pairs", []string{w.Id})
	require.NoError(t, app.Save(comp))

	nm := makeMatch(t, app, comp.Id, w.Id, p.Id, "pending")
	require.NoError(t, settleSeeds(app, nm))

	assert.Equal(t, w.Id, nm.GetString("pair1"))
	assert.Equal(t, p.Id, nm.GetString("pair2"))
	assert.Equal(t, "pending", nm.GetString("status"))
	assert.Empty(t, nm.GetString("winner"))
}

// A single seeded pair wins by walkover over the empty side.
func TestSettleSeeds_OneEmptySideWalksOver(t *testing.T) {
	t.Parallel()
	for _, emptyIs1 := range []bool{true, false} {
		app := newTestApp(t)
		p := makePair(t, app, "Viva")
		q := makePair(t, app, "Otra")
		comp := makeCompetition(t, app, []*core.Record{p, q})
		nm := makeMatch(t, app, comp.Id, p.Id, q.Id, "pending")
		if emptyIs1 {
			nm.Set("pair1", "")
			nm.Set("pair2", p.Id)
		} else {
			nm.Set("pair2", "")
		}
		require.NoError(t, settleSeeds(app, nm))
		assert.Equal(t, "final", nm.GetString("status"))
		assert.Equal(t, p.Id, nm.GetString("winner"))
		assert.Equal(t, "walkover", nm.GetString("review_type"))
	}
}

// A match with no seeded pair at all is void: final, no winner, no score.
func TestSettleSeeds_BothEmptyVoids(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p := makePair(t, app, "Viva")
	q := makePair(t, app, "Otra")
	comp := makeCompetition(t, app, []*core.Record{p, q})
	nm := makeMatch(t, app, comp.Id, p.Id, q.Id, "pending")
	nm.Set("pair1", "")
	nm.Set("pair2", "")
	require.NoError(t, settleSeeds(app, nm))

	assert.Equal(t, "final", nm.GetString("status"))
	assert.Empty(t, nm.GetString("winner"))
	assert.Empty(t, nm.GetString("scores"))
	assert.Empty(t, nm.GetString("review_type"))
}

// PlayoffMaxRound is ok only for a playoff competition that already has matches.
func TestPlayoffMaxRound(t *testing.T) {
	t.Parallel()
	t.Run("league competition is not ok, even with matches", func(t *testing.T) {
		t.Parallel()
		app := newTestApp(t)
		p1 := makePair(t, app, "A")
		p2 := makePair(t, app, "B")
		comp := makeCompetition(t, app, []*core.Record{p1, p2})
		makeMatchRound(t, app, comp.Id, p1.Id, p2.Id, 1)

		maxRound, ok := PlayoffMaxRound(app, comp)
		assert.False(t, ok)
		assert.Equal(t, 0, maxRound)
	})
	t.Run("playoff competition with no matches is not ok", func(t *testing.T) {
		t.Parallel()
		app := newTestApp(t)
		p1 := makePair(t, app, "A")
		p2 := makePair(t, app, "B")
		comp := makeCompetition(t, app, []*core.Record{p1, p2})
		comp.Set("type", "playoff")
		require.NoError(t, app.Save(comp))

		maxRound, ok := PlayoffMaxRound(app, comp)
		assert.False(t, ok)
		assert.Equal(t, 0, maxRound)
	})
	t.Run("playoff competition reports its last round, whatever the insert order", func(t *testing.T) {
		t.Parallel()
		app := newTestApp(t)
		p := []*core.Record{makePair(t, app, "A"), makePair(t, app, "B"), makePair(t, app, "C"), makePair(t, app, "D")}
		comp := makeCompetition(t, app, p)
		comp.Set("type", "playoff")
		require.NoError(t, app.Save(comp))
		makeMatchRound(t, app, comp.Id, p[2].Id, p[3].Id, 2)
		makeMatchRound(t, app, comp.Id, p[0].Id, p[2].Id, 3)
		makeMatchRound(t, app, comp.Id, p[1].Id, p[3].Id, 1)

		maxRound, ok := PlayoffMaxRound(app, comp)
		assert.True(t, ok)
		assert.Equal(t, 3, maxRound)
	})
}

// Planned repeats exist only in double legs: single leg meets every pair once,
// double meets every pair exactly twice (once per venue side), and home/away
// stays balanced (within one for single, exactly even for double).
func TestRoundRobin_PlannedRepeatsAndBalance(t *testing.T) {
	t.Parallel()
	for n := 3; n <= 10; n++ {
		for _, double := range []bool{false, true} {
			t.Run(fmt.Sprintf("n=%d/double=%v", n, double), func(t *testing.T) {
				ids := genPairIDs(n)
				meets := map[string]int{}
				home := map[string]int{}
				for _, r := range RoundRobin(ids, double) {
					for _, m := range r.Matches {
						meets[unorderedKey(m)]++
						home[m.Home]++
					}
				}
				want := 1
				if double {
					want = 2
				}
				require.Len(t, meets, n*(n-1)/2)
				for k, c := range meets {
					assert.Equal(t, want, c, "pairing %s", k)
				}
				for _, id := range ids {
					games := want * (n - 1)
					assert.LessOrEqual(t, abs(2*home[id]-games), 1, "pair %s: %d home of %d", id, home[id], games)
					if double {
						assert.Equal(t, n-1, home[id], "double leg is exactly even")
					}
				}
			})
		}
	}
}
