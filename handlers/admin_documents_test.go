package handlers

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/league"
	"padelleague/notify"
	"padelleague/render"
)

func setupDocRoutes(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
	setupFullAdminRoutes(tb, app, e)

	doc := NewDocumentHandler(app, render.New(os.DirFS(".."), "").Page)
	g := e.Router.Group("/admin")
	g.BindFunc(requireAuthTest)
	g.BindFunc(requireAdminTest)
	g.GET("/documents", doc.Documents)
	g.POST("/documents", doc.DocumentsCreate)
	g.POST("/documents/{id}", doc.DocumentsUpdate)
	g.POST("/documents/{id}/delete", doc.DocumentsDelete)

	comp := NewCompetitionHandler(app, league.New(app, notify.NewNotifier(app, "", "")), render.New(os.DirFS(".."), "").Page)
	g.POST("/competitions/{id}/attach-doc", comp.AttachDocument)
	g.POST("/competitions/{id}/detach-doc/{docId}", comp.DetachDocument)
}

func makeDocument(t testing.TB, app core.App, title, url string, isDefault bool) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("documents")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Set("title", title)
	record.Set("url", url)
	record.Set("is_default", isDefault)
	require.NoError(t, app.Save(record))
	return record
}

func TestDocumentsListGET(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /admin/documents lists documents",
		Method:          http.MethodGet,
		URL:             "/admin/documents",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Documentos", "Reglamento"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupDocRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		makeDocument(tb, app, "Reglamento", "https://example.com/rules", false)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestDocumentsCreateWithURLOnly(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/documents with URL only creates doc",
		Method:         http.MethodPost,
		URL:            "/admin/documents",
		ExpectedStatus: 204,
	}
	var adminRec *core.Record
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupDocRoutes(tb, app, e)
		adminRec = makeAdminUserTB(tb, app)
		s.Body = strings.NewReader("title=Reglamento&url=https%3A%2F%2Fexample.com%2Frules&is_mandatory=on")
		hdrs := authHeaders(tb, adminRec)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		docs, err := app.FindRecordsByFilter("documents", "title = 'Reglamento'", "", 0, 0, nil)
		require.NoError(tb, err)
		require.Len(tb, docs, 1)
		assert.Equal(tb, "https://example.com/rules", docs[0].GetString("url"))
		assert.True(tb, docs[0].GetBool("is_mandatory"))
		assert.Equal(tb, "", docs[0].GetString("file"))
	}
	s.Test(t)
}

func TestDocumentsCreateWithFileOnly(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/documents with file only creates doc",
		Method:         http.MethodPost,
		URL:            "/admin/documents",
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupDocRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)

		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		_ = w.WriteField("title", "Tarifas")
		_ = w.WriteField("is_default", "on")
		part, _ := w.CreateFormFile("file", "tarifas.pdf")
		_, _ = part.Write([]byte("%PDF-1.4 fake content"))
		require.NoError(tb, w.Close())

		s.Body = &buf
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = w.FormDataContentType()
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		docs, err := app.FindRecordsByFilter("documents", "title = 'Tarifas'", "", 0, 0, nil)
		require.NoError(tb, err)
		require.Len(tb, docs, 1)
		assert.True(tb, docs[0].GetBool("is_default"))
		assert.NotEmpty(tb, docs[0].GetString("file"))
		assert.Equal(tb, "", docs[0].GetString("url"))
	}
	s.Test(t)
}

func TestDocumentsCreateWithNeither(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/documents with neither file nor URL rejects",
		Method:          http.MethodPost,
		URL:             "/admin/documents",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Añade un archivo o un enlace"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupDocRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		s.Body = strings.NewReader("title=Empty")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		docs, _ := app.FindRecordsByFilter("documents", "title = 'Empty'", "", 0, 0, nil)
		assert.Empty(tb, docs)
	}
	s.Test(t)
}

func TestDocumentsCreateWithBoth(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/documents with both file and URL rejects",
		Method:          http.MethodPost,
		URL:             "/admin/documents",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Añade un archivo o un enlace"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupDocRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)

		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		_ = w.WriteField("title", "Both")
		_ = w.WriteField("url", "https://example.com")
		part, _ := w.CreateFormFile("file", "both.pdf")
		_, _ = part.Write([]byte("data"))
		require.NoError(tb, w.Close())

		s.Body = &buf
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = w.FormDataContentType()
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		docs, _ := app.FindRecordsByFilter("documents", "title = 'Both'", "", 0, 0, nil)
		assert.Empty(tb, docs)
	}
	s.Test(t)
}

func TestDocumentsCreateEmptyTitle(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/documents empty title rejects",
		Method:          http.MethodPost,
		URL:             "/admin/documents",
		ExpectedStatus:  200,
		ExpectedContent: []string{"El título es obligatorio"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupDocRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		s.Body = strings.NewReader("title=&url=https%3A%2F%2Fexample.com")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestDocumentsUpdate(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/documents/{id} updates doc",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var docID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupDocRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		doc := makeDocument(tb, app, "Old Title", "https://old.com", false)
		docID = doc.Id
		s.URL = "/admin/documents/" + doc.Id
		s.Body = strings.NewReader("title=New+Title&url=https%3A%2F%2Fnew.com&is_mandatory=on")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		doc, err := app.FindRecordById("documents", docID)
		require.NoError(tb, err)
		assert.Equal(tb, "New Title", doc.GetString("title"))
		assert.Equal(tb, "https://new.com", doc.GetString("url"))
		assert.True(tb, doc.GetBool("is_mandatory"))
	}
	s.Test(t)
}

func TestDocumentsDelete(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/documents/{id}/delete removes doc",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var docID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupDocRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		doc := makeDocument(tb, app, "To Delete", "https://del.com", false)
		docID = doc.Id
		s.URL = "/admin/documents/" + doc.Id + "/delete"
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		_, err := app.FindRecordById("documents", docID)
		assert.Error(tb, err)
	}
	s.Test(t)
}

func TestDocumentsDefaultAndMandatoryFlags(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/documents saves default+mandatory flags",
		Method:         http.MethodPost,
		URL:            "/admin/documents",
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupDocRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		s.Body = strings.NewReader("title=Full+Flags&url=https%3A%2F%2Fflags.com&is_default=on&is_mandatory=on")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		docs, err := app.FindRecordsByFilter("documents", "title = 'Full Flags'", "", 0, 0, nil)
		require.NoError(tb, err)
		require.Len(tb, docs, 1)
		assert.True(tb, docs[0].GetBool("is_default"))
		assert.True(tb, docs[0].GetBool("is_mandatory"))
	}
	s.Test(t)
}

// T4 tests — preload defaults on competition create + attach/detach

func TestCompetitionCreatePreloadsDefaultDocs(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions preloads default docs",
		Method:         http.MethodPost,
		URL:            "/admin/competitions",
		ExpectedStatus: 204,
	}
	var defaultDocIDs []string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupDocRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		d1 := makeDocument(tb, app, "Default1", "https://d1.com", true)
		d2 := makeDocument(tb, app, "Default2", "https://d2.com", true)
		makeDocument(tb, app, "Not Default", "https://nd.com", false)
		defaultDocIDs = []string{d1.Id, d2.Id}
		s.Body = strings.NewReader("name=Preload+Test&type=league")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		comps, err := app.FindRecordsByFilter("competitions", "name = 'Preload Test'", "", 0, 0, nil)
		require.NoError(tb, err)
		require.Len(tb, comps, 1)
		attached := comps[0].GetStringSlice("documents")
		for _, id := range defaultDocIDs {
			assert.Contains(tb, attached, id)
		}
		assert.Len(tb, attached, 2)
	}
	s.Test(t)
}

func TestCompetitionAttachDocument(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions/{id}/attach-doc attaches a doc",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, docID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupDocRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "AttA")
		p2 := makePairTB(tb, app, "AttB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		compID = comp.Id
		doc := makeDocument(tb, app, "Attach Me", "https://att.com", false)
		docID = doc.Id
		s.URL = "/admin/competitions/" + comp.Id + "/attach-doc"
		s.Body = strings.NewReader("document=" + doc.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		comp, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.Contains(tb, comp.GetStringSlice("documents"), docID)
	}
	s.Test(t)
}

func TestCompetitionDetachDocument(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions/{id}/detach-doc/{docId} detaches doc, keeps library",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, docID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupDocRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "DetA")
		p2 := makePairTB(tb, app, "DetB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		compID = comp.Id
		doc := makeDocument(tb, app, "Detach Me", "https://det.com", false)
		docID = doc.Id
		comp.Set("documents", []string{doc.Id})
		require.NoError(tb, app.Save(comp))
		s.URL = fmt.Sprintf("/admin/competitions/%s/detach-doc/%s", comp.Id, doc.Id)
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		comp, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.NotContains(tb, comp.GetStringSlice("documents"), docID)
		_, err = app.FindRecordById("documents", docID)
		assert.NoError(tb, err, "document should still exist in the library")
	}
	s.Test(t)
}

func TestCompetitionDetachKeepsOtherComps(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "detach from C1 keeps doc attached to C2",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var comp1ID, comp2ID, docID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupDocRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "KC1A")
		p2 := makePairTB(tb, app, "KC1B")
		comp1 := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp1ID = comp1.Id
		p3 := makePairTB(tb, app, "KC2A")
		p4 := makePairTB(tb, app, "KC2B")
		comp2 := makeCompetitionTB(tb, app, "league", []*core.Record{p3, p4})
		comp2ID = comp2.Id
		doc := makeDocument(tb, app, "Shared Doc", "https://shared.com", false)
		docID = doc.Id
		comp1.Set("documents", []string{doc.Id})
		require.NoError(tb, app.Save(comp1))
		comp2.Set("documents", []string{doc.Id})
		require.NoError(tb, app.Save(comp2))
		s.URL = fmt.Sprintf("/admin/competitions/%s/detach-doc/%s", comp1.Id, doc.Id)
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c1, _ := app.FindRecordById("competitions", comp1ID)
		assert.NotContains(tb, c1.GetStringSlice("documents"), docID)
		c2, _ := app.FindRecordById("competitions", comp2ID)
		assert.Contains(tb, c2.GetStringSlice("documents"), docID)
	}
	s.Test(t)
}
