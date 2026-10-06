package league

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvisionalResults(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "ProvRA")
	p2 := makePair(t, app, "ProvRB")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	author := p1.GetString("player1")

	t.Run("decided pending proposal becomes a synthetic result keeping the match id", func(t *testing.T) {
		m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, StatusScheduled)
		m.Set("date", "2026-03-01 10:00:00.000Z")
		require.NoError(t, app.Save(m))
		makeProposal(t, app, m.Id, author, "6-3 6-4")

		got, err := ProvisionalResults(app, []*core.Record{m})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, m.Id, got[0].Id)
		assert.Equal(t, p1.Id, got[0].GetString("winner"))
		assert.Equal(t, "6-3 6-4", got[0].GetString("scores"))
		assert.Equal(t, comp.Id, got[0].GetString("competition"))
		assert.Equal(t, p1.Id, got[0].GetString("pair1"))
		assert.Equal(t, p2.Id, got[0].GetString("pair2"))
		assert.Equal(t, 1.0, got[0].GetFloat("round_number"))
		assert.Contains(t, got[0].GetString("date"), "2026-03-01")
	})

	t.Run("final and disputed candidates are excluded", func(t *testing.T) {
		for _, status := range []string{StatusFinal, StatusDisputed} {
			m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, status)
			makeProposal(t, app, m.Id, author, "6-3 6-4")
			got, err := ProvisionalResults(app, []*core.Record{m})
			require.NoError(t, err)
			assert.Empty(t, got, status)
		}
	})

	t.Run("undecided open set is excluded", func(t *testing.T) {
		m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, StatusScheduled)
		makeProposal(t, app, m.Id, author, "6-3 2-1")
		got, err := ProvisionalResults(app, []*core.Record{m})
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("match without a pending proposal is excluded", func(t *testing.T) {
		m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, StatusScheduled)
		got, err := ProvisionalResults(app, []*core.Record{m})
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("newest pending proposal wins", func(t *testing.T) {
		m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, StatusScheduled)
		makeProposal(t, app, m.Id, author, "6-3 6-4")
		newer := makeProposal(t, app, m.Id, p2.GetString("player1"), "3-6 4-6")
		dt, err := types.ParseDateTime("2099-01-01 00:00:00.000Z")
		require.NoError(t, err)
		newer.SetRaw("created", dt)
		require.NoError(t, app.Save(newer))

		got, err := ProvisionalResults(app, []*core.Record{m})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, p2.Id, got[0].GetString("winner"))
		assert.Equal(t, "3-6 4-6", got[0].GetString("scores"))
	})

	t.Run("no candidates", func(t *testing.T) {
		got, err := ProvisionalResults(app, nil)
		require.NoError(t, err)
		assert.Empty(t, got)
	})
}

func TestNewestPendingProposals_ScopedToGivenMatches(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "ScopeA")
	p2 := makePair(t, app, "ScopeB")
	compA := makeCompetition(t, app, []*core.Record{p1, p2})
	compB := makeCompetition(t, app, []*core.Record{p1, p2})
	mine := makeMatch(t, app, compA.Id, p1.Id, p2.Id, StatusScheduled)
	other := makeMatch(t, app, compB.Id, p1.Id, p2.Id, StatusScheduled)
	author := p1.GetString("player1")
	wantProposal := makeProposal(t, app, mine.Id, author, "6-3 6-4")
	makeProposal(t, app, other.Id, author, "6-3 6-4")

	got, err := newestPendingProposals(app, []*core.Record{mine})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, wantProposal.Id, got[mine.Id].Id)
	assert.NotContains(t, got, other.Id, "a pending proposal in another competition must not be loaded")
}

func TestNewestPendingProposals_AcrossChunks(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "ChunkA")
	p2 := makePair(t, app, "ChunkB")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	author := p1.GetString("player1")

	matches := make([]*core.Record, proposalChunkSize+5)
	for i := range matches {
		matches[i] = makeMatch(t, app, comp.Id, p1.Id, p2.Id, StatusScheduled)
	}
	first := makeProposal(t, app, matches[0].Id, author, "6-3 6-4")
	last := makeProposal(t, app, matches[len(matches)-1].Id, author, "6-3 6-4")

	got, err := newestPendingProposals(app, matches)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, first.Id, got[matches[0].Id].Id)
	assert.Equal(t, last.Id, got[matches[len(matches)-1].Id].Id, "a match in the second chunk must be found")
}
