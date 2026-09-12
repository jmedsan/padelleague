package league

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchStart(t *testing.T) {
	app := newTestApp(t)
	p1 := makePair(t, app, "A")
	p2 := makePair(t, app, "B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	tests := []struct {
		name     string
		date     string
		timeStr  string
		wantOK   bool
		wantHour int
		wantMin  int
	}{
		{"full datetime", "2026-09-14", "18:00", true, 18, 0},
		{"date with timestamp suffix", "2026-09-14 00:00:00.000Z", "09:30", true, 9, 30},
		{"empty date", "", "18:00", false, 0, 0},
		{"empty time", "2026-09-14", "", false, 0, 0},
		{"bad time format", "2026-09-14", "18h", false, 0, 0},
		{"bad time accepts partial", "2026-09-14", "18:0x", false, 0, 0},
		{"DST spring forward", "2026-03-29", "02:30", true, 3, 30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, StatusPending)
			m.Set("date", tt.date)
			m.Set("time", tt.timeStr)
			require.NoError(t, app.Save(m))

			got, ok := MatchStart(m)
			assert.Equal(t, tt.wantOK, ok)
			if ok {
				assert.Equal(t, tt.wantHour, got.Hour())
				assert.Equal(t, tt.wantMin, got.Minute())
				assert.Equal(t, Madrid, got.Location())
			}
		})
	}
}

func TestParseReminderHours(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    []int
		wantErr bool
	}{
		{"empty", "", nil, false},
		{"spaces only", "   ", nil, false},
		{"single", "26", []int{26}, false},
		{"two values", "26, 1", []int{26, 1}, false},
		{"dedup", "26, 1, 26", []int{26, 1}, false},
		{"unsorted input", "1, 26, 12", []int{26, 12, 1}, false},
		{"over max hours", "200", nil, true},
		{"over max count", "1, 2, 3, 4, 5, 6", nil, true},
		{"non-number", "abc", nil, true},
		{"zero", "0", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseReminderHours(tt.raw)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestReminderHours_Precedence(t *testing.T) {
	app := newTestApp(t)

	settings := LoadSettings(app)

	makeUserWithPrefs := func(t *testing.T, prefs map[string]any) *core.Record {
		t.Helper()
		u := makeUser(t, app, "Test", "")
		if prefs != nil {
			b, _ := json.Marshal(prefs)
			u.Set("notification_prefs", string(b))
			require.NoError(t, app.Save(u))
		}
		return u
	}

	compWithHours := func(t *testing.T, hours []int) *core.Record {
		t.Helper()
		p1 := makePair(t, app, "X")
		p2 := makePair(t, app, "Y")
		comp := makeCompetition(t, app, []*core.Record{p1, p2})
		if hours != nil {
			b, _ := json.Marshal(hours)
			comp.Set("match_reminder_hours", string(b))
			require.NoError(t, app.Save(comp))
		}
		return comp
	}

	t.Run("hardcoded default", func(t *testing.T) {
		u := makeUserWithPrefs(t, nil)
		got := ReminderHours(u, nil, AppSettings{})
		assert.Equal(t, []int{26, 1}, got)
	})

	t.Run("global settings", func(t *testing.T) {
		u := makeUserWithPrefs(t, nil)
		s := settings
		s.MatchReminderHours = []int{48, 2}
		got := ReminderHours(u, nil, s)
		assert.Equal(t, []int{48, 2}, got)
	})

	t.Run("competition overrides global", func(t *testing.T) {
		u := makeUserWithPrefs(t, nil)
		comp := compWithHours(t, []int{12, 3})
		s := settings
		s.MatchReminderHours = []int{48, 2}
		got := ReminderHours(u, comp, s)
		assert.Equal(t, []int{12, 3}, got)
	})

	t.Run("user overrides competition", func(t *testing.T) {
		u := makeUserWithPrefs(t, map[string]any{"match_reminder_hours": []int{4}})
		comp := compWithHours(t, []int{12, 3})
		got := ReminderHours(u, comp, settings)
		assert.Equal(t, []int{4}, got)
	})

	t.Run("user empty list means no reminders", func(t *testing.T) {
		u := makeUserWithPrefs(t, map[string]any{"match_reminder_hours": []int{}})
		comp := compWithHours(t, []int{12, 3})
		got := ReminderHours(u, comp, settings)
		assert.Equal(t, []int{}, got)
	})
}

func TestDueReminderHours(t *testing.T) {
	tests := []struct {
		name       string
		hours      []int
		untilHours float64
		want       []int
	}{
		{"exactly 26h", []int{26, 1}, 26, []int{26}},
		{"just over 26h", []int{26, 1}, 26.001, nil},
		{"at 1h", []int{26, 1}, 1, []int{26, 1}},
		{"past start", []int{26, 1}, 0, nil},
		{"negative", []int{26, 1}, -1, nil},
		{"between tiers", []int{26, 1}, 2, []int{26}},
		{"far out", []int{26, 1}, 100, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DueReminderHours(tt.hours, tt.untilHours)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDueReminderHours_Boundary(t *testing.T) {
	hours := []int{26, 1}

	got := DueReminderHours(hours, 26.0)
	assert.Contains(t, got, 26)

	got = DueReminderHours(hours, 26.001)
	assert.NotContains(t, got, 26)

	got = DueReminderHours(hours, 1.0)
	assert.Contains(t, got, 1)

	got = DueReminderHours(hours, 1.001)
	assert.NotContains(t, got, 1)
}

func TestClearMatchReminders(t *testing.T) {
	app := newTestApp(t)
	p1 := makePair(t, app, "A")
	p2 := makePair(t, app, "B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, StatusScheduled)
	u := makeUser(t, app, "Player", "")

	col, err := app.FindCollectionByNameOrId("match_reminders")
	require.NoError(t, err)
	rec := core.NewRecord(col)
	rec.Set("match", m.Id)
	rec.Set("user", u.Id)
	rec.Set("hours_before", 26)
	require.NoError(t, app.Save(rec))

	rows, _ := app.FindRecordsByFilter("match_reminders", "match = {:mid}", "", 0, 0, map[string]any{"mid": m.Id})
	require.Len(t, rows, 1)

	ClearMatchReminders(app, m.Id)

	rows, _ = app.FindRecordsByFilter("match_reminders", "match = {:mid}", "", 0, 0, map[string]any{"mid": m.Id})
	assert.Empty(t, rows)
}

func TestMatchStart_DST(t *testing.T) {
	app := newTestApp(t)
	p1 := makePair(t, app, "A")
	p2 := makePair(t, app, "B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, StatusPending)

	m.Set("date", "2026-03-29")
	m.Set("time", "02:30")
	require.NoError(t, app.Save(m))

	got, ok := MatchStart(m)
	require.True(t, ok)

	_, offset := got.Zone()
	_ = offset
	assert.Equal(t, "Europe/Madrid", got.Location().String())
	expected := time.Date(2026, 3, 29, 2, 30, 0, 0, Madrid)
	assert.True(t, got.Equal(expected), "got %v, want %v", got, expected)
}
