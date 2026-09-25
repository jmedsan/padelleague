package league

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

func truncateToNoonUTC(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 12, 0, 0, 0, time.UTC)
}

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
	t := start.Add(time.Duration(float64(end.Sub(start)) * fraction))
	return truncateToNoonUTC(t), true
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

// competitionTypeLabels is the single source of truth for a competition's
// type Spanish label — shared by the create/edit form <option>s and the
// admin activity log.
var competitionTypeLabels = map[string]string{
	"league":  "Liga",
	"playoff": "Playoff",
}

// CompetitionTypeLabel returns the Spanish label for a competition's type
// value, or the raw value if unrecognized.
func CompetitionTypeLabel(compType string) string {
	if label, ok := competitionTypeLabels[compType]; ok {
		return label
	}
	return compType
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

const defaultRecoveryDays = 7

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
	if !comp.GetBool("active") {
		return PhaseUnknown
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

// PlayerCanModify reports whether a non-admin may mutate matches in comp.
func PlayerCanModify(comp *core.Record, now time.Time) bool {
	return comp.GetBool("active") && CompetitionPhase(comp, now) != PhaseFinished
}

// PhaseOf derives the phase, exempting playoffs (which have no recovery
// window) by always returning PhaseUnknown for them.
func PhaseOf(comp *core.Record, now time.Time) Phase {
	if IsPlayoff(comp) {
		return PhaseUnknown
	}
	return CompetitionPhase(comp, now)
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

	for i := range len(rounds) - 1 {
		prev := rounds[i]
		curr := rounds[i+1]
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
	for i := range rounds {
		r := i + 1
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
				return truncateToNoonUTC(t), true
			}
		}
	}
	start := comp.GetDateTime("start_date").Time()
	end := comp.GetDateTime("end_date").Time()
	rounds := comp.GetInt("rounds")
	return RecommendedArrangeBy(start, end, rounds, roundNumber)
}

const defaultTimezone = "Atlantic/Canary"

var (
	tzOnce sync.Once
	tzLoc  *time.Location
)

// Timezone returns the league's display timezone, read once from the
// league_timezone field in app_settings. Falls back to Atlantic/Canary.
func Timezone(app core.App) *time.Location {
	tzOnce.Do(func() {
		tzLoc = loadLeagueTimezone(app)
	})
	return tzLoc
}

func loadLeagueTimezone(app core.App) *time.Location {
	fallback := mustLoadLocation(defaultTimezone)
	if app == nil {
		return fallback
	}
	recs, err := app.FindRecordsByFilter("app_settings", "", "", 1, 0, nil)
	if err != nil || len(recs) == 0 {
		return fallback
	}
	name := recs[0].GetString("league_timezone")
	if name == "" {
		return fallback
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		slog.Error("league: invalid league_timezone", "value", name, "err", err)
		return fallback
	}
	return loc
}

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
}

// Madrid is the league's display timezone (Atlantic/Canary).
// Deprecated: call Timezone(app) to read from settings; this var
// is kept for call sites not yet threaded with app.
var Madrid = mustLoadLocation(defaultTimezone)

// MatchStart combines the match's date and time fields into an instant in
// the league timezone. ok is false when either field is missing or malformed.
func MatchStart(m *core.Record) (time.Time, bool) {
	dateStr := m.GetString("date")
	if dateStr == "" {
		return time.Time{}, false
	}
	if len(dateStr) > 10 {
		dateStr = dateStr[:10]
	}
	d, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return time.Time{}, false
	}
	timeStr := m.GetString("time")
	if timeStr == "" {
		return time.Time{}, false
	}
	t, err := time.Parse("15:04", timeStr)
	if err != nil {
		return time.Time{}, false
	}
	return time.Date(d.Year(), d.Month(), d.Day(), t.Hour(), t.Minute(), 0, 0, Madrid), true
}

func fmtShortDate(t time.Time) string {
	return t.In(Madrid).Format("02/01")
}

// assignmentDeadline returns the arrange-by deadline for a newly created
// leveled-league match. Returns ok=false when target_matches is 0 or either
// date is missing.
func assignmentDeadline(comp *core.Record, now time.Time) (time.Time, bool) {
	target := comp.GetInt("target_matches")
	if target <= 0 {
		return time.Time{}, false
	}
	start := comp.GetDateTime("start_date").Time()
	end := comp.GetDateTime("end_date").Time()
	if start.IsZero() || end.IsZero() {
		return time.Time{}, false
	}
	open := comp.GetInt("open_assignments")
	slot := float64(end.Sub(start)) * float64(open) / float64(target)
	base := start
	if now.After(start) {
		base = now
	}
	deadline := base.Add(time.Duration(slot))
	if deadline.After(end) {
		deadline = end
	}
	return truncateToNoonUTC(deadline), true
}

// SlotDeadline returns the arrange-by deadline for a match at the given
// per-pair ordinal slot, dividing the competition's window into
// target_matches equal-length pace units. Returns ok=false when
// target_matches, slot, or either date is missing.
func SlotDeadline(comp *core.Record, slot int) (time.Time, bool) {
	target := comp.GetInt("target_matches")
	if target <= 0 || slot <= 0 {
		return time.Time{}, false
	}
	start := comp.GetDateTime("start_date").Time()
	end := comp.GetDateTime("end_date").Time()
	if start.IsZero() || end.IsZero() {
		return time.Time{}, false
	}
	u := end.Sub(start) / time.Duration(target)
	deadline := start.Add(time.Duration(slot) * u)
	if deadline.After(end) {
		deadline = end
	}
	return truncateToNoonUTC(deadline), true
}

// MatchArrangeDate returns the arrange-by date for a match. When the match has
// its own arrange_by field set, it is returned capped at the competition's
// end_date. Otherwise falls through to RoundArrangeDate for the match's
// round_number (round-robin and playoff path, unchanged behavior).
func MatchArrangeDate(comp *core.Record, match *core.Record) (time.Time, bool) {
	arrangeBy := match.GetDateTime("arrange_by").Time()
	if !arrangeBy.IsZero() {
		end := comp.GetDateTime("end_date").Time()
		if !end.IsZero() && arrangeBy.After(end) {
			return truncateToNoonUTC(end), true
		}
		return truncateToNoonUTC(arrangeBy), true
	}
	return RoundArrangeDate(comp, match.GetInt("round_number"))
}
