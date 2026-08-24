package league

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// Score holds parsed set and game totals for a padel match.
type Score struct {
	Sets1  int
	Sets2  int
	Games1 int
	Games2 int
}

var tiebreakRe = regexp.MustCompile(`\(\d+\)`)

// ParseScore parses a padel score string into set and game totals.
func ParseScore(score string) (Score, error) {
	score = strings.TrimSpace(score)
	if score == "" {
		return Score{}, fmt.Errorf("empty score")
	}
	if strings.EqualFold(score, "WO") {
		return Score{}, nil
	}

	score = tiebreakRe.ReplaceAllString(score, "")
	parts := strings.Fields(score)

	var s Score
	for _, part := range parts {
		g1, g2, err := parseSet(part)
		if err != nil {
			return Score{}, err
		}
		s.Games1 += g1
		s.Games2 += g2
		if g1 > g2 {
			s.Sets1++
		} else {
			s.Sets2++
		}
	}

	numSets := len(parts)
	if numSets < 2 || numSets > 3 {
		return Score{}, fmt.Errorf("invalid number of sets: %d", numSets)
	}
	if s.Sets1 != 2 && s.Sets2 != 2 {
		return Score{}, fmt.Errorf("winner must have exactly 2 sets")
	}

	return s, nil
}

func parseSet(part string) (int, int, error) {
	halves := strings.SplitN(part, "-", 2)
	if len(halves) != 2 {
		return 0, 0, fmt.Errorf("invalid set format: %q", part)
	}
	g1, err1 := strconv.Atoi(strings.TrimSpace(halves[0]))
	g2, err2 := strconv.Atoi(strings.TrimSpace(halves[1]))
	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("invalid score numbers in %q", part)
	}
	if g1 < 0 || g2 < 0 {
		return 0, 0, fmt.Errorf("negative numbers not allowed: %q", part)
	}
	if g1 == g2 {
		return 0, 0, fmt.Errorf("tied set not allowed: %q", part)
	}
	winner, loser := g1, g2
	if g2 > g1 {
		winner, loser = g2, g1
	}
	if winner < 6 || winner > 7 {
		return 0, 0, fmt.Errorf("invalid set score: %q", part)
	}
	if winner == 7 && loser != 5 && loser != 6 {
		return 0, 0, fmt.Errorf("invalid set score: %q", part)
	}
	if winner == 6 && loser > 4 {
		return 0, 0, fmt.Errorf("invalid set score: %q", part)
	}
	return g1, g2, nil
}

// DetermineWinner returns the pair ID of the match winner based on the score.
func DetermineWinner(match *core.Record, score string) (string, error) {
	if strings.EqualFold(strings.TrimSpace(score), "WO") {
		return "", fmt.Errorf("walkover requires manual winner selection")
	}

	s, err := ParseScore(score)
	if err != nil {
		return "", err
	}

	if s.Sets1 > s.Sets2 {
		return match.GetString("pair1"), nil
	}
	return match.GetString("pair2"), nil
}
