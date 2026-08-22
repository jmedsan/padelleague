package render

import (
	"io/fs"
	"net/http"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/template"
)

type Renderer struct {
	registry       *template.Registry
	viewsFS        fs.FS
	vapidPublicKey string
}

func New(viewsFS fs.FS, vapidPublicKey string) *Renderer {
	return &Renderer{
		registry:       template.NewRegistry(),
		viewsFS:        viewsFS,
		vapidPublicKey: vapidPublicKey,
	}
}

func (r *Renderer) Page(e *core.RequestEvent, page string, data map[string]any) error {
	if data == nil {
		data = map[string]any{}
	}
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
	html, err := r.registry.LoadFS(r.viewsFS, "views/layout.html", "views/"+page).Render(data)
	if err != nil {
		return err
	}
	return e.HTML(http.StatusOK, html)
}

func (r *Renderer) ErrorPage(e *core.RequestEvent, statusCode int, message string) error {
	data := map[string]any{"ErrorMessage": message}
	data["VAPIDPublicKey"] = r.vapidPublicKey
	if e.Auth != nil {
		data["DisplayName"] = e.Auth.GetString("display_name")
		data["IsAdmin"] = e.Auth.GetString("role") == "admin"
		data["Verified"] = e.Auth.Verified()
		data["AuthID"] = e.Auth.Id
	}
	html, err := r.registry.LoadFS(r.viewsFS, "views/layout.html", "views/error.html").Render(data)
	if err != nil {
		return e.HTML(statusCode, message)
	}
	return e.HTML(statusCode, html)
}

func (r *Renderer) Partial(e *core.RequestEvent, page string, data map[string]any) error {
	if data == nil {
		data = map[string]any{}
	}
	if e.Auth != nil {
		if _, ok := data["IsAdmin"]; !ok {
			data["IsAdmin"] = e.Auth.GetString("role") == "admin"
		}
	}
	html, err := r.registry.LoadFS(r.viewsFS, "views/"+page).Render(data)
	if err != nil {
		return err
	}
	return e.HTML(http.StatusOK, html)
}
