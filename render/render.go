// Package render provides HTML template rendering helpers.
package render

import (
	"io/fs"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/template"

	"padelleague/league"
)

// Renderer renders HTML templates with auth context injected.
type Renderer struct {
	registry       *template.Registry
	viewsFS        fs.FS
	vapidPublicKey string
	appDevTools    bool
}

// New creates a Renderer backed by the given views filesystem.
func New(viewsFS fs.FS, vapidPublicKey string, appDevTools bool) *Renderer {
	reg := template.NewRegistry()
	reg.AddFuncs(map[string]any{
		"contains": func(slice []string, item string) bool {
			return slices.Contains(slice, item)
		},
		"entityURL": league.EntityURL,
		"avatarURL": league.AvatarURL,
		"fmtDate":   FmtDate,
		"elink": func(id, name string) map[string]string {
			return map[string]string{"ID": id, "Name": name}
		},
		"hasKey": func(m map[string]struct{}, key string) bool {
			_, ok := m[key]
			return ok
		},
		"sub":  func(a, b int) int { return a - b },
		"mulf": func(a float64, b int) float64 { return a * float64(b) },
		"dict": func(pairs ...any) map[string]any {
			m := make(map[string]any, len(pairs)/2)
			for i := 0; i+1 < len(pairs); i += 2 {
				if k, ok := pairs[i].(string); ok {
					m[k] = pairs[i+1]
				}
			}
			return m
		},
		"ternary": func(cond bool, ifTrue, ifFalse any) any {
			if cond {
				return ifTrue
			}
			return ifFalse
		},
		"scoreWinner": scoreWinner,
	})
	return &Renderer{
		registry:       reg,
		viewsFS:        viewsFS,
		vapidPublicKey: vapidPublicKey,
		appDevTools:    appDevTools,
	}
}

func (r *Renderer) withAuth(e *core.RequestEvent, data map[string]any) {
	data["VAPIDPublicKey"] = r.vapidPublicKey
	data["AppDevTools"] = r.appDevTools
	if e.Auth == nil {
		return
	}
	if _, ok := data["DisplayName"]; !ok {
		data["DisplayName"] = e.Auth.GetString("display_name")
	}
	if _, ok := data["IsAdmin"]; !ok {
		setViewContext(e, data)
	}
	if _, ok := data["Verified"]; !ok {
		data["Verified"] = e.Auth.Verified()
	}
	if _, ok := data["AuthID"]; !ok {
		data["AuthID"] = e.Auth.Id
	}
}

// AdminView reports whether the request should render admin controls: the user
// is an admin AND is not currently viewing as a player (the view_as cookie).
func AdminView(e *core.RequestEvent) bool {
	roles := e.Auth.GetStringSlice("roles")
	if !slices.Contains(roles, "admin") {
		return false
	}
	if c, err := e.Request.Cookie("view_as"); err == nil {
		return c.Value != "player"
	}
	return true
}

// setViewContext derives the admin/player view flags from the user's roles and
// the view_as cookie. Admins default to the admin view; either role alone has
// no choice, so HasBothRoles gates the switcher UI.
func setViewContext(e *core.RequestEvent, data map[string]any) {
	roles := e.Auth.GetStringSlice("roles")
	isAdmin := slices.Contains(roles, "admin")
	viewAs := "player"
	if isAdmin {
		viewAs = "admin"
	}
	if c, err := e.Request.Cookie("view_as"); err == nil && (c.Value == "admin" || c.Value == "player") {
		viewAs = c.Value
	}
	data["IsAdmin"] = isAdmin
	data["HasBothRoles"] = isAdmin && slices.Contains(roles, "player")
	data["ViewAs"] = viewAs
	data["AdminView"] = AdminView(e)
}

func (r *Renderer) partialFiles() []string {
	entries, _ := fs.Glob(r.viewsFS, "views/partials/*.html")
	return entries
}

// Page renders a full page within the site layout.
func (r *Renderer) Page(e *core.RequestEvent, page string, data map[string]any) error {
	if data == nil {
		data = map[string]any{}
	}
	r.withAuth(e, data)
	files := append([]string{"views/layout.html"}, r.partialFiles()...)
	files = append(files, "views/"+page)
	html, err := r.registry.LoadFS(r.viewsFS, files...).Render(data)
	if err != nil {
		slog.Error("render page", "page", page, "err", err)
		return e.HTML(http.StatusInternalServerError, "Error al cargar la página. Inténtalo de nuevo.")
	}
	return e.HTML(http.StatusOK, html)
}

// ErrorPage renders an error page with the given status code and message.
func (r *Renderer) ErrorPage(e *core.RequestEvent, statusCode int, message string) error {
	data := map[string]any{"ErrorMessage": message}
	r.withAuth(e, data)
	files := append([]string{"views/layout.html"}, r.partialFiles()...)
	files = append(files, "views/error.html")
	html, err := r.registry.LoadFS(r.viewsFS, files...).Render(data)
	if err != nil {
		slog.Error("render error page", "err", err)
		return e.HTML(statusCode, message)
	}
	return e.HTML(statusCode, html)
}

var dateLayouts = []string{
	"2006-01-02 15:04:05.000Z",
	time.RFC3339,
	"2006-01-02 15:04",
	"2006-01-02",
}

var madrid = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		return time.UTC
	}
	return loc
}()

// FmtDate parses a date string and returns it in Spanish DD/MM/YYYY format.
// Timestamps with an explicit UTC marker (Z or +00:00) are converted to
// Europe/Madrid; wall-clock values from datetime-local inputs are rendered
// as-is.
func FmtDate(raw string) string {
	if raw == "" {
		return ""
	}
	for _, layout := range dateLayouts {
		t, err := time.Parse(layout, raw)
		if err != nil {
			continue
		}
		if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 {
			return t.Format("02/01/2006")
		}
		if hasExplicitUTC(raw) {
			t = t.In(madrid)
		}
		return t.Format("02/01/2006 15:04")
	}
	return raw
}

func hasExplicitUTC(raw string) bool {
	return strings.HasSuffix(raw, "Z") ||
		strings.Contains(raw, "+00:00") ||
		strings.Contains(raw, "+0000")
}

// scoreWinner returns the winning pair's name for a complete, valid padel
// score, or "" when the score is empty, incomplete, or invalid.
func scoreWinner(score, pair1Name, pair2Name string) string {
	s, err := league.ParseScore(score)
	if err != nil {
		return ""
	}
	if s.Sets1 > s.Sets2 {
		return pair1Name
	}
	return pair2Name
}

// FmtTime formats a time.Time in Europe/Madrid as DD/MM/YYYY, appending
// HH:MM when the local time is not midnight.
func FmtTime(t time.Time) string {
	t = t.In(madrid)
	if t.Hour() == 0 && t.Minute() == 0 {
		return t.Format("02/01/2006")
	}
	return t.Format("02/01/2006 15:04")
}

// FmtShortTime formats a time.Time in Europe/Madrid as DD/MM HH:MM.
func FmtShortTime(t time.Time) string {
	return t.In(madrid).Format("02/01 15:04")
}

// Partial renders an HTML fragment without the site layout.
func (r *Renderer) Partial(e *core.RequestEvent, page string, data map[string]any) error {
	if data == nil {
		data = map[string]any{}
	}
	r.withAuth(e, data)
	files := append([]string{"views/" + page}, r.partialFiles()...)
	html, err := r.registry.LoadFS(r.viewsFS, files...).Render(data)
	if err != nil {
		return err
	}
	return e.HTML(http.StatusOK, html)
}
