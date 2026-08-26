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
		{"rounds zero", start, end, 0, 1, time.Time{}, false},
		{"rounds negative", start, end, -1, 1, time.Time{}, false},
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

func TestBuildRoundSchedule(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)

	sched := BuildRoundSchedule(start, end, 4)
	require.Len(t, sched, 4)
	assert.Equal(t, time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC), sched[1])
	assert.Equal(t, time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC), sched[2])
	assert.Equal(t, time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC), sched[3])
	assert.Equal(t, end, sched[4])

	assert.Nil(t, BuildRoundSchedule(time.Time{}, end, 4))
	assert.Nil(t, BuildRoundSchedule(start, time.Time{}, 4))
	assert.Nil(t, BuildRoundSchedule(start, end, 0))
	assert.Nil(t, BuildRoundSchedule(start, end, -1))

	// rounds==1 must produce a 1-entry schedule (kills <1 → <=1 mutant).
	one := BuildRoundSchedule(start, end, 1)
	require.Len(t, one, 1)
	assert.Equal(t, end, one[1])
}

func TestStoreRoundSchedule(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)

	s := StoreRoundSchedule(start, end, 4)
	assert.NotEmpty(t, s)
	assert.Contains(t, s, "2026-09-06")

	assert.Empty(t, StoreRoundSchedule(time.Time{}, end, 4))
	assert.Empty(t, StoreRoundSchedule(start, end, 0))
}

func TestRoundArrangeDate_StoredHit(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	comp := makeCompetition(t, app, nil)

	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	comp.Set("start_date", start.Format(time.RFC3339))
	comp.Set("end_date", end.Format(time.RFC3339))
	comp.Set("rounds", 4)
	comp.Set("round_arrange_dates", StoreRoundSchedule(start, end, 4))
	require.NoError(t, app.Save(comp))

	got, ok := RoundArrangeDate(comp, 2)
	require.True(t, ok)
	assert.Equal(t, time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC), got)
}

func TestRoundArrangeDate_EmptyFallback(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	comp := makeCompetition(t, app, nil)

	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	comp.Set("start_date", start.Format(time.RFC3339))
	comp.Set("end_date", end.Format(time.RFC3339))
	comp.Set("rounds", 4)
	comp.Set("round_arrange_dates", "")
	require.NoError(t, app.Save(comp))

	got, ok := RoundArrangeDate(comp, 2)
	require.True(t, ok)
	assert.Equal(t, time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC), got)
}

func TestRoundArrangeDate_BadJSON(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	comp := makeCompetition(t, app, nil)

	comp.Set("start_date", "2026-09-01")
	comp.Set("end_date", "2026-09-21")
	comp.Set("rounds", 4)
	comp.Set("round_arrange_dates", "{invalid json")
	require.NoError(t, app.Save(comp))

	got, ok := RoundArrangeDate(comp, 1)
	require.True(t, ok)
	assert.Equal(t, time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC), got)
}

func TestRoundArrangeDate_ZeroRounds(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	comp := makeCompetition(t, app, nil)

	comp.Set("start_date", "2026-09-01")
	comp.Set("end_date", "2026-09-21")
	comp.Set("rounds", 0)
	comp.Set("round_arrange_dates", "")
	require.NoError(t, app.Save(comp))

	_, ok := RoundArrangeDate(comp, 1)
	assert.False(t, ok)
}

func TestRecoveryDays(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	unset := makeCompetition(t, app, nil)
	assert.Equal(t, 14, RecoveryDays(unset))

	set := makeCompetition(t, app, nil)
	set.Set("recovery_days", 21)
	require.NoError(t, app.Save(set))
	assert.Equal(t, 21, RecoveryDays(set))
}

func TestCompetitionPhase(t *testing.T) {
	t.Parallel()
	end := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		end       time.Time
		recovery  int
		finalized bool
		now       time.Time
		want      Phase
	}{
		{"finalized wins even with no end date", time.Time{}, 0, true, end, PhaseFinished},
		{"zero end date never auto-finishes", time.Time{}, 0, false, end.AddDate(1, 0, 0), PhasePlaying},
		{"before end date", end, 14, false, end.AddDate(0, 0, -1), PhasePlaying},
		{"exactly at end date", end, 14, false, end, PhasePlaying},
		{"just after end date enters recovery", end, 14, false, end.Add(time.Second), PhaseRecovery},
		{"exactly at end+recovery is still recovery", end, 14, false, end.AddDate(0, 0, 14), PhaseRecovery},
		{"one second past end+recovery finishes", end, 14, false, end.AddDate(0, 0, 14).Add(time.Second), PhaseFinished},
		{"unset recovery_days uses default 14", end, 0, false, end.AddDate(0, 0, 14), PhaseRecovery},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app := newTestApp(t)
			comp := makeCompetition(t, app, nil)
			if !tt.end.IsZero() {
				comp.Set("end_date", tt.end.Format(time.RFC3339))
			}
			comp.Set("recovery_days", tt.recovery)
			comp.Set("finalized", tt.finalized)
			require.NoError(t, app.Save(comp))

			assert.Equal(t, tt.want, CompetitionPhase(comp, tt.now))
		})
	}
}

func TestPhaseLabels(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", PhaseUnknown.String())
	assert.Equal(t, "", PhaseUnknown.Label())
	assert.Equal(t, "playing", PhasePlaying.String())
	assert.Equal(t, "En juego", PhasePlaying.Label())
	assert.Equal(t, "recovery", PhaseRecovery.String())
	assert.Equal(t, "En recuperación", PhaseRecovery.Label())
	assert.Equal(t, "finished", PhaseFinished.String())
	assert.Equal(t, "Finalizada", PhaseFinished.Label())
}
