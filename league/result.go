package league

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNoWinner indicates the score is valid but no side has won yet.
var ErrNoWinner = errors.New("league: no winner")

// Outcome is what an accepted score means for the match.
type Outcome struct {
	Won        bool
	WinnerSide int
	Carried    string
}

// EvaluateScore applies the league rule to a parsed score: a full 2-set winner
// wins; an open-set leader wins only when ahead by 3+ games AND already holding
// one completed set. Anything else is "no terminado".
func EvaluateScore(sc Score) Outcome {
	if sc.Sets1 == 2 {
		return Outcome{Won: true, WinnerSide: 1}
	}
	if sc.Sets2 == 2 {
		return Outcome{Won: true, WinnerSide: 2}
	}

	carried := serializeCompletedSets(sc)

	if sc.Open != nil {
		diff := sc.Open.G1 - sc.Open.G2
		if diff >= 3 && sc.Sets1 >= 1 {
			return Outcome{Won: true, WinnerSide: 1}
		}
		if diff <= -3 && sc.Sets2 >= 1 {
			return Outcome{Won: true, WinnerSide: 2}
		}
	}

	return Outcome{Won: false, Carried: carried}
}

// ScoreNote returns an annotation for a score: empty for complete scores,
// a descriptive note for unfinished ones.
func ScoreNote(score string) string {
	sc, err := ParseScoreMode(score, AllowOpenSet)
	if err != nil {
		return ""
	}
	out := EvaluateScore(sc)
	if out.Won && sc.Open == nil {
		return ""
	}
	if out.Won {
		return "No terminado · finalizado por la regla de los 3 juegos"
	}
	if sc.Sets1+sc.Sets2 > 0 || sc.Open != nil {
		return "No terminado · se reanudará otro día"
	}
	return ""
}

// TallyScore parses a stored final score and returns per-side sets/games with
// the open set (if any) awarded to its leader. ok is false for walkovers and
// unparsable scores.
func TallyScore(score string) (Score, bool) {
	sc, err := ParseScoreMode(score, AllowOpenSet)
	if err != nil || (sc.Sets1 == 0 && sc.Sets2 == 0 && sc.Open == nil) {
		return Score{}, false
	}
	if sc.Open != nil {
		if sc.Open.G1 > sc.Open.G2 {
			sc.Sets1++
		} else if sc.Open.G2 > sc.Open.G1 {
			sc.Sets2++
		}
		sc.Open = nil
	}
	return sc, true
}

func serializeCompletedSets(sc Score) string {
	if len(sc.CompletedSets) == 0 {
		return ""
	}
	parts := make([]string, len(sc.CompletedSets))
	for i, s := range sc.CompletedSets {
		parts[i] = fmt.Sprintf("%d-%d", s[0], s[1])
	}
	return strings.Join(parts, " ")
}
