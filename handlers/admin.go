package handlers

import (
	"fmt"
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

type AdminHandler struct {
	app        core.App
	renderPage func(e *core.RequestEvent, page string, data map[string]any) error
}

func NewAdminHandler(app core.App, renderPage func(e *core.RequestEvent, page string, data map[string]any) error) *AdminHandler {
	return &AdminHandler{app: app, renderPage: renderPage}
}

func (h *AdminHandler) Categorias(e *core.RequestEvent) error {
	categories, err := h.app.FindAllRecords("categorias")
	if err != nil {
		categories = []*core.Record{}
	}

	return h.renderPage(e, "admin/categorias.html", map[string]any{
		"DisplayName": e.Auth.GetString("display_name"),
		"IsAdmin":     e.Auth.GetString("role") == "admin",
		"Categories":  categories,
	})
}

func (h *AdminHandler) CategoriasCreate(e *core.RequestEvent) error {
	collection, err := h.app.FindCollectionByNameOrId("categorias")
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error interno</div>`)
	}

	record := core.NewRecord(collection)
	record.Set("name", e.Request.FormValue("name"))
	record.Set("description", e.Request.FormValue("description"))
	record.Set("sport_type", e.Request.FormValue("sport_type"))

	if err := h.app.Save(record); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al crear la categoría. Verifica los datos.</div>`)
	}

	e.Response.Header().Set("HX-Redirect", "/admin/categorias")
	return e.NoContent(http.StatusNoContent)
}

func (h *AdminHandler) Temporadas(e *core.RequestEvent) error {
	categoriaID := e.Request.URL.Query().Get("categoria")
	if categoriaID == "" {
		return e.Redirect(http.StatusFound, "/admin/categorias")
	}

	categoria, err := h.app.FindRecordById("categorias", categoriaID)
	if err != nil {
		return e.Redirect(http.StatusFound, "/admin/categorias")
	}

	seasons, _ := h.app.FindRecordsByFilter("temporadas",
		"categoria = {:cat}",
		"-start_date", 0, 0,
		map[string]any{"cat": categoriaID})

	pairCounts := make(map[string]int)
	for _, s := range seasons {
		pairs, _ := h.app.FindRecordsByFilter("parejas",
			"temporada = {:id}",
			"", 0, 0,
			map[string]any{"id": s.Id})
		pairCounts[s.Id] = len(pairs)
	}

	return h.renderPage(e, "admin/temporadas.html", map[string]any{
		"DisplayName": e.Auth.GetString("display_name"),
		"IsAdmin":     true,
		"Category":    categoria,
		"Seasons":     seasons,
		"PairCounts":  pairCounts,
	})
}

func (h *AdminHandler) TemporadasCreate(e *core.RequestEvent) error {
	categoriaID := e.Request.FormValue("categoria")

	collection, err := h.app.FindCollectionByNameOrId("temporadas")
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error interno</div>`)
	}

	record := core.NewRecord(collection)
	record.Set("name", e.Request.FormValue("name"))
	record.Set("categoria", categoriaID)
	record.Set("start_date", e.Request.FormValue("start_date"))
	record.Set("end_date", e.Request.FormValue("end_date"))
	record.Set("play_twice", e.Request.FormValue("play_twice") == "on")

	if err := h.app.Save(record); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al crear la temporada. Verifica los datos.</div>`)
	}

	e.Response.Header().Set("HX-Redirect", fmt.Sprintf("/admin/temporadas?categoria=%s", categoriaID))
	return e.NoContent(http.StatusNoContent)
}

func (h *AdminHandler) TemporadasToggle(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("temporadas", id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Temporada no encontrada</div>`)
	}

	categoriaID := record.GetString("categoria")
	wasActive := record.GetBool("active")

	err = h.app.RunInTransaction(func(txApp core.App) error {
		activeSeasons, _ := txApp.FindRecordsByFilter("temporadas",
			"categoria = {:cat} && active = true",
			"", 0, 0,
			map[string]any{"cat": categoriaID})
		for _, s := range activeSeasons {
			s.Set("active", false)
			if err := txApp.Save(s); err != nil {
				return err
			}
		}

		if !wasActive {
			rec, err := txApp.FindRecordById("temporadas", id)
			if err != nil {
				return err
			}
			rec.Set("active", true)
			return txApp.Save(rec)
		}
		return nil
	})

	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al cambiar estado</div>`)
	}

	e.Response.Header().Set("HX-Redirect", fmt.Sprintf("/admin/temporadas?categoria=%s", categoriaID))
	return e.NoContent(http.StatusNoContent)
}

func (h *AdminHandler) CategoriasUpdate(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("categorias", id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Categoría no encontrada</div>`)
	}

	record.Set("name", e.Request.FormValue("name"))
	record.Set("description", e.Request.FormValue("description"))
	record.Set("sport_type", e.Request.FormValue("sport_type"))

	if err := h.app.Save(record); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al actualizar la categoría</div>`)
	}

	e.Response.Header().Set("HX-Redirect", "/admin/categorias")
	return e.NoContent(http.StatusNoContent)
}
