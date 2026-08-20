package handlers

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

func (h *AdminHandler) Jugadores(e *core.RequestEvent) error {
	jugadores, err := h.app.FindAllRecords("jugadores")
	if err != nil {
		jugadores = []*core.Record{}
	}

	h.app.ExpandRecords(jugadores, []string{"user", "categorias"}, nil)

	users, err := h.app.FindAllRecords("users")
	if err != nil {
		users = []*core.Record{}
	}

	categories, err := h.app.FindAllRecords("categorias")
	if err != nil {
		categories = []*core.Record{}
	}

	return h.renderPage(e, "admin/jugadores.html", map[string]any{
		"DisplayName": e.Auth.GetString("display_name"),
		"IsAdmin":     e.Auth.GetString("role") == "admin",
		"Jugadores":   jugadores,
		"Users":       users,
		"Categories":  categories,
	})
}

func (h *AdminHandler) JugadoresCreate(e *core.RequestEvent) error {
	collection, err := h.app.FindCollectionByNameOrId("jugadores")
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error interno</div>`)
	}

	userID := e.Request.FormValue("user")
	if userID == "" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Debes seleccionar un usuario</div>`)
	}

	if err := e.Request.ParseForm(); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al procesar el formulario</div>`)
	}
	categoryIDs := e.Request.Form["categorias"]

	record := core.NewRecord(collection)
	record.Set("user", userID)
	if len(categoryIDs) > 0 {
		record.Set("categorias", categoryIDs)
	}

	if err := h.app.Save(record); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al registrar el jugador. ¿Ya está registrado?</div>`)
	}

	return h.renderJugadoresList(e)
}

func (h *AdminHandler) renderJugadoresList(e *core.RequestEvent) error {
	jugadores, err := h.app.FindAllRecords("jugadores")
	if err != nil {
		jugadores = []*core.Record{}
	}

	h.app.ExpandRecords(jugadores, []string{"user", "categorias"}, nil)

	html := `<div id="list"><table class="table"><thead><tr><th>Nombre</th><th>Email</th><th>Categorías</th></tr></thead><tbody>`
	for _, j := range jugadores {
		name := ""
		email := ""
		if u, ok := j.Expand()["user"].(*core.Record); ok {
			name = u.GetString("display_name")
			email = u.GetString("email")
		}
		catBadges := ""
		if cats, ok := j.Expand()["categorias"].([]*core.Record); ok {
			for _, c := range cats {
				catBadges += `<span class="badge badge-outline mr-1">` + c.GetString("name") + `</span>`
			}
		}
		html += `<tr><td>` + name + `</td><td>` + email + `</td><td>` + catBadges + `</td></tr>`
	}
	html += `</tbody></table></div>`

	return e.HTML(http.StatusOK, html)
}
