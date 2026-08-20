package handlers

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

var tiebreakRe = regexp.MustCompile(`\(\d+\)`)

func parseScore(score string) (sets1, sets2, games1, games2 int, err error) {
	score = strings.TrimSpace(score)
	if score == "" {
		return 0, 0, 0, 0, fmt.Errorf("empty score")
	}
	if strings.EqualFold(score, "WO") {
		return 0, 0, 0, 0, nil
	}

	score = tiebreakRe.ReplaceAllString(score, "")
	parts := strings.Fields(score)

	for _, part := range parts {
		halves := strings.SplitN(part, "-", 2)
		if len(halves) != 2 {
			return 0, 0, 0, 0, fmt.Errorf("invalid set format: %q", part)
		}
		g1, err1 := strconv.Atoi(strings.TrimSpace(halves[0]))
		g2, err2 := strconv.Atoi(strings.TrimSpace(halves[1]))
		if err1 != nil || err2 != nil {
			return 0, 0, 0, 0, fmt.Errorf("invalid score numbers in %q", part)
		}
		if g1 < 0 || g2 < 0 {
			return 0, 0, 0, 0, fmt.Errorf("negative numbers not allowed: %q", part)
		}
		games1 += g1
		games2 += g2
		if g1 > g2 {
			sets1++
		} else if g2 > g1 {
			sets2++
		} else {
			return 0, 0, 0, 0, fmt.Errorf("tied set not allowed: %q", part)
		}
	}

	if sets1 == sets2 {
		return 0, 0, 0, 0, fmt.Errorf("tied match: %d-%d sets", sets1, sets2)
	}

	return sets1, sets2, games1, games2, nil
}

func determineWinner(partido *core.Record, score string) (string, error) {
	if strings.EqualFold(strings.TrimSpace(score), "WO") {
		return "", fmt.Errorf("walkover requires manual winner selection")
	}

	sets1, sets2, _, _, err := parseScore(score)
	if err != nil {
		return "", err
	}

	if sets1 > sets2 {
		return partido.GetString("pareja1"), nil
	}
	return partido.GetString("pareja2"), nil
}
