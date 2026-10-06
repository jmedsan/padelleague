package handlers

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"

	"padelleague/league"
)

// provisional_internal_test.go covers the unexported helpers that carry the
// "unconfirmed result" flag from league.ProvisionalResults to the templates:
// markProvisional (match cards) and roundGroup.countPlayed (round badges).

func TestMarkProvisional(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "MP A")
	p2 := makePair(t, app, "MP B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	author := p1.GetString("player1")

	proposed := makeMatch(t, app, comp.Id, p1.Id, p2.Id, league.StatusScheduled)
	makeResultProposal(t, app, proposed.Id, author, "6-3 6-4")
	undecided := makeMatch(t, app, comp.Id, p1.Id, p2.Id, league.StatusScheduled)
	makeResultProposal(t, app, undecided.Id, author, "6-3 2-1")
	plain := makeMatch(t, app, comp.Id, p1.Id, p2.Id, league.StatusScheduled)
	final := makeMatch(t, app, comp.Id, p1.Id, p2.Id, league.StatusFinal)
	makeResultProposal(t, app, final.Id, author, "6-3 6-4")

	cards := []MatchCard{{Match: proposed}, {Match: undecided}, {Match: plain}, {Match: final}}
	markProvisional(app, cards)

	assert.True(t, cards[0].Provisional, "decided pending proposal is provisional")
	assert.False(t, cards[1].Provisional, "an undecided open set is not a result yet")
	assert.False(t, cards[2].Provisional, "no proposal, no result")
	assert.False(t, cards[3].Provisional, "a final match is confirmed, never provisional")
}

func TestMarkProvisional_ClearsStaleFlag(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "MC A")
	p2 := makePair(t, app, "MC B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, league.StatusScheduled)

	cards := []MatchCard{{Match: m, Provisional: true}}
	markProvisional(app, cards)
	assert.False(t, cards[0].Provisional)
}

func TestRoundGroupCountPlayed(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "CP A")
	p2 := makePair(t, app, "CP B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	final := makeMatch(t, app, comp.Id, p1.Id, p2.Id, league.StatusFinal)
	open := makeMatch(t, app, comp.Id, p1.Id, p2.Id, league.StatusScheduled)
	proposed := makeMatch(t, app, comp.Id, p1.Id, p2.Id, league.StatusScheduled)

	t.Run("final and unconfirmed count as played, only the latter flags", func(t *testing.T) {
		g := roundGroup{Matches: []MatchCard{{Match: final}, {Match: open}, {Match: proposed, Provisional: true}}}
		g.countPlayed()
		assert.Equal(t, 2, g.Played)
		assert.True(t, g.HasProvisional)
	})

	t.Run("confirmed results alone do not flag", func(t *testing.T) {
		g := roundGroup{Matches: []MatchCard{{Match: final}, {Match: open}}}
		g.countPlayed()
		assert.Equal(t, 1, g.Played)
		assert.False(t, g.HasProvisional)
	})
}
