package league

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlipScoreSides(t *testing.T) {
	t.Parallel()
	tests := []struct {
		score, want string
	}{
		{"6-3 6-4", "3-6 4-6"},
		{"6-3 4-6 7-5", "3-6 6-4 5-7"},
		{"7-6(5) 6-4", "6-7(5) 4-6"},
		{"WO", "WO"},
		{"wo", "wo"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := flipScoreSides(tt.score); got != tt.want {
			t.Errorf("flipScoreSides(%q) = %q, want %q", tt.score, got, tt.want)
		}
	}
}

func TestPrecedents_NoPriorMeetings(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "PrecA")
	p2 := makePair(t, app, "PrecB")

	_, ok := Precedents(app, PrecedentsQuery{Pair1ID: p1.Id, Pair2ID: p2.Id})
	assert.False(t, ok, "pairs with no shared history must report ok=false")
}

func TestPrecedents_TalliesWinsAndLastScore(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "PrecC")
	p2 := makePair(t, app, "PrecD")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	// Match 1: p1 wins, p1 as pair1.
	m1 := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "final")
	m1.Set("scores", "6-3 6-4")
	m1.Set("winner", p1.Id)
	require.NoError(t, app.Save(m1))

	// Match 2 (most recent, created later): p2 wins, p2 as pair1 this time —
	// the score must be normalized back to p1/p2 order in the summary.
	m2 := makeMatch(t, app, comp.Id, p2.Id, p1.Id, "final")
	m2.Set("scores", "6-2 6-1")
	m2.Set("winner", p2.Id)
	require.NoError(t, app.Save(m2))

	// The current match being viewed — excluded from the tally.
	current := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "pending")

	summary, ok := Precedents(app, PrecedentsQuery{Pair1ID: p1.Id, Pair2ID: p2.Id, CompetitionID: comp.Id, ExcludeMatchID: current.Id})
	require.True(t, ok)
	assert.Equal(t, 1, summary.Pair1Wins)
	assert.Equal(t, 1, summary.Pair2Wins)
	assert.Equal(t, m2.Id, summary.LastMatchID, "the most recent meeting must win, not just insertion order")
	assert.Equal(t, "2-6 1-6", summary.LastScore, "score normalized to pair1/pair2 order even though m2 stored the pairs reversed")
}

func TestPrecedents_SortsByPlayDateNotCreationOrder(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "PrecE")
	p2 := makePair(t, app, "PrecF")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	// Played later (2026-02-01) but its result is entered into the DB
	// FIRST — created before the earlier-played match's record.
	playedLater := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "final")
	playedLater.Set("date", "2026-02-01")
	playedLater.Set("scores", "6-3 6-4")
	playedLater.Set("winner", p1.Id)
	require.NoError(t, app.Save(playedLater))

	// Played earlier (2026-01-01) but its result is entered into the DB
	// SECOND — created after the later-played match's record. Sorting by
	// -created alone would incorrectly pick this as "the most recent
	// meeting" (MinMatchesForLevel-style off-by-intent bug); sorting by
	// -date first must pick playedLater instead.
	playedEarlier := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "final")
	playedEarlier.Set("date", "2026-01-01")
	playedEarlier.Set("scores", "6-1 6-1")
	playedEarlier.Set("winner", p2.Id)
	require.NoError(t, app.Save(playedEarlier))

	current := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "pending")

	summary, ok := Precedents(app, PrecedentsQuery{Pair1ID: p1.Id, Pair2ID: p2.Id, CompetitionID: comp.Id, ExcludeMatchID: current.Id})
	require.True(t, ok)
	assert.Equal(t, playedLater.Id, summary.LastMatchID, "the match played most recently must win, not the one entered into the DB most recently")
	assert.Equal(t, "6-3 6-4", summary.LastScore)
}

func TestSponsorLogoURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		sponsorID, filename, want string
	}{
		{"spon1", "logo.png", "/api/files/sponsors/spon1/logo.png"},
		{"spon1", "", ""},
	}
	for _, tt := range tests {
		got := SponsorLogoURL(tt.sponsorID, tt.filename)
		assert.Equal(t, tt.want, got, "SponsorLogoURL(%q, %q)", tt.sponsorID, tt.filename)
	}
}

func TestEntityURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		kind, id, want string
	}{
		{"player", "abc", "/player/abc"},
		{"competition", "xyz", "/competition/xyz"},
		{"match", "m1", "/match/m1"},
		{"pair", "p1", "/pair/p1"},
		{"unknown", "x", "#"},
		{"player", "", "#"},
		{"", "", "#"},
	}
	for _, tt := range tests {
		if got := EntityURL(tt.kind, tt.id); got != tt.want {
			t.Errorf("EntityURL(%q, %q) = %q, want %q", tt.kind, tt.id, got, tt.want)
		}
	}
}
