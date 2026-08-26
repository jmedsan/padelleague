package league

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
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
// roundNumber is 1-based. Returns ok=false if start/end is zero or rounds < 1.
func RecommendedArrangeBy(start, end time.Time, rounds, roundNumber int) (time.Time, bool) {
	if start.IsZero() || end.IsZero() || rounds < 1 {
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

// Phase represents where a round-robin competition sits in its lifecycle.
type Phase int

// Competition lifecycle phases. PhaseUnknown is never a derived value; it
// guards against an uncomputed zero value reading as an actionable phase.
const (
	PhaseUnknown Phase = iota
	PhasePlaying
	PhaseRecovery
	PhaseFinished
)

const defaultRecoveryDays = 14

func (p Phase) String() string {
	switch p {
	case PhasePlaying:
		return "playing"
	case PhaseRecovery:
		return "recovery"
	case PhaseFinished:
		return "finished"
	default:
		return ""
	}
}

// Label returns the Spanish UI label for the phase.
func (p Phase) Label() string {
	switch p {
	case PhasePlaying:
		return "En juego"
	case PhaseRecovery:
		return "En recuperación"
	case PhaseFinished:
		return "Finalizada"
	default:
		return ""
	}
}

// RecoveryDays returns the competition's recovery window length, defaulting
// to 14 when unset or explicitly 0.
func RecoveryDays(comp *core.Record) int {
	if days := comp.GetInt("recovery_days"); days > 0 {
		return days
	}
	return defaultRecoveryDays
}

// CompetitionPhase derives a round-robin competition's lifecycle phase from
// its finalized flag, end_date, and recovery window. Never stored; recomputed
// on every read.
func CompetitionPhase(comp *core.Record, now time.Time) Phase {
	if comp.GetBool("finalized") {
		return PhaseFinished
	}
	end := comp.GetDateTime("end_date").Time()
	if end.IsZero() {
		return PhasePlaying
	}
	if !now.After(end) {
		return PhasePlaying
	}
	if !now.After(end.AddDate(0, 0, RecoveryDays(comp))) {
		return PhaseRecovery
	}
	return PhaseFinished
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

// BuildRoundSchedule returns round-number → arrange-by date for a round-robin
// competition. Empty when start/end zero or rounds < 1.
func BuildRoundSchedule(start, end time.Time, rounds int) map[int]time.Time {
	if start.IsZero() || end.IsZero() || rounds < 1 {
		return nil
	}
	schedule := make(map[int]time.Time, rounds)
	for r := 1; r <= rounds; r++ {
		d, _ := RecommendedArrangeBy(start, end, rounds, r)
		schedule[r] = d
	}
	return schedule
}

// StoreRoundSchedule builds the schedule and marshals it to the JSON string
// stored in competitions.round_arrange_dates. Empty string when nothing to store.
func StoreRoundSchedule(start, end time.Time, rounds int) string {
	sched := BuildRoundSchedule(start, end, rounds)
	if len(sched) == 0 {
		return ""
	}
	b, err := json.Marshal(sched)
	if err != nil {
		return ""
	}
	return string(b)
}

// RoundArrangeDate returns the stored arrange-by date for a round. Falls back
// to RecommendedArrangeBy on empty/error/absent key. Returns ok=false when
// neither stored nor valid fallback exists.
func RoundArrangeDate(comp *core.Record, roundNumber int) (time.Time, bool) {
	raw := comp.GetString("round_arrange_dates")
	if raw != "" {
		var stored map[string]time.Time
		if json.Unmarshal([]byte(raw), &stored) == nil {
			key := strconv.Itoa(roundNumber)
			if t, ok := stored[key]; ok {
				return t, true
			}
		}
	}
	start := comp.GetDateTime("start_date").Time()
	end := comp.GetDateTime("end_date").Time()
	rounds := comp.GetInt("rounds")
	return RecommendedArrangeBy(start, end, rounds, roundNumber)
}
