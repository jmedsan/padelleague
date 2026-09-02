package handlers

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDocumentView_LinkDoc(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	doc := makeDocumentTB(t, app, "Reglas", true, "https://example.com/reglas")
	dv := NewDocumentView(doc, PlayerRow)

	assert.Equal(t, "Reglas", dv.Title)
	assert.False(t, dv.IsFile)
	assert.Equal(t, "https://example.com/reglas", dv.OpenURL)
	assert.True(t, dv.IsMandatory)
	assert.False(t, dv.Mode.Admin)
}

func TestNewDocumentView_DefaultFlags(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	col, _ := app.FindCollectionByNameOrId("documents")
	doc := core.NewRecord(col)
	doc.Set("title", "Tarifas")
	doc.Set("url", "https://example.com/tarifas")
	doc.Set("is_default", true)
	require.NoError(t, app.Save(doc))

	dv := NewDocumentView(doc, AdminFull)

	assert.Equal(t, "Tarifas", dv.Title)
	assert.False(t, dv.IsFile)
	assert.Equal(t, "https://example.com/tarifas", dv.OpenURL)
	assert.True(t, dv.IsDefault)
	assert.False(t, dv.IsMandatory)
	assert.True(t, dv.Mode.Admin)
	assert.True(t, dv.Mode.Editable)
}

func TestNewDocumentViewWithAck(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	doc := makeDocumentTB(t, app, "Reglas", true, "https://example.com/reglas")
	other := makeDocumentTB(t, app, "Tarifas", false, "https://example.com/tarifas")
	acked := map[string]struct{}{doc.Id: {}}

	dvAcked := NewDocumentViewWithAck(doc, PlayerRow, acked)
	dvNotAcked := NewDocumentViewWithAck(other, PlayerRow, acked)

	assert.True(t, dvAcked.Acked)
	assert.False(t, dvNotAcked.Acked)
}

func TestDocumentCard_PlayerRowNoControls(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player tab shows doc card without admin controls",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Documentos"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "DocA")
		p2 := makePairTB(tb, app, "DocB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		doc := makeDocumentTB(tb, app, "Normativa", true, "https://example.com/n")
		comp.Set("documents", []string{doc.Id})
		require.NoError(tb, app.Save(comp))

		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.Contains(tb, body, "Normativa", "card shows title")
		assert.Contains(tb, body, `data-testid="document-card"`, "renders via documentCard")
		assert.Contains(tb, body, "Abrir", "open action present")
		assert.NotContains(tb, body, "Editar", "no edit control in player view")
		assert.NotContains(tb, body, "Eliminar", "no delete control in player view")
	}
	s.Test(t)
}

func TestDocumentCard_AdminFullHasControls(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "admin library shows doc card with edit/delete",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Documentos"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupDocRoutes(tb, app, e)
		makeDocumentTB(tb, app, "Reglamento Admin", false, "https://example.com/r")

		s.URL = "/admin/documents"
		admin := makeAdminUserTB(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.Contains(tb, body, "Reglamento Admin", "card shows title")
		assert.Contains(tb, body, `data-testid="document-card"`, "renders via documentCard")
		assert.Contains(tb, body, "Editar", "edit control present in admin view")
		assert.Contains(tb, body, "Eliminar", "delete control present in admin view")
	}
	s.Test(t)
}

func TestDocumentCard_AttachRowHasDetach(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "admin competition detail shows attached doc with detach",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Documentos"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "AttA")
		p2 := makePairTB(tb, app, "AttB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		doc := makeDocumentTB(tb, app, "Manual", false, "https://example.com/m")
		comp.Set("documents", []string{doc.Id})
		require.NoError(tb, app.Save(comp))

		s.URL = "/admin/competitions/" + comp.Id
		admin := makeAdminUserTB(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.Contains(tb, body, "Manual", "attached doc shows title")
		assert.Contains(tb, body, `data-testid="document-attach-row"`, "renders via documentAttachRow")
		assert.Contains(tb, body, "Quitar", "detach control present")
	}
	s.Test(t)
}
