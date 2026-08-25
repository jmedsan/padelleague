package league

import (
	"fmt"
	"sort"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// Warning represents how urgently a match needs to be arranged.
type Warning int

// Warning levels for match scheduling deadlines.
const (
	WarnNone    Warning = 0
	WarnHeadsUp Warning = 1
	WarnUrgent  Warning = 2
	WarnOverdue Warning = 3
)

const (
	headsUpStartDays = 5
	urgentStartDays  = 1
)

func (w Warning) String() string {
	switch w {
	case WarnHeadsUp:
		return "heads_up"
	case WarnUrgent:
		return "urgent"
	case WarnOverdue:
		return "overdue"
	default:
		return ""
	}
}

// Label returns the Spanish UI label for the warning level.
func (w Warning) Label() string {
	switch w {
	case WarnHeadsUp:
		return "Próximo"
	case WarnUrgent:
		return "Urgente"
	case WarnOverdue:
		return "Vencido"
	default:
		return ""
	}
}

// RecommendedArrangeBy returns the suggested deadline for a given round.
// roundNumber is 1-based. Returns ok=false if start or end is zero.
func RecommendedArrangeBy(start, end time.Time, rounds, roundNumber int) (time.Time, bool) {
	if start.IsZero() || end.IsZero() {
		return time.Time{}, false
	}
	fraction := float64(roundNumber) / float64(rounds)
	return start.Add(time.Duration(float64(end.Sub(start)) * fraction)), true
}

// WarningLevel returns the warning severity for a match based on its
// recommended deadline, a grace period, and the current time.
func WarningLevel(recommendedBy time.Time, graceDays int, now time.Time) Warning {
	overdue := recommendedBy.AddDate(0, 0, graceDays)
	if now.After(overdue) {
		return WarnOverdue
	}
	urgent := recommendedBy.AddDate(0, 0, -urgentStartDays)
	if !now.Before(urgent) {
		return WarnUrgent
	}
	headsUp := recommendedBy.AddDate(0, 0, -headsUpStartDays)
	if !now.Before(headsUp) {
		return WarnHeadsUp
	}
	return WarnNone
}

// IsPlayoff reports whether the competition is a playoff bracket.
func IsPlayoff(comp *core.Record) bool {
	return comp.GetString("type") == "playoff"
}

// ValidatePlayoffDates checks that match dates in later rounds are not
// earlier than dates in preceding rounds. Matches with no date are ignored.
func ValidatePlayoffDates(matches []*core.Record) error {
	type roundDate struct {
		round int
		date  time.Time
	}
	var dated []roundDate
	for _, m := range matches {
		d := m.GetDateTime("date").Time()
		if d.IsZero() {
			continue
		}
		dated = append(dated, roundDate{
			round: m.GetInt("round_number"),
			date:  d,
		})
	}

	minByRound := make(map[int]time.Time)
	maxByRound := make(map[int]time.Time)
	for _, rd := range dated {
		if existing, ok := minByRound[rd.round]; !ok || rd.date.Before(existing) {
			minByRound[rd.round] = rd.date
		}
		if existing, ok := maxByRound[rd.round]; !ok || rd.date.After(existing) {
			maxByRound[rd.round] = rd.date
		}
	}

	rounds := make([]int, 0, len(maxByRound))
	for r := range maxByRound {
		rounds = append(rounds, r)
	}
	sort.Ints(rounds)

	for i := 1; i < len(rounds); i++ {
		prev := rounds[i-1]
		curr := rounds[i]
		if maxByRound[prev].After(minByRound[curr]) {
			return fmt.Errorf("las fechas de la ronda %d no pueden ser anteriores a las de la ronda %d", curr, prev)
		}
	}

	return nil
}
