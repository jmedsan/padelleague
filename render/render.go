// Package render provides HTML template rendering helpers.
package render

import (
	"io/fs"
	"log/slog"
	"net/http"

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
	return &Renderer{
		registry:       template.NewRegistry(),
		viewsFS:        viewsFS,
		vapidPublicKey: vapidPublicKey,
	}
}

func (r *Renderer) withAuth(e *core.RequestEvent, data map[string]any) {
	data["VAPIDPublicKey"] = r.vapidPublicKey
	if e.Auth != nil {
		if _, ok := data["DisplayName"]; !ok {
			data["DisplayName"] = e.Auth.GetString("display_name")
		}
		if _, ok := data["IsAdmin"]; !ok {
			data["IsAdmin"] = e.Auth.GetString("role") == "admin"
		}
		if _, ok := data["Verified"]; !ok {
			data["Verified"] = e.Auth.Verified()
		}
		if _, ok := data["AuthID"]; !ok {
			data["AuthID"] = e.Auth.Id
		}
	}
}

// Page renders a full page within the site layout.
func (r *Renderer) Page(e *core.RequestEvent, page string, data map[string]any) error {
	if data == nil {
		data = map[string]any{}
	}
	r.withAuth(e, data)
	html, err := r.registry.LoadFS(r.viewsFS, "views/layout.html", "views/"+page).Render(data)
	if err != nil {
		return err
	}
	return e.HTML(http.StatusOK, html)
}

// ErrorPage renders an error page with the given status code and message.
func (r *Renderer) ErrorPage(e *core.RequestEvent, statusCode int, message string) error {
	data := map[string]any{"ErrorMessage": message}
	r.withAuth(e, data)
	html, err := r.registry.LoadFS(r.viewsFS, "views/layout.html", "views/error.html").Render(data)
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
	html, err := r.registry.LoadFS(r.viewsFS, "views/"+page).Render(data)
	if err != nil {
		return err
	}
	return e.HTML(http.StatusOK, html)
}
