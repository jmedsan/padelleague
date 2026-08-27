package league

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsParticipant(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Part A")
	p2 := makePair(t, app, "Part B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	assert.True(t, IsParticipant(comp, map[string]struct{}{p1.Id: {}}))
	assert.False(t, IsParticipant(comp, map[string]struct{}{"other": {}}))
}

func TestAppendUniqueRemoveString(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"a", "b"}, AppendUnique([]string{"a"}, "b"))
	assert.Equal(t, []string{"a"}, AppendUnique([]string{"a"}, "a"))
	assert.Equal(t, []string{"a", "c"}, RemoveString([]string{"a", "b", "c"}, "b"))
	assert.Equal(t, []string{}, RemoveString([]string{"b", "b"}, "b"))
}

func TestMandatoryGateLifecycle(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Gate A")
	p2 := makePair(t, app, "Gate B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	userID := p1.GetString("player1")

	reglamento := makeDoc(t, app, "Reglamento", true)
	tarifas := makeDoc(t, app, "Tarifas", false)
	comp.Set("documents", []string{reglamento.Id, tarifas.Id})
	require.NoError(t, app.Save(comp))

	assert.Equal(t, []string{reglamento.Id}, MandatoryDocIDs(app, comp))
	assert.Len(t, UnacknowledgedMandatory(app, comp, userID), 1, "unread mandatory pending before ack")

	ack, err := FindOrNewAck(app, comp.Id, userID)
	require.NoError(t, err)
	ack.Set("documents", MandatoryDocIDs(app, comp))
	require.NoError(t, app.Save(ack))
	assert.Empty(t, UnacknowledgedMandatory(app, comp, userID), "no pending after ack")

	video := makeDoc(t, app, "Video normas", true)
	comp.Set("documents", []string{reglamento.Id, tarifas.Id, video.Id})
	require.NoError(t, app.Save(comp))
	assert.Len(t, UnacknowledgedMandatory(app, comp, userID), 1, "new mandatory re-gates after prior ack")
}

func TestAttachedDocumentsSortedByTitle(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "Sort A")
	p2 := makePair(t, app, "Sort B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	zeta := makeDoc(t, app, "Zeta", false)
	alfa := makeDoc(t, app, "Alfa", false)
	mike := makeDoc(t, app, "Mike", false)
	comp.Set("documents", []string{zeta.Id, mike.Id, alfa.Id})
	require.NoError(t, app.Save(comp))

	got := AttachedDocuments(app, comp)
	titles := make([]string, len(got))
	for i, d := range got {
		titles[i] = d.GetString("title")
	}
	assert.Equal(t, []string{"Alfa", "Mike", "Zeta"}, titles, "attached documents render in title order regardless of attach order")
}

func makeDoc(t *testing.T, app core.App, title string, mandatory bool) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("documents")
	require.NoError(t, err)
	r := core.NewRecord(col)
	r.Set("title", title)
	r.Set("is_mandatory", mandatory)
	r.Set("url", "https://example.com/doc")
	require.NoError(t, app.Save(r))
	return r
}
