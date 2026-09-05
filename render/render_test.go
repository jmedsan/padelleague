package render

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "padelleague/migrations"
)

// testApp is a package-wide throwaway PocketBase test app used to satisfy
// league.FooterContext's app.FindRecordsByFilter call from Page/Partial.
var testApp *tests.TestApp

func TestMain(m *testing.M) {
	app, err := tests.NewTestApp()
	if err != nil {
		panic(err)
	}
	testApp = app
	code := m.Run()
	testApp.Cleanup()
	os.Exit(code)
}

func makeEvent(auth *core.Record) (*core.RequestEvent, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	e := &core.RequestEvent{
		Auth: auth,
	}
	e.App = testApp
	e.Response = &router.ResponseWriter{ResponseWriter: rec}
	e.Request = req
	return e, rec
}

func makeAdmin() *core.Record {
	col := core.NewAuthCollection("users")
	r := core.NewRecord(col)
	r.Set("roles", []string{"admin"})
	r.Set("display_name", "Admin User")
	r.SetVerified(true)
	return r
}

func makePlayer() *core.Record {
	col := core.NewAuthCollection("users")
	r := core.NewRecord(col)
	r.Set("roles", []string{"player"})
	r.Set("display_name", "Test Player")
	r.SetVerified(false)
	return r
}

var goodFS = fstest.MapFS{
	"views/layout.html": &fstest.MapFile{
		Data: []byte(`<html><body>{{block "content" .}}{{end}}</body></html>`),
	},
	"views/page.html": &fstest.MapFile{
		Data: []byte(`{{define "content"}}<h1>{{.Title}}</h1>{{end}}`),
	},
	"views/partial.html": &fstest.MapFile{
		Data: []byte(`<span>{{.Label}}</span>`),
	},
	"views/error.html": &fstest.MapFile{
		Data: []byte(`{{define "content"}}<p>{{.ErrorMessage}}</p>{{end}}`),
	},
}

var brokenErrorFS = fstest.MapFS{
	"views/layout.html": &fstest.MapFile{
		Data: []byte(`<html><body>{{block "content" .}}{{end}}</body></html>`),
	},
	"views/error.html": &fstest.MapFile{
		Data: []byte(`{{define "content"}}{{template "nonexistent"}}{{end}}`),
	},
}

func body(rec *httptest.ResponseRecorder) string {
	b, _ := io.ReadAll(rec.Result().Body)
	return string(b)
}

func TestPage_RendersWithLayout(t *testing.T) {
	t.Parallel()
	r := New(goodFS, "vapid-key-123", true)
	e, rec := makeEvent(nil)

	err := r.Page(e, "page.html", map[string]any{"Title": "Hola"})

	require.NoError(t, err)
	b := body(rec)
	assert.Contains(t, b, "<html>", "layout wrapper present")
	assert.Contains(t, b, "<h1>Hola</h1>", "page content rendered")
	assert.Equal(t, http.StatusOK, rec.Code)
}

var competitionURLFS = fstest.MapFS{
	"views/layout.html": &fstest.MapFile{
		Data: []byte(`<html><body>{{block "content" .}}{{end}}</body></html>`),
	},
	"views/comp.html": &fstest.MapFile{
		Data: []byte(`{{define "content"}}<a href="{{competitionURL .ID}}">go</a>{{end}}`),
	},
}

func TestCompetitionURL_Func(t *testing.T) {
	t.Parallel()
	r := New(competitionURLFS, "", true)
	e, rec := makeEvent(nil)

	err := r.Page(e, "comp.html", map[string]any{"ID": "abc123"})

	require.NoError(t, err)
	assert.Contains(t, body(rec), `href="/competition/abc123"`)
}

func TestPage_NilData(t *testing.T) {
	t.Parallel()
	r := New(goodFS, "vapid-key-123", true)
	e, rec := makeEvent(nil)

	err := r.Page(e, "page.html", nil)

	require.NoError(t, err)
	assert.Contains(t, body(rec), "<html>")
}

var flashFS = fstest.MapFS{
	"views/layout.html": &fstest.MapFile{
		Data: []byte(`<html><body>{{if .Flash}}<div id="flash-msg">{{.Flash}}</div>{{end}}{{block "content" .}}{{end}}</body></html>`),
	},
	"views/page.html": &fstest.MapFile{
		Data: []byte(`{{define "content"}}<h1>ok</h1>{{end}}`),
	},
}

func TestPage_Flash_ReadsAndClearsCookie(t *testing.T) {
	t.Parallel()
	r := New(flashFS, "", true)
	e, rec := makeEvent(nil)
	e.Request.AddCookie(&http.Cookie{Name: "flash_msg", Value: url.QueryEscape("Patrocinador creado")})

	err := r.Page(e, "page.html", map[string]any{})

	require.NoError(t, err)
	assert.Contains(t, body(rec), `<div id="flash-msg">Patrocinador creado</div>`)

	setCookie := rec.Result().Header.Get("Set-Cookie")
	assert.Contains(t, setCookie, "flash_msg=;", "response must clear the flash cookie")
}

func TestPage_Flash_NoCookieMeansNoFlash(t *testing.T) {
	t.Parallel()
	r := New(flashFS, "", true)
	e, rec := makeEvent(nil)

	err := r.Page(e, "page.html", map[string]any{})

	require.NoError(t, err)
	assert.NotContains(t, body(rec), "flash-msg")
}

func TestPartial_RendersWithoutLayout(t *testing.T) {
	t.Parallel()
	r := New(goodFS, "vapid-key-123", true)
	e, rec := makeEvent(nil)

	err := r.Partial(e, "partial.html", map[string]any{"Label": "Parcial"})

	require.NoError(t, err)
	b := body(rec)
	assert.Contains(t, b, "<span>Parcial</span>", "partial content rendered")
	assert.NotContains(t, b, "<html>", "no layout wrapper")
}

func TestPartial_NilData(t *testing.T) {
	t.Parallel()
	r := New(goodFS, "vapid-key-123", true)
	e, rec := makeEvent(nil)

	err := r.Partial(e, "partial.html", nil)

	require.NoError(t, err)
	assert.NotContains(t, body(rec), "<html>")
}

func TestErrorPage_RendersWithLayout(t *testing.T) {
	t.Parallel()
	r := New(goodFS, "vapid-key-123", true)
	e, rec := makeEvent(nil)

	err := r.ErrorPage(e, http.StatusNotFound, "No encontrado")

	require.NoError(t, err)
	b := body(rec)
	assert.Contains(t, b, "<html>", "layout wrapper")
	assert.Contains(t, b, "<p>No encontrado</p>", "error message")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestErrorPage_FallbackOnBrokenTemplate(t *testing.T) {
	t.Parallel()
	r := New(brokenErrorFS, "vapid-key-123", true)
	e, rec := makeEvent(nil)

	err := r.ErrorPage(e, http.StatusInternalServerError, "Error grave")

	require.NoError(t, err)
	b := body(rec)
	assert.Equal(t, "Error grave", strings.TrimSpace(b), "fallback emits raw message")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestWithAuth_AdminUser(t *testing.T) {
	t.Parallel()
	r := New(goodFS, "vapid-key-123", true)
	admin := makeAdmin()
	e, rec := makeEvent(admin)

	err := r.Page(e, "page.html", map[string]any{"Title": "Admin"})

	require.NoError(t, err)
	b := body(rec)
	assert.Contains(t, b, "<h1>Admin</h1>")
	_ = rec
}

func TestWithAuth_PlayerUser(t *testing.T) {
	t.Parallel()
	r := New(goodFS, "vapid-key-123", true)
	player := makePlayer()
	e, _ := makeEvent(player)

	data := map[string]any{"Title": "Player"}
	r.withAuth(e, data)

	assert.Equal(t, false, data["IsAdmin"], "player is not admin")
	assert.Equal(t, "Test Player", data["DisplayName"])
	assert.Equal(t, false, data["Verified"], "player not verified")
	assert.Equal(t, player.Id, data["AuthID"])
	assert.Equal(t, "vapid-key-123", data["VAPIDPublicKey"])
}

func TestWithAuth_AdminRole(t *testing.T) {
	t.Parallel()
	r := New(goodFS, "vapid-key-123", true)
	admin := makeAdmin()
	e, _ := makeEvent(admin)

	data := map[string]any{}
	r.withAuth(e, data)

	assert.Equal(t, true, data["IsAdmin"], "admin role detected")
	assert.Equal(t, "Admin User", data["DisplayName"])
	assert.Equal(t, true, data["Verified"])
}

func TestWithAuth_NoAuth(t *testing.T) {
	t.Parallel()
	r := New(goodFS, "vapid-key-123", true)
	e, _ := makeEvent(nil)

	data := map[string]any{}
	r.withAuth(e, data)

	assert.Equal(t, "vapid-key-123", data["VAPIDPublicKey"])
	assert.Nil(t, data["IsAdmin"], "no auth → no IsAdmin key")
	assert.Nil(t, data["DisplayName"], "no auth → no DisplayName key")
}

func TestWithAuth_DoesNotOverrideExisting(t *testing.T) {
	t.Parallel()
	r := New(goodFS, "vapid-key-123", true)
	admin := makeAdmin()
	e, _ := makeEvent(admin)

	data := map[string]any{
		"DisplayName": "Custom Name",
		"IsAdmin":     false,
		"Verified":    false,
		"AuthID":      "custom-id",
	}
	r.withAuth(e, data)

	assert.Equal(t, "Custom Name", data["DisplayName"], "caller-provided value preserved")
	assert.Equal(t, false, data["IsAdmin"], "caller-provided value preserved")
	assert.Equal(t, false, data["Verified"], "caller-provided value preserved")
	assert.Equal(t, "custom-id", data["AuthID"], "caller-provided value preserved")
}

func TestFmtDate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, input, want string
	}{
		{"empty", "", ""},
		{"PB datetime midnight", "2026-08-28 00:00:00.000Z", "28/08/2026"},
		{"PB datetime with time", "2026-08-28 19:30:00.000Z", "28/08/2026 21:30"},
		{"date only", "2026-08-28", "28/08/2026"},
		{"RFC3339", "2026-08-28T19:30:00Z", "28/08/2026 21:30"},
		{"date + time no seconds (wall-clock)", "2026-10-15 19:30", "15/10/2026 19:30"},
		{"unparseable", "not-a-date", "not-a-date"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, FmtDate(tt.input))
		})
	}
}

func TestFmtDate_MadridTimezone(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, input, want string
	}{
		{"summer UTC+2", "2026-07-15 22:00:00.000Z", "16/07/2026 00:00"},
		{"winter UTC+1", "2026-12-15 23:30:00.000Z", "16/12/2026 00:30"},
		{"summer date rollover", "2026-08-01 23:00:00.000Z", "02/08/2026 01:00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FmtDate(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFmtTime_MadridTimezone(t *testing.T) {
	t.Parallel()
	utc := time.Date(2026, 7, 15, 22, 0, 0, 0, time.UTC)
	got := FmtTime(utc)
	assert.Equal(t, "16/07/2026", got, "22:00 UTC = 00:00+1d CEST, midnight omits time")
}

func TestFmtShortTime_MadridTimezone(t *testing.T) {
	t.Parallel()
	utc := time.Date(2026, 12, 15, 23, 30, 0, 0, time.UTC)
	got := FmtShortTime(utc)
	assert.Equal(t, "16/12 00:30", got, "23:30 UTC = 00:30+1d CET")
}

func TestScoreWinner(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, score, want string
	}{
		{"pair1 wins in two sets", "6-3 6-4", "Pareja A"},
		{"pair2 wins in two sets", "3-6 4-6", "Pareja B"},
		{"pair2 wins in three sets", "6-4 4-6 5-7", "Pareja B"},
		{"empty score", "", ""},
		{"incomplete: only one set", "6-3", ""},
		{"invalid: tied set", "6-6 6-4", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, scoreWinner(tt.score, "Pareja A", "Pareja B"))
		})
	}
}

func TestRelDate(t *testing.T) {
	t.Parallel()
	now := time.Now().In(madrid)
	dayStr := func(offset int) string {
		return now.AddDate(0, 0, offset).Format("2006-01-02")
	}
	tests := []struct {
		name, input, want string
	}{
		{"empty", "", ""},
		{"unparseable", "not-a-date", "not-a-date"},
		{"today", dayStr(0), "hoy"},
		{"tomorrow", dayStr(1), "mañana"},
		{"yesterday", dayStr(-1), "ayer"},
		{"in 2 days", dayStr(2), "en 2 días"},
		{"in 7 days", dayStr(7), "en 7 días"},
		{"8 days out falls back to absolute", dayStr(8), FmtDate(dayStr(8))},
		{"2 days ago", dayStr(-2), "hace 2 días"},
		{"7 days ago", dayStr(-7), "hace 7 días"},
		{"8 days ago falls back to absolute", dayStr(-8), FmtDate(dayStr(-8))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, RelDate(tt.input))
		})
	}
}

func TestRelDate_TodayYesterdayIncludeTime(t *testing.T) {
	t.Parallel()
	now := time.Now().In(madrid)
	dayTimeStr := func(offset int) string {
		d := now.AddDate(0, 0, offset)
		return time.Date(d.Year(), d.Month(), d.Day(), 14, 39, 0, 0, madrid).Format("2006-01-02 15:04")
	}
	tests := []struct {
		name, input, want string
	}{
		{"today with time", dayTimeStr(0), "hoy 14:39"},
		{"yesterday with time", dayTimeStr(-1), "ayer 14:39"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, RelDate(tt.input))
		})
	}
}

func TestRelDate_TomorrowNeverIncludesTime(t *testing.T) {
	t.Parallel()
	now := time.Now().In(madrid)
	tomorrow := now.AddDate(0, 0, 1)
	input := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 14, 39, 0, 0, madrid).Format("2006-01-02 15:04")
	assert.Equal(t, "mañana", RelDate(input))
}

func TestRelDate_MidnightTodayHasNoTime(t *testing.T) {
	t.Parallel()
	now := time.Now().In(madrid)
	assert.Equal(t, "hoy", RelDate(now.Format("2006-01-02")))
}

func TestInitials(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, want string
	}{
		{"Jugador 1", "J1"},
		{"Luis García", "LG"},
		{"Ana", "A"},
		{"Ñoño García", "ÑG"},
		{"Álvaro", "Á"},
		{"", "?"},
		{"  ", "?"},
		{"María del Carmen López", "ML"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Initials(tc.name))
		})
	}
}
