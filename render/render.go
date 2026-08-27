// Package render provides HTML template rendering helpers.
package render

import (
	"io/fs"
	"log/slog"
	"net/http"
	"slices"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/template"
)

// Renderer renders HTML templates with auth context injected.
type Renderer struct {
	registry       *template.Registry
	viewsFS        fs.FS
	vapidPublicKey string
}

// New creates a Renderer backed by the given views filesystem.
func New(viewsFS fs.FS, vapidPublicKey string) *Renderer {
	reg := template.NewRegistry()
	reg.AddFuncs(map[string]any{
		"contains": func(slice []string, item string) bool {
			return slices.Contains(slice, item)
		},
	})
	return &Renderer{
		registry:       reg,
		viewsFS:        viewsFS,
		vapidPublicKey: vapidPublicKey,
	}
}

func (r *Renderer) withAuth(e *core.RequestEvent, data map[string]any) {
	data["VAPIDPublicKey"] = r.vapidPublicKey
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
		return err
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
