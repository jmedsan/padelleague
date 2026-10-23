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
		if got := FlipScoreSides(tt.score); got != tt.want {
			t.Errorf("FlipScoreSides(%q) = %q, want %q", tt.score, got, tt.want)
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

	// Match 1 (earlier date): p1 wins, p1 as pair1.
	m1 := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "final")
	m1.Set("scores", "6-3 6-4")
	m1.Set("winner", p1.Id)
	m1.Set("date", "2026-01-01 10:00:00.000Z")
	require.NoError(t, app.Save(m1))

	// Match 2 (later date): p2 wins, p2 as pair1 this time —
	// the score must be normalized back to p1/p2 order in the summary.
	m2 := makeMatch(t, app, comp.Id, p2.Id, p1.Id, "final")
	m2.Set("scores", "6-2 6-1")
	m2.Set("winner", p2.Id)
	m2.Set("date", "2026-02-01 10:00:00.000Z")
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
		{"spon1", "logo.png", "/logo/sponsor/spon1"},
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

func TestIsWithdrawn_NotWithdrawn(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "WD NotA")
	p2 := makePair(t, app, "WD NotB")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	w1, err := IsWithdrawn(app, p1.Id, comp.Id)
	require.NoError(t, err)
	assert.False(t, w1)
	w2, err := IsWithdrawn(app, p2.Id, comp.Id)
	require.NoError(t, err)
	assert.False(t, w2)
}

func TestIsWithdrawn_Withdrawn(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "WD YesA")
	p2 := makePair(t, app, "WD YesB")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	comp.Set("withdrawn_pairs", []string{p1.Id})
	require.NoError(t, app.Save(comp))

	w1, err := IsWithdrawn(app, p1.Id, comp.Id)
	require.NoError(t, err)
	assert.True(t, w1)
	w2, err := IsWithdrawn(app, p2.Id, comp.Id)
	require.NoError(t, err)
	assert.False(t, w2)
}

func TestIsWithdrawn_LookupFailureFailsClosed(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "WD Missing")

	withdrawn, err := IsWithdrawn(app, p1.Id, "missing-competition")
	require.Error(t, err)
	assert.False(t, withdrawn)
}

func TestPrecedents_CountsUnconfirmedResultAndFlagsIt(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "PrecG")
	p2 := makePair(t, app, "PrecH")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	confirmed := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "final")
	confirmed.Set("date", "2026-01-01")
	confirmed.Set("scores", "6-3 6-4")
	confirmed.Set("winner", p1.Id)
	require.NoError(t, app.Save(confirmed))

	// Later meeting, proposed with p2 on the pair1 side and not yet accepted.
	proposed := makeMatch(t, app, comp.Id, p2.Id, p1.Id, StatusScheduled)
	proposed.Set("date", "2026-02-01")
	require.NoError(t, app.Save(proposed))
	makeProposal(t, app, proposed.Id, p2.GetString("player1"), "6-2 6-1")

	current := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "pending")

	summary, ok := Precedents(app, PrecedentsQuery{Pair1ID: p1.Id, Pair2ID: p2.Id, CompetitionID: comp.Id, ExcludeMatchID: current.Id})
	require.True(t, ok)
	assert.True(t, summary.HasProvisional)
	assert.Equal(t, 1, summary.Pair1Wins)
	assert.Equal(t, 1, summary.Pair2Wins)
	assert.Equal(t, proposed.Id, summary.LastMatchID, "the unconfirmed meeting is the most recent")
	assert.Equal(t, "2-6 1-6", summary.LastScore)
}

func TestPrecedents_OnlyUnconfirmedResultIsEnough(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "PrecI")
	p2 := makePair(t, app, "PrecJ")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	proposed := makeMatch(t, app, comp.Id, p1.Id, p2.Id, StatusScheduled)
	makeProposal(t, app, proposed.Id, p1.GetString("player1"), "6-3 6-4")

	summary, ok := Precedents(app, PrecedentsQuery{Pair1ID: p1.Id, Pair2ID: p2.Id, CompetitionID: comp.Id})
	require.True(t, ok)
	assert.True(t, summary.HasProvisional)
	assert.Equal(t, 1, summary.Pair1Wins)
}

func TestPrecedents_ConfirmedOnlyIsNotFlagged(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "PrecK")
	p2 := makePair(t, app, "PrecL")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "final")
	m.Set("scores", "6-3 6-4")
	m.Set("winner", p1.Id)
	require.NoError(t, app.Save(m))

	summary, ok := Precedents(app, PrecedentsQuery{Pair1ID: p1.Id, Pair2ID: p2.Id, CompetitionID: comp.Id})
	require.True(t, ok)
	assert.False(t, summary.HasProvisional)
}

func TestMessageAuthorLabel(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Label A")
	p2 := makePair(t, app, "Label B")
	author := p2.GetString("player1")
	authorName := PlayerName(app, author)

	msg := core.NewRecord(matchMessagesCollection(t, app))
	msg.Set("author", author)

	t.Run("no stored pair is the bare name, whatever the current team", func(t *testing.T) {
		assert.Equal(t, authorName, MessageAuthorLabel(app, msg))
	})
	t.Run("stored pair names the pair, not the current team", func(t *testing.T) {
		msg.Set("author_pair", p1.Id)
		assert.Equal(t, authorName+" (Label A)", MessageAuthorLabel(app, msg))
	})
	t.Run("no author has no label", func(t *testing.T) {
		assert.Empty(t, MessageAuthorLabel(app, core.NewRecord(matchMessagesCollection(t, app))))
	})
}

func TestPairTeamAndStoredTeams(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Team A")
	p2 := makePair(t, app, "Team B")
	other := makePair(t, app, "Team C")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	match := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "pending")

	assert.Equal(t, 1, PairTeam(match, p1.Id))
	assert.Equal(t, 2, PairTeam(match, p2.Id))
	assert.Equal(t, 0, PairTeam(match, other.Id), "a pair outside the match")
	assert.Equal(t, 0, PairTeam(match, ""), "no pair")

	msg := core.NewRecord(matchMessagesCollection(t, app))
	assert.Equal(t, 0, MessageAuthorTeam(msg, match))
	msg.Set("author_pair", p2.Id)
	assert.Equal(t, 2, MessageAuthorTeam(msg, match))

	assert.Equal(t, 0, SubmitterTeam(match))
	assert.Equal(t, 0, DisputerTeam(match))
	match.Set("submitted_by_pair", p1.Id)
	match.Set("disputed_by_pair", p2.Id)
	assert.Equal(t, 1, SubmitterTeam(match))
	assert.Equal(t, 2, DisputerTeam(match))
}

func TestPairTeam_EmptyPairOnMatchIsNoTeam(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Unseeded A")
	p2 := makePair(t, app, "Unseeded B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	match := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "pending")
	match.Set("pair2", "")
	require.NoError(t, app.SaveNoValidate(match))

	assert.Equal(t, 0, PairTeam(match, ""), "an empty pair id is no team, even beside an empty seed")
	assert.Equal(t, 1, PairTeam(match, p1.Id))
}

func TestCurrentPairID(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Current A")
	p2 := makePair(t, app, "Current B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	match := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "pending")
	outsider := makeUser(t, app, "Fuera", "fuera@test.local")

	tests := []struct {
		name   string
		userID string
		want   string
	}{
		{name: "player of pair1", userID: p1.GetString("player1"), want: p1.Id},
		{name: "player of pair2", userID: p2.GetString("player1"), want: p2.Id},
		{name: "user outside the match", userID: outsider.Id, want: ""},
		{name: "no user", userID: "", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, CurrentPairID(app, tc.userID, match))
		})
	}
}

func TestAvatarURL(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", AvatarURL("u1", ""), "no file, no URL")
	assert.Equal(t, "/api/files/users/u1/me.png", AvatarURL("u1", "me.png"))
}

func TestPlayerAvatarURL(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	withAvatar := makeUser(t, app, "Con foto", "foto@test.local")
	withAvatar.Set("avatar", "me.png")
	require.NoError(t, app.SaveNoValidate(withAvatar)) // the stored filename, not a new upload
	withoutAvatar := makeUser(t, app, "Sin foto", "sinfoto@test.local")

	assert.Equal(t, "/api/files/users/"+withAvatar.Id+"/me.png", PlayerAvatarURL(app, withAvatar.Id))
	assert.Equal(t, "", PlayerAvatarURL(app, withoutAvatar.Id), "a user with no avatar has no URL")
	assert.Equal(t, "", PlayerAvatarURL(app, "missing-user-id"), "an unknown user has no URL")
	assert.Equal(t, "", PlayerAvatarURL(app, ""), "no user has no URL")
}

func TestStampActorPairs(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Stamp A")
	p2 := makePair(t, app, "Stamp B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	match := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "pending")

	match.Set("submitted_by", p1.GetString("player1"))
	match.Set("disputed_by", p2.GetString("player2"))
	StampActorPairs(app, match)
	assert.Equal(t, p1.Id, match.GetString("submitted_by_pair"))
	assert.Equal(t, p2.Id, match.GetString("disputed_by_pair"))
	require.NoError(t, app.Save(match))

	t.Run("an unchanged actor keeps the stored pair after a swap", func(t *testing.T) {
		p1.Set("player1", makePair(t, app, "Stamp Spare").GetString("player1"))
		require.NoError(t, app.Save(p1))
		saved, err := app.FindRecordById("matches", match.Id)
		require.NoError(t, err)
		saved.Set("status", "pending")
		StampActorPairs(app, saved)
		assert.Equal(t, p1.Id, saved.GetString("submitted_by_pair"))
	})
	t.Run("a new actor is stamped with their current pair", func(t *testing.T) {
		saved, err := app.FindRecordById("matches", match.Id)
		require.NoError(t, err)
		saved.Set("submitted_by", p2.GetString("player1"))
		StampActorPairs(app, saved)
		assert.Equal(t, p2.Id, saved.GetString("submitted_by_pair"))
	})
	t.Run("clearing the actor clears the pair", func(t *testing.T) {
		saved, err := app.FindRecordById("matches", match.Id)
		require.NoError(t, err)
		saved.Set("disputed_by", "")
		StampActorPairs(app, saved)
		assert.Empty(t, saved.GetString("disputed_by_pair"))
	})
}

func matchMessagesCollection(t *testing.T, app core.App) *core.Collection {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("match_messages")
	require.NoError(t, err)
	return col
}
