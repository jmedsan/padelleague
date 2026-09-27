package league

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTruncate_WithinLimit(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "hello", Truncate("hello", 10))
}

func TestTruncate_ExactLimit(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "hello", Truncate("hello", 5))
}

func TestTruncate_OverLimit(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "hel...", Truncate("hello world", 3))
}

func TestTruncate_MultiByte(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "señ...", Truncate("señora", 3))
}

func TestTruncate_Empty(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", Truncate("", 5))
}

func TestPairNames(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Los Lobos")
	p2 := makePair(t, app, "Las Águilas")

	names := PairNames(app, []string{p1.Id, p2.Id})
	assert.Equal(t, "Los Lobos", names[p1.Id])
	assert.Equal(t, "Las Águilas", names[p2.Id])
}

func TestPairNames_UnknownID(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	names := PairNames(app, []string{"nonexistent"})
	assert.Equal(t, "Pareja desconocida", names["nonexistent"])
}

func TestPairNames_EmptyID(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	names := PairNames(app, []string{""})
	assert.Empty(t, names[""])
}

func TestPlayerTeam(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Team A")
	p2 := makePair(t, app, "Team B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	match := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "pending")

	player1 := p1.GetString("player1")
	team, err := PlayerTeam(app, player1, match)
	require.NoError(t, err)
	assert.Equal(t, 1, team)

	player2 := p2.GetString("player1")
	team, err = PlayerTeam(app, player2, match)
	require.NoError(t, err)
	assert.Equal(t, 2, team)
}

func TestPlayerTeam_NotParticipant(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Team A")
	p2 := makePair(t, app, "Team B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	match := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "pending")

	outsider := makeUser(t, app, "Outsider", "")
	_, err := PlayerTeam(app, outsider.Id, match)
	assert.ErrorContains(t, err, "not a participant")
}

func TestPlayerName(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	u := makeUser(t, app, "Carlos García", "")
	assert.Equal(t, "Carlos García", PlayerName(app, u.Id))
}

func TestPlayerName_Empty(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	assert.Equal(t, "?", PlayerName(app, ""))
}

func TestPlayerName_NotFound(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	assert.Equal(t, "?", PlayerName(app, "nonexistent"))
}

func TestPlayersForPair(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p := makePair(t, app, "Test Pair")
	players := PlayersForPair(app, p.Id)
	assert.Len(t, players, 2)
	assert.Equal(t, p.GetString("player1"), players[0])
	assert.Equal(t, p.GetString("player2"), players[1])
}

func TestPlayersForPair_NotFound(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	players := PlayersForPair(app, "nonexistent")
	assert.Nil(t, players)
}

func TestPairsForPlayer(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p := makePair(t, app, "My Pair")
	playerID := p.GetString("player1")

	pairs, err := PairsForPlayer(app, playerID)
	require.NoError(t, err)
	require.Len(t, pairs, 1)
	assert.Equal(t, p.Id, pairs[0].Id)
}

func TestRivalContacts_ViewerInPair1(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	pair1 := makePair(t, app, "Alpha")
	pair2 := makePair(t, app, "Beta")
	comp := makeCompetition(t, app, []*core.Record{pair1, pair2})
	match := makeMatch(t, app, comp.Id, pair1.Id, pair2.Id, "final")
	setPhone(t, app, pair2.GetString("player1"), "+34612345671")
	setPhone(t, app, pair2.GetString("player2"), "+34612345672")

	rivals, err := RivalContacts(app, match, pair1.GetString("player1"))
	require.NoError(t, err)
	require.Len(t, rivals, 2)
	assert.Equal(t, pair2.GetString("player1"), rivals[0].ID)
	assert.Equal(t, pair2.GetString("player2"), rivals[1].ID)
}

func TestRivalContacts_ViewerInPair2(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	pair1 := makePair(t, app, "Gamma")
	pair2 := makePair(t, app, "Delta")
	comp := makeCompetition(t, app, []*core.Record{pair1, pair2})
	match := makeMatch(t, app, comp.Id, pair1.Id, pair2.Id, "final")

	rivals, err := RivalContacts(app, match, pair2.GetString("player2"))
	require.NoError(t, err)
	require.Len(t, rivals, 2)
	assert.Equal(t, pair1.GetString("player1"), rivals[0].ID)
	assert.Equal(t, pair1.GetString("player2"), rivals[1].ID)
}

func TestRivalContacts_NonParticipant(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	pair1 := makePair(t, app, "Epsilon")
	pair2 := makePair(t, app, "Zeta")
	comp := makeCompetition(t, app, []*core.Record{pair1, pair2})
	match := makeMatch(t, app, comp.Id, pair1.Id, pair2.Id, "final")
	outsider := makeUser(t, app, "Outsider", "")

	rivals, err := RivalContacts(app, match, outsider.Id)
	require.NoError(t, err)
	assert.Nil(t, rivals)
}

func TestRivalContacts_ViewerInBothPairs(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	pair1 := makePair(t, app, "Eta")
	pair2 := makePair(t, app, "Theta")
	shared := pair1.GetString("player1")
	pair2.Set("player1", shared)
	require.NoError(t, app.Save(pair2))
	comp := makeCompetition(t, app, []*core.Record{pair1, pair2})
	match := makeMatch(t, app, comp.Id, pair1.Id, pair2.Id, "final")

	rivals, err := RivalContacts(app, match, shared)
	require.NoError(t, err)
	require.Len(t, rivals, 1)
	assert.Equal(t, pair2.GetString("player2"), rivals[0].ID)
}

func TestUserContactInfo_WithPhoneAndEmail(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	user := makeUser(t, app, "Contactable", "contactable@test.local")
	setPhone(t, app, user.Id, "+34612345678")

	info := UserContactInfo(user)
	assert.Equal(t, "https://wa.me/34612345678", info.WhatsAppURL)
	assert.NotEmpty(t, info.WhatsAppURLWithText("x"))
	assert.Equal(t, "mailto:contactable@test.local", info.EmailURL)
}

func TestUserContactInfo_NoPhone(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	user := makeUser(t, app, "NoPhone", "")

	info := UserContactInfo(user)
	assert.Empty(t, info.WhatsAppURL)
	assert.Empty(t, info.WhatsAppURLWithText("x"))
}

// setPhone sets a user's E.164 phone directly, bypassing NormalizePhone
// since these tests already supply normalized values.
func setPhone(t *testing.T, app core.App, userID, e164 string) {
	t.Helper()
	user, err := app.FindRecordById("users", userID)
	require.NoError(t, err)
	user.Set("phone", e164)
	require.NoError(t, app.Save(user))
}
