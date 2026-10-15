package league

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func finalize(t *testing.T, app core.App, m *core.Record, winner string) {
	t.Helper()
	m.Set("status", "final")
	m.Set("winner", winner)
	m.Set("scores", "6-0 6-0")
	require.NoError(t, app.Save(m))
}

func TestLineup_FrozenOnceAtResult(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Lin A")
	p2 := makePair(t, app, "Lin B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	match := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "pending")

	got, err := app.FindRecordById("matches", match.Id)
	require.NoError(t, err)
	assert.Empty(t, MatchLineup(got, p1.Id), "no result yet, nothing frozen")

	orig := p1.GetString("player1")
	finalize(t, app, match, p1.Id)
	got, _ = app.FindRecordById("matches", match.Id)
	assert.ElementsMatch(t, PlayersForPair(app, p1.Id), MatchLineup(got, p1.Id))
	assert.ElementsMatch(t, PlayersForPair(app, p2.Id), MatchLineup(got, p2.Id))

	p1.Set("player1", makeUser(t, app, "Lin Repl", "lin-repl@test.local").Id)
	require.NoError(t, app.Save(p1))
	got.Set("scores", "6-1 6-0")
	require.NoError(t, app.Save(got))
	got, _ = app.FindRecordById("matches", match.Id)
	assert.Contains(t, MatchLineup(got, p1.Id), orig, "a later save never overwrites the lineup")
	assert.False(t, PlayedMatch(got, p1.Id, p1.GetString("player1")))
	assert.True(t, PlayedMatch(got, p1.Id, orig))
}

func TestLineup_FrozenWhenResultProposedInThread(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Prop A")
	p2 := makePair(t, app, "Prop B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	match := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "pending")

	msg := core.NewRecord(matchMessagesCollection(t, app))
	msg.Set("match", match.Id)
	msg.Set("author", p1.GetString("player1"))
	msg.Set("type", "result_submission")
	msg.Set("proposal_data", `{"scores":"6-3 6-4"}`)
	msg.Set("proposal_status", "pending")
	require.NoError(t, app.Save(msg))

	got, err := app.FindRecordById("matches", match.Id)
	require.NoError(t, err)
	assert.ElementsMatch(t, PlayersForPair(app, p1.Id), MatchLineup(got, p1.Id))
}

func TestSummarizePlayer_CountsOnlyMatchesTheyPlayed(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Sum A")
	p2 := makePair(t, app, "Sum B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	orig := p1.GetString("player1")

	before := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "pending")
	finalize(t, app, before, p1.Id)

	repl := makeUser(t, app, "Sum Repl", "sum-repl@test.local")
	p1.Set("player1", repl.Id)
	require.NoError(t, app.Save(p1))

	after := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "pending")
	finalize(t, app, after, p1.Id)

	svc := New(app, &fakeNotifier{})
	assert.Equal(t, 1, svc.SummarizePlayer(orig).TotalPlayed, "the ex-player keeps the match they played")
	assert.Equal(t, 1, svc.SummarizePlayer(repl.Id).TotalPlayed, "the replacement counts only what came after")
	assert.Equal(t, 2, svc.SummarizePlayer(p1.GetString("player2")).TotalPlayed, "the partner played both")
	assert.Equal(t, 2, svc.Summarize([]string{p1.Id}).TotalPlayed, "the pair keeps every result")
}
