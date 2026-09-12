package league

import (
	"encoding/json"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeProposal(t *testing.T, app core.App, matchID, authorID, scores string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("match_messages")
	require.NoError(t, err)
	rec := core.NewRecord(col)
	rec.Set("match", matchID)
	rec.Set("author", authorID)
	rec.Set("type", "result_submission")
	rec.Set("proposal_status", "pending")
	pd, _ := json.Marshal(map[string]string{"scores": scores})
	rec.Set("proposal_data", string(pd))
	require.NoError(t, app.Save(rec))
	return rec
}

func TestApplyAcceptedResult_Won(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Pareja A")
	p2 := makePair(t, app, "Pareja B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	match := makeMatch(t, app, comp.Id, p1.Id, p2.Id, StatusScheduled)

	proposer := p1.GetString("player1")
	proposal := makeProposal(t, app, match.Id, proposer, "6-3 6-4")

	// Create a sibling proposal to verify superseding
	sibling := makeProposal(t, app, match.Id, p2.GetString("player1"), "3-6 4-6")

	responder := p2.GetString("player1")
	notifier := &fakeNotifier{}
	svc := New(app, notifier)

	out, err := svc.ApplyAcceptedResult(match, AcceptedResult{Proposal: proposal, ActorID: responder})
	require.NoError(t, err)
	assert.True(t, out.Won)
	assert.Equal(t, 1, out.WinnerSide)

	updated, err := app.FindRecordById("matches", match.Id)
	require.NoError(t, err)
	assert.Equal(t, StatusFinal, updated.GetString("status"))
	assert.Equal(t, p1.Id, updated.GetString("winner"))
	assert.Equal(t, "6-3 6-4", updated.GetString("scores"))

	updatedProposal, err := app.FindRecordById("match_messages", proposal.Id)
	require.NoError(t, err)
	assert.Equal(t, "accepted", updatedProposal.GetString("proposal_status"))

	updatedSibling, err := app.FindRecordById("match_messages", sibling.Id)
	require.NoError(t, err)
	assert.Equal(t, "superseded", updatedSibling.GetString("proposal_status"))

	require.True(t, len(notifier.calls) > 0)

	entries, err := app.FindRecordsByFilter("match_messages",
		"match = {:mid} && type = 'result_response'", "", 0, 0, map[string]any{"mid": match.Id})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, responder, entries[0].GetString("author"))
}

func TestApplyAcceptedResult_WonByRule(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Pareja A")
	p2 := makePair(t, app, "Pareja B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	match := makeMatch(t, app, comp.Id, p1.Id, p2.Id, StatusScheduled)

	proposer := p1.GetString("player1")
	proposal := makeProposal(t, app, match.Id, proposer, "6-3 4-1")
	responder := p2.GetString("player1")
	notifier := &fakeNotifier{}
	svc := New(app, notifier)

	out, err := svc.ApplyAcceptedResult(match, AcceptedResult{Proposal: proposal, ActorID: responder})
	require.NoError(t, err)
	assert.True(t, out.Won)
	assert.Equal(t, 1, out.WinnerSide)

	updated, err := app.FindRecordById("matches", match.Id)
	require.NoError(t, err)
	assert.Equal(t, StatusFinal, updated.GetString("status"))
	assert.Equal(t, p1.Id, updated.GetString("winner"))
}

func TestApplyAcceptedResult_NotWon(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Pareja A")
	p2 := makePair(t, app, "Pareja B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	match := makeMatch(t, app, comp.Id, p1.Id, p2.Id, StatusScheduled)
	match.Set("date", "2026-09-15 00:00:00.000Z")
	match.Set("time", "18:00")
	require.NoError(t, app.Save(match))

	proposer := p1.GetString("player1")
	proposal := makeProposal(t, app, match.Id, proposer, "6-3 2-1")
	responder := p2.GetString("player1")
	notifier := &fakeNotifier{}
	svc := New(app, notifier)

	out, err := svc.ApplyAcceptedResult(match, AcceptedResult{Proposal: proposal, ActorID: responder})
	require.NoError(t, err)
	assert.False(t, out.Won)
	assert.Equal(t, "6-3", out.Carried)

	updated, err := app.FindRecordById("matches", match.Id)
	require.NoError(t, err)
	assert.Equal(t, StatusPending, updated.GetString("status"))
	assert.Equal(t, "6-3", updated.GetString("carried_sets"))
	assert.Equal(t, "", updated.GetString("date"))
	assert.Equal(t, "", updated.GetString("time"))
	assert.Equal(t, 0, int(updated.GetFloat("last_warn_level")))
	assert.Equal(t, "", updated.GetString("submitted_by"))

	// Notification: "Partido por reanudar" to both pairs
	require.Len(t, notifier.calls, 2)
	assert.Equal(t, "scheduling", notifier.calls[0].notifType)
	assert.Equal(t, "Partido por reanudar", notifier.calls[0].title)

	// System result_event entry
	events, err := app.FindRecordsByFilter("match_messages",
		"match = {:mid} && type = 'result_event'", "", 0, 0, map[string]any{"mid": match.Id})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Contains(t, events[0].GetString("content"), "se reanuda desde 6-3")
}

func TestApplyAcceptedResult_QuorumTimeout(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Pareja A")
	p2 := makePair(t, app, "Pareja B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	match := makeMatch(t, app, comp.Id, p1.Id, p2.Id, StatusScheduled)

	proposer := p1.GetString("player1")
	proposal := makeProposal(t, app, match.Id, proposer, "6-3 6-4")
	notifier := &fakeNotifier{}
	svc := New(app, notifier)

	out, err := svc.ApplyAcceptedResult(match, AcceptedResult{Proposal: proposal})
	require.NoError(t, err)
	assert.True(t, out.Won)

	updated, err := app.FindRecordById("matches", match.Id)
	require.NoError(t, err)
	assert.Contains(t, updated.GetString("dispute_notes"), "Auto-confirmado")

	// Both pairs notified
	require.Len(t, notifier.calls, 2)
	assert.Equal(t, "general", notifier.calls[0].notifType)
	assert.Equal(t, "Resultado confirmado automáticamente", notifier.calls[0].title)
}

func TestApplyAcceptedResult_StaleStatus(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Pareja A")
	p2 := makePair(t, app, "Pareja B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	match := makeMatch(t, app, comp.Id, p1.Id, p2.Id, StatusFinal)

	proposer := p1.GetString("player1")
	proposal := makeProposal(t, app, match.Id, proposer, "6-3 6-4")
	notifier := &fakeNotifier{}
	svc := New(app, notifier)

	_, err := svc.ApplyAcceptedResult(match, AcceptedResult{Proposal: proposal, ActorID: proposer})
	require.ErrorIs(t, err, ErrMatchNotPreScore)
}

func TestApplyAcceptedResult_Accumulation(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Pareja A")
	p2 := makePair(t, app, "Pareja B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	match := makeMatch(t, app, comp.Id, p1.Id, p2.Id, StatusScheduled)
	match.Set("carried_sets", "6-3")
	require.NoError(t, app.Save(match))

	proposer := p1.GetString("player1")
	proposal := makeProposal(t, app, match.Id, proposer, "6-3 3-6 2-1")
	responder := p2.GetString("player1")
	notifier := &fakeNotifier{}
	svc := New(app, notifier)

	out, err := svc.ApplyAcceptedResult(match, AcceptedResult{Proposal: proposal, ActorID: responder})
	require.NoError(t, err)
	assert.False(t, out.Won)
	assert.Equal(t, "6-3 3-6", out.Carried)

	updated, err := app.FindRecordById("matches", match.Id)
	require.NoError(t, err)
	assert.Equal(t, "6-3 3-6", updated.GetString("carried_sets"))
	assert.Equal(t, StatusPending, updated.GetString("status"))
}
