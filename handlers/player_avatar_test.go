package handlers

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tinyWebP is a small (442-byte) lossless WebP fixture, lifted verbatim from
// golang.org/x/image's own testdata, used to prove the webp decoder is
// registered and avatar uploads from Android (which sends image/webp) work.
const tinyWebPBase64 = "UklGRrIBAABXRUJQVlA4TKUBAAAvSsAYAA8w//M///MfeJAkbXvaSG7m8Q3GfYSBJekwQztm/IcZlgwnmWImn2BK7aFmBtnVir6q//8VOkFE/xm4baTIu8c48ArEo6+B3zFKYln3pqClSCKX0begFTAXFOLXHSyF8cCNcZEG4OywuA4KVVfJCiArU7GAgJI8+lJP/OKMT/fBAjevg1cYB7YVkFuWga2lyPi5I0HFy5YTpWIHg0RZpkniRVW9odHAKOwosWuOGdxIyn2OvaCDvhg/we6TwadPBPbqBV58MsLmMJ8yZnOWk8SRz4N+QoyPL+MnamzMvcE1rHNEr91F9GKZPVUcS9w7PhhH36suB9qPeYb/oLk6cuTiJ0wOK3m5h1cKjW6EVZCYMK7dxcKCBdgP9HkKr9gkAO2P8GKZGWVdIAatQa+1IDpt6qyorVwdy01xdW8Jkfk6xjEXmVQQ+HQdFr6OKhIN34dXWq0+0qr6EJSCeeVLH9+gvGTLyqM65PQ44ihzlTXxQKjKbAvshXgir7Lil9w4L2bvMycmjQcqXaMCO6BlY28i+FOLzbfI1vEqxAhotocAAA=="

func tinyWebPBytes(t testing.TB) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(tinyWebPBase64)
	require.NoError(t, err)
	return b
}

func testPNGBytes(t testing.TB, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 50, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

// testPNGBytesWithAlpha builds a PNG with a transparent half and an opaque
// red half, used to verify JPEG re-encoding flattens transparency to white
// instead of leaving black.
func testPNGBytesWithAlpha(t testing.TB, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x < w/2 {
				img.Set(x, y, color.RGBA{}) // fully transparent
			} else {
				img.Set(x, y, color.RGBA{R: 255, A: 255})
			}
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

// buildExifAPP1 builds a minimal EXIF APP1 segment payload (after the
// marker/length) containing a single Orientation tag (0x0112).
func buildExifAPP1(orientation uint16) []byte {
	var buf bytes.Buffer
	buf.WriteString("Exif\x00\x00")
	_ = binary.Write(&buf, binary.LittleEndian, [2]byte{'I', 'I'})
	_ = binary.Write(&buf, binary.LittleEndian, uint16(42))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(8))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(0x0112))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(3))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(1))
	_ = binary.Write(&buf, binary.LittleEndian, orientation)
	_ = binary.Write(&buf, binary.LittleEndian, uint16(0))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(0))
	return buf.Bytes()
}

// testJPEGWithOrientation builds a w x h JPEG whose left half is red and
// right half is blue, with an EXIF Orientation tag set to the given value.
// Asymmetric coloring lets a test assert the rotation actually happened.
func testJPEGWithOrientation(t testing.TB, w, h int, orientation uint16) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x < w/2 {
				img.Set(x, y, color.RGBA{R: 255, A: 255})
			} else {
				img.Set(x, y, color.RGBA{B: 255, A: 255})
			}
		}
	}
	var body bytes.Buffer
	require.NoError(t, jpeg.Encode(&body, img, nil))
	raw := body.Bytes()

	app1Data := buildExifAPP1(orientation)
	var app1 bytes.Buffer
	app1.WriteByte(0xFF)
	app1.WriteByte(0xE1)
	require.NoError(t, binary.Write(&app1, binary.BigEndian, uint16(len(app1Data)+2)))
	app1.Write(app1Data)

	var out bytes.Buffer
	out.Write(raw[0:2])
	out.Write(app1.Bytes())
	out.Write(raw[2:])
	return out.Bytes()
}

func multipartAvatarBody(t testing.TB, imgBytes []byte) (*bytes.Buffer, string) {
	t.Helper()
	return multipartAvatarBodyWithType(t, imgBytes, "photo.png", "image/png")
}

func multipartAvatarBodyWithType(t testing.TB, imgBytes []byte, filename, contentType string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="avatar"; filename="`+filename+`"`)
	header.Set("Content-Type", contentType)
	part, err := w.CreatePart(header)
	require.NoError(t, err)
	_, err = part.Write(imgBytes)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return &buf, w.FormDataContentType()
}

func TestPlayerAvatarUpload_ValidImage(t *testing.T) {
	t.Parallel()
	var userID string
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /player/{id}/avatar with a valid image sets the avatar",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"avatar-file-input"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Self Uploader", "")
		userID = user.Id
		s.URL = "/player/" + user.Id + "/avatar"

		body, contentType := multipartAvatarBody(tb, testPNGBytes(tb, 800, 600))
		s.Body = body
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = contentType
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		user, err := app.FindRecordById("users", userID)
		require.NoError(tb, err)
		assert.NotEmpty(tb, user.GetString("avatar"))
	}
	s.Test(t)
}

func TestPlayerAvatarUpload_WrongUserRejected(t *testing.T) {
	t.Parallel()
	var targetID string
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /player/{id}/avatar as a different user is rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"No puedes cambiar la foto de otro jugador"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		target := makeUserTB(tb, app, "Target Player", "")
		attacker := makeUserTB(tb, app, "Attacker Player", "")
		targetID = target.Id
		s.URL = "/player/" + target.Id + "/avatar"

		body, contentType := multipartAvatarBody(tb, testPNGBytes(tb, 100, 100))
		s.Body = body
		hdrs := authHeaders(tb, attacker)
		hdrs["Content-Type"] = contentType
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		user, err := app.FindRecordById("users", targetID)
		require.NoError(tb, err)
		assert.Empty(tb, user.GetString("avatar"))
	}
	s.Test(t)
}

func TestPlayerAvatarUpload_NonImageRejected(t *testing.T) {
	t.Parallel()
	var userID string
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /player/{id}/avatar with a non-image file is rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"El archivo debe ser una imagen"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Self Uploader", "")
		userID = user.Id
		s.URL = "/player/" + user.Id + "/avatar"

		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		part, err := w.CreateFormFile("avatar", "not-an-image.txt")
		require.NoError(tb, err)
		_, err = part.Write([]byte("plain text content"))
		require.NoError(tb, err)
		require.NoError(tb, w.Close())

		s.Body = &buf
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = w.FormDataContentType()
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		user, err := app.FindRecordById("users", userID)
		require.NoError(tb, err)
		assert.Empty(tb, user.GetString("avatar"))
	}
	s.Test(t)
}

func TestPlayerAvatarUpload_OversizedFileRejected(t *testing.T) {
	t.Parallel()
	var userID string
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /player/{id}/avatar with a file over 5MB is rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"La imagen no puede superar los 5 MB"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Self Uploader", "")
		userID = user.Id
		s.URL = "/player/" + user.Id + "/avatar"

		oversized := make([]byte, avatarMaxUploadSize+1)
		body, contentType := multipartAvatarBody(tb, oversized)
		s.Body = body
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = contentType
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		user, err := app.FindRecordById("users", userID)
		require.NoError(tb, err)
		assert.Empty(tb, user.GetString("avatar"))
	}
	s.Test(t)
}

// savedAvatarImage reads back and decodes the avatar file saved on the user
// record, for pixel-level assertions on the stored JPEG.
func savedAvatarImage(tb testing.TB, app *tests.TestApp, userID string) image.Image {
	tb.Helper()
	user, err := app.FindRecordById("users", userID)
	require.NoError(tb, err)
	filename := user.GetString("avatar")
	require.NotEmpty(tb, filename)

	fsys, err := app.NewFilesystem()
	require.NoError(tb, err)
	defer func() { _ = fsys.Close() }()

	r, err := fsys.GetReader(user.BaseFilesPath() + "/" + filename)
	require.NoError(tb, err)
	defer func() { _ = r.Close() }()

	img, _, err := image.Decode(r)
	require.NoError(tb, err)
	return img
}

func TestPlayerAvatarUpload_NonSquareImageIsCroppedToSquare(t *testing.T) {
	t.Parallel()
	var userID string
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /player/{id}/avatar with a wide image crops it to a square",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"avatar-file-input"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Wide Photo Uploader", "")
		userID = user.Id
		s.URL = "/player/" + user.Id + "/avatar"

		body, contentType := multipartAvatarBody(tb, testPNGBytes(tb, 800, 200))
		s.Body = body
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = contentType
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		img := savedAvatarImage(tb, app, userID)
		b := img.Bounds()
		assert.Equal(tb, b.Dx(), b.Dy(), "avatar should be cropped to a square")
	}
	s.Test(t)
}

func TestPlayerAvatarUpload_TransparencyFlattenedToWhite(t *testing.T) {
	t.Parallel()
	var userID string
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /player/{id}/avatar with transparency flattens it opaque",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"avatar-file-input"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Transparent Photo Uploader", "")
		userID = user.Id
		s.URL = "/player/" + user.Id + "/avatar"

		body, contentType := multipartAvatarBody(tb, testPNGBytesWithAlpha(tb, 100, 100))
		s.Body = body
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = contentType
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		img := savedAvatarImage(tb, app, userID)
		b := img.Bounds()
		r, g, bl, a := img.At(b.Min.X, b.Min.Y).RGBA()
		assert.Equal(tb, uint32(0xffff), a, "JPEG output must be fully opaque")
		assert.Greater(tb, r, uint32(0xe000), "previously-transparent area should be near-white, not black")
		assert.Greater(tb, g, uint32(0xe000))
		assert.Greater(tb, bl, uint32(0xe000))
	}
	s.Test(t)
}

func TestPlayerAvatarUpload_ExifOrientationCorrected(t *testing.T) {
	t.Parallel()
	var userID string
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /player/{id}/avatar with EXIF orientation 6 rotates the image upright",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"avatar-file-input"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Phone Photo Uploader", "")
		userID = user.Id
		s.URL = "/player/" + user.Id + "/avatar"

		// A landscape source (left=red, right=blue) tagged Orientation=6
		// (rotate 90 CW to display) must come out with red on top, blue on
		// bottom once corrected.
		jpegBytes := testJPEGWithOrientation(tb, 200, 100, 6)
		body, contentType := multipartAvatarBodyWithType(tb, jpegBytes, "phone.jpg", "image/jpeg")
		s.Body = body
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = contentType
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		img := savedAvatarImage(tb, app, userID)
		b := img.Bounds()
		topR, _, topB, _ := img.At(b.Min.X, b.Min.Y).RGBA()
		bottomR, _, bottomB, _ := img.At(b.Min.X, b.Max.Y-1).RGBA()
		assert.Greater(tb, topR, topB, "top of the corrected image should be the red half")
		assert.Greater(tb, bottomB, bottomR, "bottom of the corrected image should be the blue half")
	}
	s.Test(t)
}

func TestPlayerAvatarUpload_ImplausibleDimensionsRejected(t *testing.T) {
	t.Parallel()
	var userID string
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /player/{id}/avatar with an image wider than avatarMaxSourceDim is rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"La imagen es demasiado grande"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Huge Photo Uploader", "")
		userID = user.Id
		s.URL = "/player/" + user.Id + "/avatar"

		body, contentType := multipartAvatarBody(tb, testPNGBytes(tb, avatarMaxSourceDim+1, 10))
		s.Body = body
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = contentType
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		user, err := app.FindRecordById("users", userID)
		require.NoError(tb, err)
		assert.Empty(tb, user.GetString("avatar"))
	}
	s.Test(t)
}

func TestPlayerAvatarUpload_WebPImageAccepted(t *testing.T) {
	t.Parallel()
	var userID string
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /player/{id}/avatar with a WebP image (Android) is accepted",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"avatar-file-input"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Android Uploader", "")
		userID = user.Id
		s.URL = "/player/" + user.Id + "/avatar"

		body, contentType := multipartAvatarBodyWithType(tb, tinyWebPBytes(tb), "photo.webp", "image/webp")
		s.Body = body
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = contentType
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		user, err := app.FindRecordById("users", userID)
		require.NoError(tb, err)
		assert.NotEmpty(tb, user.GetString("avatar"))
	}
	s.Test(t)
}

func TestPlayerAvatarUpload_LargeImageIsResized(t *testing.T) {
	t.Parallel()
	var userID string
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /player/{id}/avatar with an oversized image resizes it down",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"avatar-file-input"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		user := makeUserTB(tb, app, "Big Photo Uploader", "")
		userID = user.Id
		s.URL = "/player/" + user.Id + "/avatar"

		body, contentType := multipartAvatarBody(tb, testPNGBytes(tb, 2000, 1500))
		s.Body = body
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = contentType
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		user, err := app.FindRecordById("users", userID)
		require.NoError(tb, err)
		filename := user.GetString("avatar")
		require.NotEmpty(tb, filename)

		fsys, err := app.NewFilesystem()
		require.NoError(tb, err)
		defer func() { _ = fsys.Close() }()

		key := user.BaseFilesPath() + "/" + filename
		r, err := fsys.GetReader(key)
		require.NoError(tb, err)
		defer func() { _ = r.Close() }()

		cfg, _, err := image.DecodeConfig(r)
		require.NoError(tb, err)
		assert.LessOrEqual(tb, cfg.Width, avatarMaxDim)
		assert.LessOrEqual(tb, cfg.Height, avatarMaxDim)
	}
	s.Test(t)
}
