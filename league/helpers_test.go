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
