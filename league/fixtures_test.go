package league

import (
	"fmt"
	"testing"

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
