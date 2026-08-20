package handlers

import (
	"net/http"
	"strconv"

	"github.com/pocketbase/pocketbase/core"
)

func (h *AdminHandler) Parejas(e *core.RequestEvent) error {
	temporadaID := e.Request.URL.Query().Get("temporada")
	if temporadaID == "" {
		return e.Redirect(http.StatusFound, "/admin/temporadas")
	}

	temporada, err := h.app.FindRecordById("temporadas", temporadaID)
	if err != nil {
		return e.Redirect(http.StatusFound, "/admin/temporadas")
	}

	h.app.ExpandRecord(temporada, []string{"categoria"}, nil)
	categoriaID := temporada.GetString("categoria")

	parejas, _ := h.app.FindRecordsByFilter("parejas",
		"temporada = {:tid}", "", 0, 0,
		map[string]any{"tid": temporadaID})

	type pairView struct {
		Record *core.Record
		Number int
		Name1  string
		Name2  string
	}
	var pairViews []pairView
	for i, p := range parejas {
		pairViews = append(pairViews, pairView{
			Record: p,
			Number: i + 1,
			Name1:  resolvePlayerName(h.app, p.GetString("jugador1")),
			Name2:  resolvePlayerName(h.app, p.GetString("jugador2")),
		})
	}

	jugadores, _ := h.app.FindRecordsByFilter("jugadores",
		"categorias ~ {:cid}", "", 0, 0,
		map[string]any{"cid": categoriaID})

	h.app.ExpandRecords(jugadores, []string{"user"}, nil)

	return h.renderPage(e, "admin/parejas.html", map[string]any{
		"DisplayName": e.Auth.GetString("display_name"),
		"IsAdmin":     e.Auth.GetString("role") == "admin",
		"Temporada":   temporada,
		"TemporadaID": temporadaID,
		"CategoriaID": categoriaID,
		"Parejas":     pairViews,
		"Jugadores":   jugadores,
	})
}

func (h *AdminHandler) ParejasCreate(e *core.RequestEvent) error {
	jugador1 := e.Request.FormValue("jugador1")
	jugador2 := e.Request.FormValue("jugador2")
	categoriaID := e.Request.FormValue("categoria")
	temporadaID := e.Request.FormValue("temporada")

	if jugador1 == "" || jugador2 == "" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Debes seleccionar ambos jugadores</div>`)
	}

	if jugador1 == jugador2 {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Los dos jugadores deben ser diferentes</div>`)
	}

	collection, err := h.app.FindCollectionByNameOrId("parejas")
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error interno</div>`)
	}

	record := core.NewRecord(collection)
	record.Set("jugador1", jugador1)
	record.Set("jugador2", jugador2)
	record.Set("categoria", categoriaID)
	record.Set("temporada", temporadaID)

	if err := h.app.Save(record); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al crear la pareja</div>`)
	}

	return h.renderParejasList(e, temporadaID)
}

func (h *AdminHandler) renderParejasList(e *core.RequestEvent, temporadaID string) error {
	parejas, _ := h.app.FindRecordsByFilter("parejas",
		"temporada = {:tid}", "", 0, 0,
		map[string]any{"tid": temporadaID})

	html := `<div id="list"><table class="table"><thead><tr><th>#</th><th>Jugador 1</th><th>Jugador 2</th></tr></thead><tbody>`
	for i, p := range parejas {
		name1 := resolvePlayerName(h.app, p.GetString("jugador1"))
		name2 := resolvePlayerName(h.app, p.GetString("jugador2"))
		html += `<tr><td>` + strconv.Itoa(i+1) + `</td><td>` + name1 + `</td><td>` + name2 + `</td></tr>`
	}
	if len(parejas) == 0 {
		html += `<tr><td colspan="3" class="text-center text-base-content/50">No hay parejas registradas</td></tr>`
	}
	html += `</tbody></table></div>`

	return e.HTML(http.StatusOK, html)
}

