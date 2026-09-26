package handlers_test

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/handlers"
)

func TestNewDocumentView_LinkDoc(t *testing.T) {
	t.Parallel()
	app := handlers.NewTestApp(t)

	doc := handlers.MakeDocumentTB(t, app, "Reglas", true, "https://example.com/reglas")
	dv := handlers.NewDocumentView(doc, handlers.PlayerRow, "")

	assert.Equal(t, "Reglas", dv.Title)
	assert.False(t, dv.IsFile)
	assert.Equal(t, "https://example.com/reglas", dv.OpenURL)
	assert.True(t, dv.IsMandatory)
	assert.False(t, dv.Mode.Admin)
}

func TestNewDocumentView_DefaultFlags(t *testing.T) {
	t.Parallel()
	app := handlers.NewTestApp(t)

	col, _ := app.FindCollectionByNameOrId("documents")
	doc := core.NewRecord(col)
	doc.Set("title", "Tarifas")
	doc.Set("url", "https://example.com/tarifas")
	doc.Set("is_default", true)
	require.NoError(t, app.Save(doc))

	dv := handlers.NewDocumentView(doc, handlers.AdminFull, "")

	assert.Equal(t, "Tarifas", dv.Title)
	assert.False(t, dv.IsFile)
	assert.Equal(t, "https://example.com/tarifas", dv.OpenURL)
	assert.True(t, dv.IsDefault)
	assert.False(t, dv.IsMandatory)
	assert.True(t, dv.Mode.Admin)
	assert.True(t, dv.Mode.Editable)
}

func TestNewDocumentView_FileDoc(t *testing.T) {
	t.Parallel()
	app := handlers.NewTestApp(t)

	col, _ := app.FindCollectionByNameOrId("documents")
	doc := core.NewRecord(col)
	doc.Set("title", "Reglamento")
	doc.Id = "docid123456789"
	// NewDocumentView only reads the field's string value to decide
	// IsFile/OpenURL, so setting it directly on an unsaved record exercises
	// the same branch without going through FileField upload validation.
	doc.Set("file", "reglamento_abc123.pdf")

	dv := handlers.NewDocumentView(doc, handlers.PlayerRow, "")

	assert.True(t, dv.IsFile)
	assert.Equal(t, "/api/files/documents/docid123456789/reglamento_abc123.pdf", dv.OpenURL)
}

func TestNewDocumentViewWithAck(t *testing.T) {
	t.Parallel()
	app := handlers.NewTestApp(t)

	doc := handlers.MakeDocumentTB(t, app, "Reglas", true, "https://example.com/reglas")
	other := handlers.MakeDocumentTB(t, app, "Tarifas", false, "https://example.com/tarifas")
	acked := map[string]struct{}{doc.Id: {}}

	dvAcked := handlers.NewDocumentViewWithAck(doc, handlers.PlayerRow, "", acked)
	dvNotAcked := handlers.NewDocumentViewWithAck(other, handlers.PlayerRow, "", acked)

	assert.True(t, dvAcked.Acked)
	assert.False(t, dvNotAcked.Acked)
}

func TestDocumentCard_PlayerRowNoControls(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "player tab shows doc card without admin controls",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Documentos"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "DocA")
		p2 := handlers.MakePairTB(tb, app, "DocB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		doc := handlers.MakeDocumentTB(tb, app, "Normativa", true, "https://example.com/n")
		comp.Set("documents", []string{doc.Id})
		require.NoError(tb, app.Save(comp))

		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
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
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "admin library shows doc card with edit/delete",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Documentos"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		handlers.MakeDocumentTB(tb, app, "Reglamento Admin", false, "https://example.com/r")

		s.URL = "/admin/documents"
		admin := handlers.MakeAdminUserTB(tb, app)
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
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
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "admin competition detail shows attached doc with detach",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Documentos"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "AttA")
		p2 := handlers.MakePairTB(tb, app, "AttB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		doc := handlers.MakeDocumentTB(tb, app, "Manual", false, "https://example.com/m")
		comp.Set("documents", []string{doc.Id})
		require.NoError(tb, app.Save(comp))

		s.URL = "/admin/competitions/" + comp.Id
		admin := handlers.MakeAdminUserTB(tb, app)
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.Contains(tb, body, "Manual", "attached doc shows title")
		assert.Contains(tb, body, `data-testid="document-attach-row"`, "renders via documentAttachRow")
		assert.Contains(tb, body, "Quitar", "detach control present")
	}
	s.Test(t)
}
