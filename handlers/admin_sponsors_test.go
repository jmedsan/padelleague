package handlers

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/filesystem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/league"
	"padelleague/notify"
	"padelleague/render"
)

func setupSponsorRoutes(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
	setupFullAdminRoutes(tb, app, e)

	sponsor := NewAdminSponsorHandler(app, render.New(os.DirFS(".."), "", true).Page)
	g := e.Router.Group("/admin")
	g.BindFunc(requireAuthTest)
	g.BindFunc(requireAdminTest)
	g.GET("/sponsors", sponsor.Sponsors)
	g.POST("/sponsors", sponsor.SponsorsCreate)
	g.POST("/sponsors/{id}", sponsor.SponsorsUpdate)
	g.POST("/sponsors/{id}/delete", sponsor.SponsorsDelete)
	g.POST("/competitions/{id}/attach-sponsor", sponsor.AttachSponsor)
	g.POST("/competitions/{id}/detach-sponsor/{sponsorId}", sponsor.DetachSponsor)

	n := notify.NewNotifier(app, "", "")
	_ = league.New(app, n)
}

func createFormImagePart(w *multipart.Writer, field, filename string) (io.Writer, error) {
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="`+field+`"; filename="`+filename+`"`)
	header.Set("Content-Type", "image/png")
	return w.CreatePart(header)
}

func makeSponsorTB(t testing.TB, app core.App, name, rawURL string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("sponsors")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Set("name", name)
	record.Set("url", rawURL)
	f, err := filesystem.NewFileFromBytes(testPNGBytes(t, 100, 100), "logo.png")
	require.NoError(t, err)
	record.Set("logo", f)
	require.NoError(t, app.Save(record))
	return record
}

func TestSponsorsListGET(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /admin/sponsors lists sponsors",
		Method:          http.MethodGet,
		URL:             "/admin/sponsors",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Patrocinadores", "Decathlon"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSponsorRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		makeSponsorTB(tb, app, "Decathlon", "https://www.decathlon.es")
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestSponsorsCreateValidImage(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/sponsors with a valid logo creates the sponsor",
		Method:         http.MethodPost,
		URL:            "/admin/sponsors",
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSponsorRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)

		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		require.NoError(tb, w.WriteField("name", "Wurko"))
		require.NoError(tb, w.WriteField("url", "https://www.wurko.es"))
		part, err := createFormImagePart(w, "logo", "logo.png")
		require.NoError(tb, err)
		_, err = part.Write(testPNGBytes(tb, 200, 200))
		require.NoError(tb, err)
		require.NoError(tb, w.Close())

		s.Body = &buf
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = w.FormDataContentType()
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		sponsors, err := app.FindRecordsByFilter("sponsors", "name = 'Wurko'", "", 0, 0, nil)
		require.NoError(tb, err)
		require.Len(tb, sponsors, 1)
		assert.Equal(tb, "https://www.wurko.es", sponsors[0].GetString("url"))
		assert.NotEmpty(tb, sponsors[0].GetString("logo"))
	}
	s.Test(t)
}

func TestSponsorsCreateMissingName(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/sponsors without a name rejects",
		Method:          http.MethodPost,
		URL:             "/admin/sponsors",
		ExpectedStatus:  200,
		ExpectedContent: []string{"El nombre es obligatorio"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSponsorRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)

		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		require.NoError(tb, w.WriteField("url", "https://example.com"))
		part, err := createFormImagePart(w, "logo", "logo.png")
		require.NoError(tb, err)
		_, err = part.Write(testPNGBytes(tb, 100, 100))
		require.NoError(tb, err)
		require.NoError(tb, w.Close())

		s.Body = &buf
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = w.FormDataContentType()
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		sponsors, _ := app.FindRecordsByFilter("sponsors", "url = 'https://example.com'", "", 0, 0, nil)
		assert.Empty(tb, sponsors)
	}
	s.Test(t)
}

func TestSponsorsCreateMissingLogo(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/sponsors without a logo rejects",
		Method:          http.MethodPost,
		URL:             "/admin/sponsors",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Selecciona un logo"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSponsorRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		s.Body = strings.NewReader("name=NoLogo")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		sponsors, _ := app.FindRecordsByFilter("sponsors", "name = 'NoLogo'", "", 0, 0, nil)
		assert.Empty(tb, sponsors)
	}
	s.Test(t)
}

func TestSponsorsCreateNonImageRejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/sponsors with a non-image file rejects",
		Method:          http.MethodPost,
		URL:             "/admin/sponsors",
		ExpectedStatus:  200,
		ExpectedContent: []string{"El archivo debe ser una imagen"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSponsorRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)

		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		require.NoError(tb, w.WriteField("name", "NotAnImage"))
		part, err := w.CreateFormFile("logo", "logo.txt")
		require.NoError(tb, err)
		_, err = part.Write([]byte("not an image"))
		require.NoError(tb, err)
		require.NoError(tb, w.Close())

		s.Body = &buf
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = w.FormDataContentType()
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		sponsors, _ := app.FindRecordsByFilter("sponsors", "name = 'NotAnImage'", "", 0, 0, nil)
		assert.Empty(tb, sponsors)
	}
	s.Test(t)
}

func TestSponsorsUpdateNameAndURL(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/sponsors/{id} updates name and url, keeping the logo",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var sponsorID, origLogo string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSponsorRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		sponsor := makeSponsorTB(tb, app, "Old Name", "https://old.com")
		sponsorID = sponsor.Id
		origLogo = sponsor.GetString("logo")
		require.NotEmpty(tb, origLogo)

		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		require.NoError(tb, w.WriteField("name", "New Name"))
		require.NoError(tb, w.WriteField("url", "https://new.com"))
		require.NoError(tb, w.Close())

		s.URL = "/admin/sponsors/" + sponsor.Id
		s.Body = &buf
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = w.FormDataContentType()
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		sponsor, err := app.FindRecordById("sponsors", sponsorID)
		require.NoError(tb, err)
		assert.Equal(tb, "New Name", sponsor.GetString("name"))
		assert.Equal(tb, "https://new.com", sponsor.GetString("url"))
		assert.Equal(tb, origLogo, sponsor.GetString("logo"), "logo must be unchanged when no new file is uploaded")
	}
	s.Test(t)
}

func TestSponsorsUpdateReplacesLogo(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/sponsors/{id} with a new logo replaces the file",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var sponsorID, origLogo string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSponsorRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		sponsor := makeSponsorTB(tb, app, "Logo Swap", "https://swap.com")
		sponsorID = sponsor.Id
		origLogo = sponsor.GetString("logo")
		require.NotEmpty(tb, origLogo)

		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		require.NoError(tb, w.WriteField("name", "Logo Swap"))
		part, err := createFormImagePart(w, "logo", "new-logo.png")
		require.NoError(tb, err)
		_, err = part.Write(testPNGBytes(tb, 200, 200))
		require.NoError(tb, err)
		require.NoError(tb, w.Close())

		s.URL = "/admin/sponsors/" + sponsor.Id
		s.Body = &buf
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = w.FormDataContentType()
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		sponsor, err := app.FindRecordById("sponsors", sponsorID)
		require.NoError(tb, err)
		assert.NotEmpty(tb, sponsor.GetString("logo"))
		assert.NotEqual(tb, origLogo, sponsor.GetString("logo"), "logo file must be replaced")
	}
	s.Test(t)
}

func TestSponsorsUpdateMissingSponsor(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/sponsors/{id} for a missing sponsor rejects",
		Method:          http.MethodPost,
		URL:             "/admin/sponsors/nonexistent",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Patrocinador no encontrado"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSponsorRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)

		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		require.NoError(tb, w.WriteField("name", "Whatever"))
		require.NoError(tb, w.Close())

		s.Body = &buf
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = w.FormDataContentType()
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestSponsorsDelete(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/sponsors/{id}/delete removes the sponsor",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var sponsorID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSponsorRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		sponsor := makeSponsorTB(tb, app, "To Delete", "https://del.com")
		sponsorID = sponsor.Id
		s.URL = "/admin/sponsors/" + sponsor.Id + "/delete"
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		_, err := app.FindRecordById("sponsors", sponsorID)
		assert.Error(tb, err)
	}
	s.Test(t)
}

func TestSponsorAttach(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions/{id}/attach-sponsor attaches a sponsor",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, sponsorID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSponsorRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "SpAttA")
		p2 := makePairTB(tb, app, "SpAttB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		compID = comp.Id
		sponsor := makeSponsorTB(tb, app, "Attach Me", "https://att.com")
		sponsorID = sponsor.Id
		s.URL = "/admin/competitions/" + comp.Id + "/attach-sponsor"
		s.Body = strings.NewReader("sponsor=" + sponsor.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		comp, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.Contains(tb, comp.GetStringSlice("sponsors"), sponsorID)
	}
	s.Test(t)
}

func TestSponsorDetach(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions/{id}/detach-sponsor/{sponsorId} detaches, keeps library",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, sponsorID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSponsorRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "SpDetA")
		p2 := makePairTB(tb, app, "SpDetB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		compID = comp.Id
		sponsor := makeSponsorTB(tb, app, "Detach Me", "https://det.com")
		sponsorID = sponsor.Id
		comp.Set("sponsors", []string{sponsor.Id})
		require.NoError(tb, app.Save(comp))
		s.URL = fmt.Sprintf("/admin/competitions/%s/detach-sponsor/%s", comp.Id, sponsor.Id)
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		comp, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.NotContains(tb, comp.GetStringSlice("sponsors"), sponsorID)
		_, err = app.FindRecordById("sponsors", sponsorID)
		assert.NoError(tb, err, "sponsor should still exist in the library")
	}
	s.Test(t)
}

func TestSponsorDetachKeepsOtherComps(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "detach sponsor from C1 keeps it attached to C2",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var comp1ID, comp2ID, sponsorID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupSponsorRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "SpKC1A")
		p2 := makePairTB(tb, app, "SpKC1B")
		comp1 := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp1ID = comp1.Id
		p3 := makePairTB(tb, app, "SpKC2A")
		p4 := makePairTB(tb, app, "SpKC2B")
		comp2 := makeCompetitionTB(tb, app, "league", []*core.Record{p3, p4})
		comp2ID = comp2.Id
		sponsor := makeSponsorTB(tb, app, "Shared Sponsor", "https://shared.com")
		sponsorID = sponsor.Id
		comp1.Set("sponsors", []string{sponsor.Id})
		require.NoError(tb, app.Save(comp1))
		comp2.Set("sponsors", []string{sponsor.Id})
		require.NoError(tb, app.Save(comp2))
		s.URL = fmt.Sprintf("/admin/competitions/%s/detach-sponsor/%s", comp1.Id, sponsor.Id)
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c1, _ := app.FindRecordById("competitions", comp1ID)
		assert.NotContains(tb, c1.GetStringSlice("sponsors"), sponsorID)
		c2, _ := app.FindRecordById("competitions", comp2ID)
		assert.Contains(tb, c2.GetStringSlice("sponsors"), sponsorID)
	}
	s.Test(t)
}
