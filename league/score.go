package league

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// ParseMode selects how strict ParseScoreMode is about the last set.
type ParseMode int

const (
	Complete     ParseMode = iota + 1
	AllowOpenSet
)

// Score holds parsed set and game totals for a padel match.
type Score struct {
	Sets1  int
	Sets2  int
	Games1 int
	Games2 int
	Open   *OpenSet
	// CompletedSets holds each finished set's games in order.
	CompletedSets [][2]int
}

// OpenSet holds the games of an unfinished last set.
type OpenSet struct{ G1, G2 int }

var tiebreakRe = regexp.MustCompile(`\(\d+\)`)

// ParseScore parses a complete padel score (2-3 finished sets, a 2-set winner).
func ParseScore(score string) (Score, error) { return ParseScoreMode(score, Complete) }

// ParseScoreMode parses a padel score string. In AllowOpenSet mode the last set
// may be unfinished (0 ≤ a,b ≤ 6, not a legal finished set, ties allowed).
func ParseScoreMode(score string, mode ParseMode) (Score, error) {
	score = strings.TrimSpace(score)
	if score == "" {
		return Score{}, fmt.Errorf("empty score")
	}
	if strings.EqualFold(score, "WO") {
		return Score{}, nil
	}

	score = tiebreakRe.ReplaceAllString(score, "")
	parts := strings.Fields(score)
	if len(parts) == 0 || len(parts) > 3 {
		return Score{}, fmt.Errorf("invalid number of sets: %d", len(parts))
	}

	var s Score
	for i, part := range parts {
		isLast := i == len(parts)-1

		g1, g2, err := parseSet(part)
		if err != nil {
			if !isLast || mode != AllowOpenSet {
				return Score{}, err
			}
			g1, g2, err = parseOpenSet(part)
			if err != nil {
				return Score{}, err
			}
			if g1 == 0 && g2 == 0 {
				continue // trailing 0-0 dropped
			}
			if s.Sets1 == 2 || s.Sets2 == 2 {
				return Score{}, fmt.Errorf("match already won, open set not allowed: %q", part)
			}
			s.Open = &OpenSet{G1: g1, G2: g2}
			s.Games1 += g1
			s.Games2 += g2
			continue
		}
		s.Games1 += g1
		s.Games2 += g2
		s.CompletedSets = append(s.CompletedSets, [2]int{g1, g2})
		if g1 > g2 {
			s.Sets1++
		} else {
			s.Sets2++
		}
	}

	if mode == Complete {
		if s.Sets1+s.Sets2 < 2 {
			return Score{}, fmt.Errorf("invalid number of sets: %d", s.Sets1+s.Sets2)
		}
		if s.Sets1 != 2 && s.Sets2 != 2 {
			return Score{}, fmt.Errorf("winner must have exactly 2 sets")
		}
	} else {
		total := s.Sets1 + s.Sets2
		if s.Open != nil {
			total++
		}
		if total == 0 {
			return Score{}, fmt.Errorf("empty score")
		}
	}

	return s, nil
}

func parseOpenSet(part string) (int, int, error) {
	halves := strings.SplitN(part, "-", 2)
	if len(halves) != 2 {
		return 0, 0, fmt.Errorf("invalid set format: %q", part)
	}
	g1, err1 := strconv.Atoi(strings.TrimSpace(halves[0]))
	g2, err2 := strconv.Atoi(strings.TrimSpace(halves[1]))
	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("invalid score numbers in %q", part)
	}
	if g1 < 0 || g2 < 0 || g1 > 6 || g2 > 6 {
		return 0, 0, fmt.Errorf("invalid open set score: %q", part)
	}
	return g1, g2, nil
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
// Returns ErrNoWinner when the score is valid but has no winner (open set
// without a 3-game lead or without a completed set already won).
func DetermineWinner(match *core.Record, score string) (string, error) {
	if strings.EqualFold(strings.TrimSpace(score), "WO") {
		return "", fmt.Errorf("walkover requires manual winner selection")
	}

	sc, err := ParseScoreMode(score, AllowOpenSet)
	if err != nil {
		return "", err
	}

	out := EvaluateScore(sc)
	if !out.Won {
		return "", ErrNoWinner
	}

	if out.WinnerSide == 1 {
		return match.GetString("pair1"), nil
	}
	return match.GetString("pair2"), nil
}
