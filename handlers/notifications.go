package handlers

import (
	"fmt"
	"html"
	"log/slog"
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
	"padelleague/notify"
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
	records, err := h.app.FindRecordsByFilter("notifications",
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
	records, err := h.app.FindRecordsByFilter("notifications",
		"user = {:uid}",
		"-created", 10, 0,
		map[string]any{"uid": e.Auth.Id})
	if err != nil {
		records = []*core.Record{}
	}

	out := `<div class="card card-compact w-80 bg-base-100 shadow-xl">`
	out += `<div class="card-body">`
	out += `<div class="flex justify-between items-center mb-2">`
	out += `<h3 class="font-bold">Notificaciones</h3>`
	out += `<button hx-post="/notifications/read-all" hx-swap="none" class="btn btn-ghost btn-xs">Marcar todas</button>`
	out += `</div>`

	if len(records) == 0 {
		out += `<p class="text-sm text-base-content/50">Sin notificaciones</p>`
	} else {
		for _, r := range records {
			readClass := ""
			if !r.GetBool("read") {
				readClass = "bg-primary/5 font-medium"
			}
			out += fmt.Sprintf(`<a hx-post="/notifications/%s/read" hx-swap="none" class="block p-2 rounded hover:bg-base-200 cursor-pointer %s">`, r.Id, readClass)
			out += fmt.Sprintf(`<p class="text-sm">%s</p>`, html.EscapeString(r.GetString("title")))
			if body := r.GetString("body"); body != "" {
				out += fmt.Sprintf(`<p class="text-xs text-base-content/60">%s</p>`, html.EscapeString(league.Truncate(body, 80)))
			}
			out += `</a>`
		}
	}

	out += `<div class="mt-2"><a href="/profile/notifications" class="text-xs link">Preferencias</a></div>`
	out += `</div></div>`

	return e.HTML(http.StatusOK, out)
}

func (h *NotificationHandler) MarkRead(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("notifications", id)
	if err != nil {
		return e.NoContent(http.StatusNoContent)
	}

	if record.GetString("user") != e.Auth.Id {
		return e.NoContent(http.StatusNoContent)
	}

	record.Set("read", true)
	if err := h.app.Save(record); err != nil {
		slog.Error("mark notification read", "id", id, "err", err)
	}

	redirect := "/"
	if related := record.GetString("related_match"); related != "" {
		redirect = "/match/" + related
	}

	return redirectHX(e, redirect)
}

func (h *NotificationHandler) MarkAllRead(e *core.RequestEvent) error {
	records, _ := h.app.FindRecordsByFilter("notifications",
		"user = {:uid} && read = false",
		"", 0, 0,
		map[string]any{"uid": e.Auth.Id})

	for _, r := range records {
		r.Set("read", true)
		if err := h.app.Save(r); err != nil {
			slog.Error("mark notification read", "id", r.Id, "err", err)
		}
	}

	return redirectHX(e, "/")
}

func (h *NotificationHandler) Prefs(e *core.RequestEvent) error {
	prefs := notify.GetNotificationPrefs(e.Auth)

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
		"scheduling":     e.Request.FormValue("scheduling") == "on",
	}

	e.Auth.Set("notification_prefs", prefs)
	if err := h.app.Save(e.Auth); err != nil {
		return alertError(e, "Error al guardar preferencias")
	}

	return h.renderPage(e, "notification-prefs.html", map[string]any{
		"Prefs":   prefs,
		"Success": true,
	})
}
