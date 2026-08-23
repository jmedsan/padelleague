package league

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAwards_Basic(t *testing.T) {
	app := newTestApp(t)
	svc := New(app, nil)

	p1 := makePair(t, app, "Aw A")
	p2 := makePair(t, app, "Aw B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	for i := 0; i < 3; i++ {
		m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "final")
		m.Set("scores", "6-3 6-4")
		m.Set("winner", p1.Id)
		m.Set("round_number", i+1)
		require.NoError(t, app.Save(m))
	}

	awards := svc.Awards(comp.Id)
	require.NotEmpty(t, awards)

	var titles []string
	for _, a := range awards {
		titles = append(titles, a.Title)
	}
	assert.Contains(t, titles, "Mejor pareja")
	assert.Contains(t, titles, "Más partidos")
	assert.Contains(t, titles, "Mayor racha")
}

func TestAwards_NoMatches(t *testing.T) {
	app := newTestApp(t)
	svc := New(app, nil)

	p1 := makePair(t, app, "AwNone A")
	p2 := makePair(t, app, "AwNone B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	_ = comp

	awards := svc.Awards(comp.Id)
	assert.Empty(t, awards)
}

func TestAwards_InvalidCompetition(t *testing.T) {
	app := newTestApp(t)
	svc := New(app, nil)

	awards := svc.Awards("nonexistent")
	assert.Nil(t, awards)
}

func TestAwards_NoStreak(t *testing.T) {
	app := newTestApp(t)
	svc := New(app, nil)

	p1 := makePair(t, app, "AwNoStr A")
	p2 := makePair(t, app, "AwNoStr B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	// One win each — no streak > 1
	m1 := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "final")
	m1.Set("scores", "6-3 6-4")
	m1.Set("winner", p1.Id)
	require.NoError(t, app.Save(m1))

	m2 := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "final")
	m2.Set("scores", "3-6 4-6")
	m2.Set("winner", p2.Id)
	m2.Set("round_number", 2)
	require.NoError(t, app.Save(m2))

	awards := svc.Awards(comp.Id)
	for _, a := range awards {
		assert.NotEqual(t, "Mayor racha", a.Title)
	}
}
