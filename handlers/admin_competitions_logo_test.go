package handlers

import (
	"bytes"
	"image"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func multipartLogoBody(t testing.TB, imgBytes []byte) (*bytes.Buffer, string) {
	t.Helper()
	return multipartLogoBodyWithType(t, imgBytes, "logo.png", "image/png")
}

func multipartLogoBodyWithType(t testing.TB, imgBytes []byte, filename, contentType string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="logo"; filename="`+filename+`"`)
	header.Set("Content-Type", contentType)
	part, err := w.CreatePart(header)
	require.NoError(t, err)
	_, err = part.Write(imgBytes)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return &buf, w.FormDataContentType()
}

func TestLogoUpload_NonAdminRejected(t *testing.T) {
	t.Parallel()
	var compID string
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions/{id}/logo as a non-admin is redirected",
		Method:         http.MethodPost,
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Logo A")
		p2 := makePairTB(tb, app, "Logo B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/logo"

		user := makeUserTB(tb, app, "Regular Player", "")
		body, contentType := multipartLogoBody(tb, testPNGBytes(tb, 100, 100))
		s.Body = body
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = contentType
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		assert.Contains(tb, res.Header.Get("Location"), "/login")
		comp, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.Empty(tb, comp.GetString("logo"))
	}
	s.Test(t)
}

func TestLogoUpload_MissingCompetitionRejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions/{id}/logo with an unknown competition id",
		Method:         http.MethodPost,
		URL:            "/admin/competitions/does-not-exist/logo",
		ExpectedStatus: 200,
		ExpectedContent: []string{
			"Competición no encontrada",
		},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		body, contentType := multipartLogoBody(tb, testPNGBytes(tb, 100, 100))
		s.Body = body
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = contentType
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestLogoUpload_NoFileRejected(t *testing.T) {
	t.Parallel()
	var compID string
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/competitions/{id}/logo with no file",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Selecciona una imagen"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "Logo A")
		p2 := makePairTB(tb, app, "Logo B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/logo"

		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		require.NoError(tb, w.Close())
		s.Body = &buf
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = w.FormDataContentType()
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		comp, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.Empty(tb, comp.GetString("logo"))
	}
	s.Test(t)
}

func TestLogoUpload_NonImageRejected(t *testing.T) {
	t.Parallel()
	var compID string
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/competitions/{id}/logo with a non-image file is rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"El archivo debe ser una imagen"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "Logo A")
		p2 := makePairTB(tb, app, "Logo B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/logo"

		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		part, err := w.CreateFormFile("logo", "not-an-image.txt")
		require.NoError(tb, err)
		_, err = part.Write([]byte("plain text content"))
		require.NoError(tb, err)
		require.NoError(tb, w.Close())

		s.Body = &buf
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = w.FormDataContentType()
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		comp, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.Empty(tb, comp.GetString("logo"))
	}
	s.Test(t)
}

func TestLogoUpload_OversizedFileRejected(t *testing.T) {
	t.Parallel()
	var compID string
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/competitions/{id}/logo with a file over 5MB is rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"La imagen no puede superar los 5 MB"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "Logo A")
		p2 := makePairTB(tb, app, "Logo B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/logo"

		oversized := make([]byte, avatarMaxUploadSize+1)
		body, contentType := multipartLogoBody(tb, oversized)
		s.Body = body
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = contentType
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		comp, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.Empty(tb, comp.GetString("logo"))
	}
	s.Test(t)
}

func TestLogoUpload_InvalidImageBytesRejected(t *testing.T) {
	t.Parallel()
	var compID string
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/competitions/{id}/logo with garbage bytes claiming to be an image",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Imagen no válida"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "Logo A")
		p2 := makePairTB(tb, app, "Logo B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/logo"

		// Content-Type claims image/png but the bytes are not a valid image,
		// so it passes the Content-Type prefix check and fails decoding.
		body, contentType := multipartLogoBodyWithType(tb, []byte("not actually a png"), "fake.png", "image/png")
		s.Body = body
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = contentType
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		comp, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.Empty(tb, comp.GetString("logo"))
	}
	s.Test(t)
}

func TestLogoUpload_ValidImageSavesAndRedirects(t *testing.T) {
	t.Parallel()
	var compID string
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions/{id}/logo with a valid image sets the logo and redirects",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "Logo A")
		p2 := makePairTB(tb, app, "Logo B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/logo"

		body, contentType := multipartLogoBody(tb, testPNGBytes(tb, 800, 600))
		s.Body = body
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = contentType
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/admin/competitions/"+compID, res.Header.Get("HX-Redirect"))
		comp, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.NotEmpty(tb, comp.GetString("logo"))
	}
	s.Test(t)
}

// TestLogoUpload_ExifOrientationCorrected proves the competition logo
// upload reuses the same compressAvatar pipeline as player avatars — EXIF
// orientation correction included — rather than a divergent code path.
func TestLogoUpload_ExifOrientationCorrected(t *testing.T) {
	t.Parallel()
	var compID string
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions/{id}/logo with EXIF orientation 6 rotates the image upright",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "Logo A")
		p2 := makePairTB(tb, app, "Logo B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/logo"

		// Landscape source (left=red, right=blue) tagged Orientation=6
		// (rotate 90 CW to display) must come out with red on top, blue on
		// bottom once corrected.
		jpegBytes := testJPEGWithOrientation(tb, 200, 100, 6)
		body, contentType := multipartLogoBodyWithType(tb, jpegBytes, "logo.jpg", "image/jpeg")
		s.Body = body
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = contentType
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		comp, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		filename := comp.GetString("logo")
		require.NotEmpty(tb, filename)

		fsys, err := app.NewFilesystem()
		require.NoError(tb, err)
		defer func() { _ = fsys.Close() }()

		r, err := fsys.GetReader(comp.BaseFilesPath() + "/" + filename)
		require.NoError(tb, err)
		defer func() { _ = r.Close() }()

		img, _, err := image.Decode(r)
		require.NoError(tb, err)
		b := img.Bounds()
		topR, _, topB, _ := img.At(b.Min.X, b.Min.Y).RGBA()
		bottomR, _, bottomB, _ := img.At(b.Min.X, b.Max.Y-1).RGBA()
		assert.Greater(tb, topR, topB, "top of the corrected image should be the red half")
		assert.Greater(tb, bottomB, bottomR, "bottom of the corrected image should be the blue half")
	}
	s.Test(t)
}
