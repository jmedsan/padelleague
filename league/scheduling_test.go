package league

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecommendedArrangeBy(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		start, end  time.Time
		rounds      int
		roundNumber int
		wantTime    time.Time
		wantOK      bool
	}{
		{"round 1 of 4", start, end, 4, 1, time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC), true},
		{"round 2 of 4", start, end, 4, 2, time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC), true},
		{"round 3 of 4", start, end, 4, 3, time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC), true},
		{"round 4 of 4", start, end, 4, 4, end, true},
		{"round 1 of 3 (no truncation)", start, end, 3, 1, time.Date(2026, 9, 7, 16, 0, 0, 0, time.UTC), true},
		{"zero start", time.Time{}, end, 4, 1, time.Time{}, false},
		{"zero end", start, time.Time{}, 4, 1, time.Time{}, false},
		{"both zero", time.Time{}, time.Time{}, 4, 1, time.Time{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := RecommendedArrangeBy(tt.start, tt.end, tt.rounds, tt.roundNumber)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantTime, got)
		})
	}
}

func TestWarningLevel(t *testing.T) {
	t.Parallel()
	deadline := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		now       time.Time
		graceDays int
		want      Warning
	}{
		{"well before heads-up", time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), 2, WarnNone},
		{"day before heads-up (−6)", time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC), 2, WarnNone},
		{"heads-up boundary (−5)", time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC), 2, WarnHeadsUp},
		{"inside heads-up (−3)", time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC), 2, WarnHeadsUp},
		{"day before urgent (−2)", time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC), 2, WarnHeadsUp},
		{"urgent boundary (−1)", time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC), 2, WarnUrgent},
		{"on deadline (0)", deadline, 2, WarnUrgent},
		{"grace day 1 (+1)", time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC), 2, WarnUrgent},
		{"last grace day (+2)", time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC), 2, WarnUrgent},
		{"overdue (+3)", time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC), 2, WarnOverdue},
		{"grace=0 on deadline", deadline, 0, WarnUrgent},
		{"grace=0 one day after", time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC), 0, WarnOverdue},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, WarningLevel(deadline, tt.graceDays, tt.now))
		})
	}
}

func TestWarningString(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", WarnNone.String())
	assert.Equal(t, "heads_up", WarnHeadsUp.String())
	assert.Equal(t, "urgent", WarnUrgent.String())
	assert.Equal(t, "overdue", WarnOverdue.String())
}

func TestWarningLabel(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", WarnNone.Label())
	assert.Equal(t, "Próximo", WarnHeadsUp.Label())
	assert.Equal(t, "Urgente", WarnUrgent.Label())
	assert.Equal(t, "Vencido", WarnOverdue.Label())
}

func TestIsPlayoff(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	league := makeCompetition(t, app, nil)
	assert.False(t, IsPlayoff(league))

	playoff := makePlayoffCompetition(t, app, nil)
	assert.True(t, IsPlayoff(playoff))
}

func TestValidatePlayoffDates_Valid(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "VPD-A")
	p2 := makePair(t, app, "VPD-B")
	comp := makePlayoffCompetition(t, app, []*core.Record{p1, p2})

	m1 := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "pending")
	m1.Set("round_number", 1)
	m1.Set("date", "2026-09-10 00:00:00.000Z")
	require.NoError(t, app.Save(m1))

	m2 := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "pending")
	m2.Set("round_number", 2)
	m2.Set("date", "2026-09-15 00:00:00.000Z")
	require.NoError(t, app.Save(m2))

	assert.NoError(t, ValidatePlayoffDates([]*core.Record{m1, m2}))
}

func TestValidatePlayoffDates_Invalid(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "VPD-C")
	p2 := makePair(t, app, "VPD-D")
	comp := makePlayoffCompetition(t, app, []*core.Record{p1, p2})

	m1 := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "pending")
	m1.Set("round_number", 1)
	m1.Set("date", "2026-09-15 00:00:00.000Z")
	require.NoError(t, app.Save(m1))

	m2 := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "pending")
	m2.Set("round_number", 2)
	m2.Set("date", "2026-09-10 00:00:00.000Z")
	require.NoError(t, app.Save(m2))

	err := ValidatePlayoffDates([]*core.Record{m1, m2})
	assert.ErrorContains(t, err, "ronda")
}

func TestValidatePlayoffDates_NoDateIgnored(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "VPD-E")
	p2 := makePair(t, app, "VPD-F")
	comp := makePlayoffCompetition(t, app, []*core.Record{p1, p2})

	m1 := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "pending")
	m1.Set("round_number", 1)
	require.NoError(t, app.Save(m1))

	m2 := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "pending")
	m2.Set("round_number", 2)
	m2.Set("date", "2026-09-10 00:00:00.000Z")
	require.NoError(t, app.Save(m2))

	assert.NoError(t, ValidatePlayoffDates([]*core.Record{m1, m2}))
}
