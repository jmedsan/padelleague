package handlers_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/filesystem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/handlers"
)

// promoForm is the multipart body the admin promotion form posts.
type promoForm struct {
	fields map[string]string
	image  bool // attach a valid PNG as "image"
	file   *struct{ name, contentType string }
}

func (f promoForm) body(tb testing.TB) (*bytes.Buffer, string) {
	tb.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range f.fields {
		require.NoError(tb, w.WriteField(k, v))
	}
	if f.image {
		part, err := createFormImagePart(w, "image", "oferta.png")
		require.NoError(tb, err)
		_, err = part.Write(testPNGBytes(tb, 200, 100))
		require.NoError(tb, err)
	}
	if f.file != nil {
		part, err := w.CreatePart(map[string][]string{
			"Content-Disposition": {`form-data; name="image"; filename="` + f.file.name + `"`},
			"Content-Type":        {f.file.contentType},
		})
		require.NoError(tb, err)
		_, err = part.Write([]byte("not an image"))
		require.NoError(tb, err)
	}
	require.NoError(tb, w.Close())
	return &buf, w.FormDataContentType()
}

// makePromo creates an offer and returns it for further edits.
func makePromo(tb testing.TB, app core.App, id, title string, active bool, expiresOn string) *core.Record {
	tb.Helper()
	makePromotionTB(tb, app, id, title, "https://a.example", active, expiresOn)
	rec, err := app.FindRecordById("promotions", id)
	require.NoError(tb, err)
	return rec
}

func promoCount(tb testing.TB, app core.App) int {
	tb.Helper()
	recs, err := app.FindAllRecords("promotions")
	require.NoError(tb, err)
	return len(recs)
}

func notificationCount(tb testing.TB, app core.App) int {
	tb.Helper()
	recs, err := app.FindAllRecords("notifications")
	require.NoError(tb, err)
	return len(recs)
}

func promoPost(tb testing.TB, s *tests.ApiScenario, app core.App, f promoForm) {
	tb.Helper()
	body, ct := f.body(tb)
	s.Body = body
	hdrs := handlers.AuthHeaders(tb, handlers.MakeAdminUserTB(tb, app))
	hdrs["Content-Type"] = ct
	s.Headers = hdrs
}

func TestPromotionAdminCreateMinimal(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/promotions with title and URL only",
		Method:         http.MethodPost,
		URL:            "/admin/promotions",
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		promoPost(tb, s, app, promoForm{fields: map[string]string{
			"title": "2x1 en palas", "url": "https://tienda.example/palas",
		}})
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		recs, err := app.FindAllRecords("promotions")
		require.NoError(tb, err)
		require.Len(tb, recs, 1)
		r := recs[0]
		assert.Equal(tb, "2x1 en palas", r.GetString("title"))
		assert.Equal(tb, "https://tienda.example/palas", r.GetString("url"))
		assert.Empty(tb, r.GetString("image"))
		assert.True(tb, r.GetDateTime("expires_on").IsZero(), "no expiry")
		assert.True(tb, r.GetBool("active"), "a new offer is active")
		assert.Equal(tb, 0, r.GetInt("views"))
		assert.Equal(tb, 0, r.GetInt("clicks"))
		var flashMsg string
		for _, c := range res.Cookies() {
			if c.Name == "flash_msg" {
				flashMsg, _ = url.QueryUnescape(c.Value)
			}
		}
		assert.Equal(tb, "Promoción creada", flashMsg)
		assert.Zero(tb, notificationCount(tb, app), "no notification")
	}
	handlers.ExpectRedirect(s, redirectTo("/admin/promotions"))
	s.Test(t)
}

func TestPromotionAdminCreateFull(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/promotions with image, expiry and uppercase scheme",
		Method:         http.MethodPost,
		URL:            "/admin/promotions",
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		promoPost(tb, s, app, promoForm{image: true, fields: map[string]string{
			"title": "Oferta", "description": "Descripción", "url": "HTTPS://a.example/x", "expires_on": "2031-05-20",
		}})
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		recs, err := app.FindAllRecords("promotions")
		require.NoError(tb, err)
		require.Len(tb, recs, 1)
		r := recs[0]
		assert.Equal(tb, "https://a.example/x", r.GetString("url"), "scheme lowercased")
		assert.Equal(tb, "Descripción", r.GetString("description"))
		assert.Equal(tb, "2031-05-20", r.GetDateTime("expires_on").Time().UTC().Format("2006-01-02"))
		assert.Equal(tb, 12, r.GetDateTime("expires_on").Time().UTC().Hour(), "stored at 12:00 UTC")
		require.NotEmpty(tb, r.GetString("image"))
		// Compressed by CompressLogoBytes: re-encoded as JPEG.
		fsys, err := app.NewFilesystem()
		require.NoError(tb, err)
		defer func() { _ = fsys.Close() }()
		blob, err := fsys.GetReader(r.BaseFilesPath() + "/" + r.GetString("image"))
		require.NoError(tb, err)
		defer func() { _ = blob.Close() }()
		head := make([]byte, 3)
		_, err = blob.Read(head)
		require.NoError(tb, err)
		assert.Equal(tb, []byte{0xff, 0xd8, 0xff}, head, "JPEG signature")
	}
	handlers.ExpectRedirect(s, redirectTo("/admin/promotions"))
	s.Test(t)
}

func TestPromotionAdminCreateExpiredIsSaved(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/promotions with a past expiry saves the offer",
		Method:         http.MethodPost,
		URL:            "/admin/promotions",
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		promoPost(tb, s, app, promoForm{fields: map[string]string{
			"title": "Vieja", "url": "https://a.example", "expires_on": "2000-01-01",
		}})
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		assert.Equal(tb, 1, promoCount(tb, app))
	}
	handlers.ExpectRedirect(s, redirectTo("/admin/promotions"))
	s.Test(t)
}

func TestPromotionAdminCreateInvalid(t *testing.T) {
	t.Parallel()
	const goodURL = "https://a.example"
	cases := []struct {
		name string
		form promoForm
		want string
	}{
		{"empty title", promoForm{fields: map[string]string{"title": "", "url": goodURL}}, "El título es obligatorio"},
		{"blank title", promoForm{fields: map[string]string{"title": "   ", "url": goodURL}}, "El título es obligatorio"},
		{"title of 101 characters", promoForm{fields: map[string]string{"title": strings.Repeat("a", 101), "url": goodURL}}, "El título no puede superar los 100 caracteres"},
		{"description of 301 characters", promoForm{fields: map[string]string{"title": "T", "description": strings.Repeat("a", 301), "url": goodURL}}, "La descripción no puede superar los 300 caracteres"},
		{"javascript URL", promoForm{fields: map[string]string{"title": "T", "url": "javascript:alert(1)"}}, "El enlace debe empezar por http:// o https://"},
		{"ftp URL", promoForm{fields: map[string]string{"title": "T", "url": "ftp://a.example"}}, "El enlace debe empezar por http:// o https://"},
		{"protocol-relative URL", promoForm{fields: map[string]string{"title": "T", "url": "//a.example"}}, "El enlace debe empezar por http:// o https://"},
		{"scheme without host", promoForm{fields: map[string]string{"title": "T", "url": "https://"}}, "El enlace debe empezar por http:// o https://"},
		{"empty URL", promoForm{fields: map[string]string{"title": "T", "url": ""}}, "El enlace debe empezar por http:// o https://"},
		{"invalid expiry", promoForm{fields: map[string]string{"title": "T", "url": goodURL, "expires_on": "31/12/2030"}}, "La fecha de caducidad no es válida"},
		{"impossible expiry", promoForm{fields: map[string]string{"title": "T", "url": goodURL, "expires_on": "2030-02-31"}}, "La fecha de caducidad no es válida"},
		{"non-image file", promoForm{fields: map[string]string{"title": "T", "url": goodURL}, file: &struct{ name, contentType string }{"oferta.txt", "text/plain"}}, "El archivo debe ser una imagen"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := &tests.ApiScenario{
				TestAppFactory:  handlers.TestAppFactory,
				Name:            "POST /admin/promotions " + tc.name,
				Method:          http.MethodPost,
				URL:             "/admin/promotions",
				ExpectedStatus:  200,
				ExpectedContent: []string{tc.want},
			}
			s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				setupProductionRoutes(tb, app, e)
				promoPost(tb, s, app, tc.form)
			}
			s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
				assert.Zero(tb, promoCount(tb, app), "nothing saved")
			}
			s.Test(t)
		})
	}
}

func TestPromotionAdminCreateImageTooLarge(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /admin/promotions with an image over 5 MB",
		Method:          http.MethodPost,
		URL:             "/admin/promotions",
		ExpectedStatus:  200,
		ExpectedContent: []string{"La imagen no puede superar los 5 MB"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		require.NoError(tb, w.WriteField("title", "T"))
		require.NoError(tb, w.WriteField("url", "https://a.example"))
		part, err := createFormImagePart(w, "image", "grande.png")
		require.NoError(tb, err)
		_, err = part.Write(bytes.Repeat([]byte{0}, 5<<20+1))
		require.NoError(tb, err)
		require.NoError(tb, w.Close())
		s.Body = &buf
		hdrs := handlers.AuthHeaders(tb, handlers.MakeAdminUserTB(tb, app))
		hdrs["Content-Type"] = w.FormDataContentType()
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		assert.Zero(tb, promoCount(tb, app), "nothing saved")
	}
	s.Test(t)
}

func TestPromotionAdminUpdateTitleKeepsTheRest(t *testing.T) {
	t.Parallel()
	const id = "promoedit000001"
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/promotions/{id} changing only the title",
		Method:         http.MethodPost,
		URL:            "/admin/promotions/" + id,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		rec := makePromo(tb, app, id, "Antes", true, promoFuture)
		f, err := filesystem.NewFileFromBytes(testPNGBytes(tb, 50, 50), "x.png")
		require.NoError(tb, err)
		rec.Set("image", f)
		rec.Set("views", 7)
		rec.Set("clicks", 3)
		require.NoError(tb, app.Save(rec))
		promoPost(tb, s, app, promoForm{fields: map[string]string{
			"title": "Después", "description": "Descripción de Antes", "url": "https://a.example", "expires_on": "2999-12-31",
		}})
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		r, err := app.FindRecordById("promotions", id)
		require.NoError(tb, err)
		assert.Equal(tb, "Después", r.GetString("title"))
		assert.NotEmpty(tb, r.GetString("image"), "image kept")
		assert.Equal(tb, 7, r.GetInt("views"))
		assert.Equal(tb, 3, r.GetInt("clicks"))
		assert.True(tb, r.GetBool("active"))
		assert.Equal(tb, "2999-12-31", r.GetDateTime("expires_on").Time().UTC().Format("2006-01-02"))
		assert.Zero(tb, notificationCount(tb, app), "no notification")
	}
	handlers.ExpectRedirect(s, redirectTo("/admin/promotions"))
	s.Test(t)
}

func TestPromotionAdminUpdateRemovesImageAndExpiry(t *testing.T) {
	t.Parallel()
	const id = "promoedit000002"
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/promotions/{id} with Quitar imagen and an empty expiry",
		Method:         http.MethodPost,
		URL:            "/admin/promotions/" + id,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		rec := makePromo(tb, app, id, "Oferta", true, promoFuture)
		f, err := filesystem.NewFileFromBytes(testPNGBytes(tb, 50, 50), "x.png")
		require.NoError(tb, err)
		rec.Set("image", f)
		require.NoError(tb, app.Save(rec))
		promoPost(tb, s, app, promoForm{fields: map[string]string{
			"title": "Oferta", "url": "https://a.example", "expires_on": "", "remove_image": "on",
		}})
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		r, err := app.FindRecordById("promotions", id)
		require.NoError(tb, err)
		assert.Empty(tb, r.GetString("image"))
		assert.True(tb, r.GetDateTime("expires_on").IsZero())
	}
	handlers.ExpectRedirect(s, redirectTo("/admin/promotions"))
	s.Test(t)
}

func TestPromotionAdminUpdateInvalid(t *testing.T) {
	t.Parallel()
	const id = "promoedit000003"
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /admin/promotions/{id} applies the create validation",
		Method:          http.MethodPost,
		URL:             "/admin/promotions/" + id,
		ExpectedStatus:  200,
		ExpectedContent: []string{"El enlace debe empezar por http:// o https://"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		makePromotionTB(tb, app, id, "Oferta", "https://a.example", true, "")
		promoPost(tb, s, app, promoForm{fields: map[string]string{"title": "Nueva", "url": "javascript:alert(1)"}})
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		r, err := app.FindRecordById("promotions", id)
		require.NoError(tb, err)
		assert.Equal(tb, "Oferta", r.GetString("title"), "nothing saved")
		assert.Equal(tb, "https://a.example", r.GetString("url"))
	}
	s.Test(t)
}

func TestPromotionAdminUpdateUnknown(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /admin/promotions/{id} unknown",
		Method:          http.MethodPost,
		URL:             "/admin/promotions/nope00000000000",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Promoción no encontrada"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		promoPost(tb, s, app, promoForm{fields: map[string]string{"title": "T", "url": "https://a.example"}})
	}
	s.Test(t)
}

func TestPromotionAdminToggle(t *testing.T) {
	t.Parallel()
	const id = "promotoggle0001"
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/promotions/{id}/toggle switches off an active offer",
		Method:         http.MethodPost,
		URL:            "/admin/promotions/" + id + "/toggle",
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		rec := makePromo(tb, app, id, "Oferta", true, promoFuture)
		rec.Set("views", 4)
		rec.Set("clicks", 2)
		require.NoError(tb, app.Save(rec))
		s.Headers = handlers.AuthHeaders(tb, handlers.MakeAdminUserTB(tb, app))
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		r, err := app.FindRecordById("promotions", id)
		require.NoError(tb, err)
		assert.False(tb, r.GetBool("active"))
		assert.Equal(tb, "Oferta", r.GetString("title"))
		assert.Equal(tb, "https://a.example", r.GetString("url"))
		assert.Equal(tb, 4, r.GetInt("views"))
		assert.Equal(tb, 2, r.GetInt("clicks"))
		assert.Equal(tb, "2999-12-31", r.GetDateTime("expires_on").Time().UTC().Format("2006-01-02"))
		assert.Zero(tb, notificationCount(tb, app), "no notification")
	}
	handlers.ExpectRedirect(s, redirectTo("/admin/promotions"))
	s.Test(t)
}

func TestPromotionAdminToggleBackOn(t *testing.T) {
	t.Parallel()
	const id = "promotoggle0002"
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/promotions/{id}/toggle switches an inactive offer on",
		Method:         http.MethodPost,
		URL:            "/admin/promotions/" + id + "/toggle",
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		makePromotionTB(tb, app, id, "Oferta", "https://a.example", false, "")
		s.Headers = handlers.AuthHeaders(tb, handlers.MakeAdminUserTB(tb, app))
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		r, err := app.FindRecordById("promotions", id)
		require.NoError(tb, err)
		assert.True(tb, r.GetBool("active"))
	}
	handlers.ExpectRedirect(s, redirectTo("/admin/promotions"))
	s.Test(t)
}

func TestPromotionAdminDelete(t *testing.T) {
	t.Parallel()
	const id = "promodelete0001"
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/promotions/{id}/delete removes the offer and its image file",
		Method:         http.MethodPost,
		URL:            "/admin/promotions/" + id + "/delete",
		ExpectedStatus: 204,
	}
	var filePath string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		rec := makePromo(tb, app, id, "Oferta", true, "")
		f, err := filesystem.NewFileFromBytes(testPNGBytes(tb, 50, 50), "x.png")
		require.NoError(tb, err)
		rec.Set("image", f)
		require.NoError(tb, app.Save(rec))
		filePath = rec.BaseFilesPath() + "/" + rec.GetString("image")
		fsys, err := app.NewFilesystem()
		require.NoError(tb, err)
		defer func() { _ = fsys.Close() }()
		exists, err := fsys.Exists(filePath)
		require.NoError(tb, err)
		require.True(tb, exists, "image stored before the delete")
		s.Headers = handlers.AuthHeaders(tb, handlers.MakeAdminUserTB(tb, app))
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		_, err := app.FindRecordById("promotions", id)
		require.Error(tb, err, "record gone")
		// PocketBase deletes record files in the background.
		assert.Eventually(tb, func() bool {
			fsys, err := app.NewFilesystem()
			require.NoError(tb, err)
			defer func() { _ = fsys.Close() }()
			exists, err := fsys.Exists(filePath)
			require.NoError(tb, err)
			return !exists
		}, 5*time.Second, 50*time.Millisecond, "image file removed")
		assert.Zero(tb, notificationCount(tb, app), "no notification")
	}
	handlers.ExpectRedirect(s, redirectTo("/admin/promotions"))
	s.Test(t)
}

func TestPromotionAdminDeleteUnknown(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /admin/promotions/{id}/delete unknown",
		Method:          http.MethodPost,
		URL:             "/admin/promotions/nope00000000000/delete",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Promoción no encontrada"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		s.Headers = handlers.AuthHeaders(tb, handlers.MakeAdminUserTB(tb, app))
	}
	s.Test(t)
}

func TestPromotionAdminList(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		active      bool
		expiresOn   string
		want, avoid []string
	}{
		{"active", true, promoFuture, []string{"Activa", "Hasta el 31/12/2999"}, []string{"Inactiva", "Expirada"}},
		{"inactive", false, "", []string{"Inactiva"}, []string{"Activa", "Expirada"}},
		{"expired", true, promoPast, []string{"Expirada"}, []string{"Activa", "Inactiva"}},
		{"inactive and expired", false, promoPast, []string{"Inactiva", "Expirada"}, []string{"Activa"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := &tests.ApiScenario{
				TestAppFactory:     handlers.TestAppFactory,
				Name:               "GET /admin/promotions " + tc.name,
				Method:             http.MethodGet,
				URL:                "/admin/promotions",
				ExpectedStatus:     200,
				ExpectedContent:    append([]string{"Promociones", "Oferta", "7 vistas", "3 clics", `href="https://a.example"`}, tc.want...),
				NotExpectedContent: tc.avoid,
			}
			s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				setupProductionRoutes(tb, app, e)
				rec := makePromo(tb, app, "promolist000001", "Oferta", tc.active, tc.expiresOn)
				rec.Set("views", 7)
				rec.Set("clicks", 3)
				require.NoError(tb, app.Save(rec))
				s.Headers = handlers.AuthHeaders(tb, handlers.MakeAdminUserTB(tb, app))
			}
			s.Test(t)
		})
	}
}

func TestPromotionAdminListEmpty(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /admin/promotions empty",
		Method:          http.MethodGet,
		URL:             "/admin/promotions",
		ExpectedStatus:  200,
		ExpectedContent: []string{"No hay promociones registradas"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		s.Headers = handlers.AuthHeaders(tb, handlers.MakeAdminUserTB(tb, app))
	}
	s.Test(t)
}

func TestPromotionAdminEditForm(t *testing.T) {
	t.Parallel()
	const id = "promoform000001"
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "GET /admin/promotions/{id}/edit renders the filled form",
		Method:         http.MethodGet,
		URL:            "/admin/promotions/" + id + "/edit",
		ExpectedStatus: 200,
		ExpectedContent: []string{
			`hx-post="/admin/promotions/` + id + `"`, `value="Oferta"`, `value="https://a.example"`,
			`value="2999-12-31"`, "Quitar imagen",
		},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		rec := makePromo(tb, app, id, "Oferta", true, promoFuture)
		f, err := filesystem.NewFileFromBytes(testPNGBytes(tb, 50, 50), "x.png")
		require.NoError(tb, err)
		rec.Set("image", f)
		require.NoError(tb, app.Save(rec))
		s.Headers = handlers.AuthHeaders(tb, handlers.MakeAdminUserTB(tb, app))
	}
	s.Test(t)
}

func TestPromotionAdminEditFormUnknown(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /admin/promotions/{id}/edit unknown",
		Method:          http.MethodGet,
		URL:             "/admin/promotions/nope00000000000/edit",
		ExpectedStatus:  404,
		ExpectedContent: []string{"The requested resource wasn't found."},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		s.Headers = handlers.AuthHeaders(tb, handlers.MakeAdminUserTB(tb, app))
	}
	s.Test(t)
}

// A player is redirected home from every admin promotion route and nothing changes.
func TestPromotionAdminRoutesRefusePlayers(t *testing.T) {
	t.Parallel()
	const id = "promorefuse0001"
	routes := []struct{ method, path string }{
		{http.MethodGet, "/admin/promotions"},
		{http.MethodGet, "/admin/promotions/" + id + "/edit"},
		{http.MethodPost, "/admin/promotions"},
		{http.MethodPost, "/admin/promotions/" + id},
		{http.MethodPost, "/admin/promotions/" + id + "/toggle"},
		{http.MethodPost, "/admin/promotions/" + id + "/delete"},
	}
	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			t.Parallel()
			s := &tests.ApiScenario{
				TestAppFactory: handlers.TestAppFactory,
				Name:           rt.method + " " + rt.path + " as a player",
				Method:         rt.method,
				URL:            rt.path,
				ExpectedStatus: 302,
			}
			s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				setupProductionRoutes(tb, app, e)
				makePromotionTB(tb, app, id, "Oferta", "https://a.example", true, "")
				if rt.method == http.MethodPost {
					body, ct := promoForm{fields: map[string]string{"title": "Hack", "url": "https://hack.example"}}.body(tb)
					s.Body = body
					hdrs := handlers.AuthHeaders(tb, handlers.MakeUserTB(tb, app, "Jugador", ""))
					hdrs["Content-Type"] = ct
					s.Headers = hdrs
				} else {
					s.Headers = handlers.AuthHeaders(tb, handlers.MakeUserTB(tb, app, "Jugador", ""))
				}
			}
			s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
				assert.Equal(tb, "/", res.Header.Get("Location"))
				recs, err := app.FindAllRecords("promotions")
				require.NoError(tb, err)
				require.Len(tb, recs, 1, "no offer created or deleted")
				assert.Equal(tb, "Oferta", recs[0].GetString("title"))
				assert.True(tb, recs[0].GetBool("active"))
			}
			s.Test(t)
		})
	}
}
