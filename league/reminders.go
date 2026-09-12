package league

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

const (
	defaultReminderHoursFirst  = 26
	defaultReminderHoursSecond = 1
	maxReminderHours           = 168
	maxReminderCount           = 5
)

// ReminderHours returns the hours-before list for a user in a competition:
// user's own list when set (may be empty = no reminders), else competition's,
// else global settings, else {26, 1}. Sorted descending, deduped.
func ReminderHours(user, comp *core.Record, settings AppSettings) []int {
	if hours, ok := userReminderHours(user); ok {
		return hours
	}
	if comp != nil {
		raw := comp.GetString("match_reminder_hours")
		if raw != "" {
			var hours []int
			if json.Unmarshal([]byte(raw), &hours) == nil && len(hours) > 0 {
				return dedupSortDesc(hours)
			}
		}
	}
	if len(settings.MatchReminderHours) > 0 {
		return dedupSortDesc(settings.MatchReminderHours)
	}
	return []int{defaultReminderHoursFirst, defaultReminderHoursSecond}
}

func userReminderHours(user *core.Record) ([]int, bool) {
	raw := user.GetString("notification_prefs")
	if raw == "" {
		return nil, false
	}
	var prefs map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &prefs) != nil {
		return nil, false
	}
	val, exists := prefs["match_reminder_hours"]
	if !exists {
		return nil, false
	}
	var floats []float64
	if json.Unmarshal(val, &floats) != nil {
		return nil, false
	}
	hours := make([]int, 0, len(floats))
	for _, f := range floats {
		h := int(f)
		if h >= 1 && h <= maxReminderHours {
			hours = append(hours, h)
		}
	}
	return dedupSortDesc(hours), true
}

// ParseReminderHours parses admin/player input "26, 1" into a validated list.
// Empty input returns an empty list with nil error.
func ParseReminderHours(raw string) ([]int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	if len(parts) > maxReminderCount {
		return nil, fmt.Errorf("máximo %d recordatorios", maxReminderCount)
	}
	var hours []int
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("%q no es un número válido", p)
		}
		if n < 1 || n > maxReminderHours {
			return nil, fmt.Errorf("las horas deben estar entre 1 y %d", maxReminderHours)
		}
		hours = append(hours, n)
	}
	return dedupSortDesc(hours), nil
}

// FormatReminderHours renders a list as "26, 1" for form prefill.
func FormatReminderHours(hours []int) string {
	parts := make([]string, len(hours))
	for i, h := range hours {
		parts[i] = strconv.Itoa(h)
	}
	return strings.Join(parts, ", ")
}

// DueReminderHours returns the tiers due now: every h in hours where
// untilStart <= h*hour and untilStart > 0.
func DueReminderHours(hours []int, untilStart float64) []int {
	if untilStart <= 0 {
		return nil
	}
	var due []int
	for _, h := range hours {
		if untilStart <= float64(h) {
			due = append(due, h)
		}
	}
	return due
}

// ClearMatchReminders deletes all reminder log rows for a match.
func ClearMatchReminders(app core.App, matchID string) {
	rows, err := app.FindRecordsByFilter("match_reminders",
		"match = {:mid}", "", 0, 0, map[string]any{"mid": matchID})
	if err != nil {
		return
	}
	for _, r := range rows {
		_ = app.Delete(r)
	}
}

func dedupSortDesc(hours []int) []int {
	seen := make(map[int]struct{}, len(hours))
	result := make([]int, 0, len(hours))
	for _, h := range hours {
		if _, ok := seen[h]; !ok {
			seen[h] = struct{}{}
			result = append(result, h)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(result)))
	return slices.Clip(result)
}
