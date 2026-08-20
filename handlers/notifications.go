package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

type NotificationHandler struct {
	app        core.App
	renderPage func(e *core.RequestEvent, page string, data map[string]any) error
}

func NewNotificationHandler(app core.App, renderPage func(e *core.RequestEvent, page string, data map[string]any) error) *NotificationHandler {
	return &NotificationHandler{app: app, renderPage: renderPage}
}

func (h *NotificationHandler) Count(e *core.RequestEvent) error {
	count := 0
	records, err := h.app.FindRecordsByFilter("notificaciones",
		"user = {:uid} && read = false",
		"", 0, 0,
		map[string]any{"uid": e.Auth.Id})
	if err == nil {
		count = len(records)
	}

	if count == 0 {
		return e.HTML(http.StatusOK, "")
	}
	return e.HTML(http.StatusOK, fmt.Sprintf(`<span class="badge badge-sm indicator-item badge-primary">%d</span>`, count))
}

func (h *NotificationHandler) List(e *core.RequestEvent) error {
	records, err := h.app.FindRecordsByFilter("notificaciones",
		"user = {:uid}",
		"-created", 10, 0,
		map[string]any{"uid": e.Auth.Id})
	if err != nil {
		records = []*core.Record{}
	}

	html := `<div class="card card-compact w-80 bg-base-100 shadow-xl">`
	html += `<div class="card-body">`
	html += `<div class="flex justify-between items-center mb-2">`
	html += `<h3 class="font-bold">Notificaciones</h3>`
	html += `<button hx-post="/notifications/read-all" hx-swap="none" class="btn btn-ghost btn-xs">Marcar todas</button>`
	html += `</div>`

	if len(records) == 0 {
		html += `<p class="text-sm text-base-content/50">Sin notificaciones</p>`
	} else {
		for _, r := range records {
			readClass := ""
			if !r.GetBool("read") {
				readClass = "bg-primary/5 font-medium"
			}
			html += fmt.Sprintf(`<a hx-post="/notifications/%s/read" hx-swap="none" class="block p-2 rounded hover:bg-base-200 cursor-pointer %s">`, r.Id, readClass)
			html += fmt.Sprintf(`<p class="text-sm">%s</p>`, r.GetString("title"))
			if body := r.GetString("body"); body != "" {
				html += fmt.Sprintf(`<p class="text-xs text-base-content/60">%s</p>`, truncate(body, 80))
			}
			html += `</a>`
		}
	}

	html += `<div class="mt-2"><a href="/profile/notifications" class="text-xs link">Preferencias</a></div>`
	html += `</div></div>`

	return e.HTML(http.StatusOK, html)
}

func (h *NotificationHandler) MarkRead(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("notificaciones", id)
	if err != nil {
		return e.NoContent(http.StatusNoContent)
	}

	if record.GetString("user") != e.Auth.Id {
		return e.NoContent(http.StatusNoContent)
	}

	record.Set("read", true)
	h.app.Save(record)

	redirect := "/mis-partidos"
	if related := record.GetString("related_partido"); related != "" {
		redirect = "/partido/" + related
	}

	e.Response.Header().Set("HX-Redirect", redirect)
	return e.NoContent(http.StatusNoContent)
}

func (h *NotificationHandler) MarkAllRead(e *core.RequestEvent) error {
	records, _ := h.app.FindRecordsByFilter("notificaciones",
		"user = {:uid} && read = false",
		"", 0, 0,
		map[string]any{"uid": e.Auth.Id})

	for _, r := range records {
		r.Set("read", true)
		h.app.Save(r)
	}

	e.Response.Header().Set("HX-Redirect", "/")
	return e.NoContent(http.StatusNoContent)
}

func (h *NotificationHandler) Prefs(e *core.RequestEvent) error {
	prefs := getNotificationPrefs(e.Auth)

	return h.renderPage(e, "notification-prefs.html", map[string]any{
		"Prefs": prefs,
	})
}

func (h *NotificationHandler) PrefsSave(e *core.RequestEvent) error {
	prefs := map[string]any{
		"quorum_request": e.Request.FormValue("quorum_request") == "on",
		"dispute":        e.Request.FormValue("dispute") == "on",
		"match_assigned": e.Request.FormValue("match_assigned") == "on",
		"general":        e.Request.FormValue("general") == "on",
	}

	e.Auth.Set("notification_prefs", prefs)
	if err := h.app.Save(e.Auth); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al guardar preferencias</div>`)
	}

	return h.renderPage(e, "notification-prefs.html", map[string]any{
		"Prefs":   prefs,
		"Success": true,
	})
}

func CheckQuorumTimeout(app core.App) {
	cutoff := time.Now().Add(-7 * 24 * time.Hour).UTC().Format(time.RFC3339)
	stale, err := app.FindRecordsByFilter("partidos",
		"status = 'confirmed' && submitted_at < {:cutoff}",
		"", 0, 0,
		map[string]any{"cutoff": cutoff})
	if err != nil || len(stale) == 0 {
		return
	}

	for _, p := range stale {
		p.Set("status", "disputed")
		p.Set("dispute_notes", "Timeout: sin confirmación en 7 días")
		if err := app.Save(p); err == nil {
			pairIDs := []string{p.GetString("pareja1"), p.GetString("pareja2")}
			names, _ := expandPairNames(app, pairIDs)
			notifyAdmins(app, "dispute", "Timeout de confirmación",
				fmt.Sprintf("Partido %s vs %s sin confirmar por más de 7 días", names[pairIDs[0]], names[pairIDs[1]]),
				p.Id)
		}
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
