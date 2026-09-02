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

// NotificationHandler handles notification listing, reading, and preferences.
type NotificationHandler struct {
	app        core.App
	renderPage RenderFunc
}

// NewNotificationHandler creates a NotificationHandler with the given dependencies.
func NewNotificationHandler(app core.App, renderPage RenderFunc) *NotificationHandler {
	return &NotificationHandler{app: app, renderPage: renderPage}
}

// Count returns the number of unread notifications as an HTMX badge fragment.
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

// List renders the notification dropdown with recent notifications.
func (h *NotificationHandler) List(e *core.RequestEvent) error {
	records, err := h.app.FindRecordsByFilter("notifications",
		"user = {:uid} && read = false",
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
	out += `<div id="notif-list">`
	out += notificationListItems(records)
	out += `</div>`

	out += `<div class="mt-2 flex justify-between items-center">`
	out += `<a href="/notifications/history" class="text-xs link">Ver todas →</a>`
	out += `<a href="/profile/notifications" class="text-xs link">Preferencias</a>`
	out += `</div>`
	out += `</div></div>`

	return e.HTML(http.StatusOK, out)
}

// notificationListItems renders the bell dropdown's notification rows, or
// its empty state — the content that both List and MarkAllRead's OOB swap
// put inside #notif-list.
func notificationListItems(records []*core.Record) string {
	if len(records) == 0 {
		return `<p class="text-sm text-base-content/50">No tienes notificaciones</p>`
	}
	var out string
	for _, r := range records {
		out += fmt.Sprintf(`<div id="notif-row-%s" class="flex items-start gap-1 bg-primary/5 font-medium rounded p-1">`, r.Id)
		out += fmt.Sprintf(`<a href="%s" hx-post="/notifications/%s/read" hx-swap="none" class="flex-1 block p-1 hover:bg-base-200 cursor-pointer rounded">`, html.EscapeString(notificationLink(r)), r.Id)
		out += fmt.Sprintf(`<p class="text-sm">%s</p>`, html.EscapeString(r.GetString("title")))
		if body := r.GetString("body"); body != "" {
			out += fmt.Sprintf(`<p class="text-xs text-base-content/60">%s</p>`, html.EscapeString(league.Truncate(body, 80)))
		}
		out += `</a>`
		out += fmt.Sprintf(`<button hx-post="/notifications/%s/dismiss" hx-target="#notif-row-%s" hx-swap="delete" class="btn btn-ghost btn-xs btn-circle opacity-50 hover:opacity-100" aria-label="marcar leída">&#10005;</button>`, r.Id, r.Id)
		out += `</div>`
	}
	return out
}

// Dismiss marks a notification as read and removes it from the bell.
func (h *NotificationHandler) Dismiss(e *core.RequestEvent) error {
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
		slog.Error("dismiss notification", "id", id, "err", err)
	}

	remaining, _ := h.app.FindRecordsByFilter("notifications",
		"user = {:uid} && read = false",
		"", 0, 0,
		map[string]any{"uid": e.Auth.Id})
	count := len(remaining)

	badge := ""
	if count > 0 {
		badge = fmt.Sprintf(`%d`, count)
	}

	out := fmt.Sprintf(`<span id="notif-badge" hx-swap-oob="innerHTML">%s</span>`, badge)
	out += fmt.Sprintf(`<span id="notif-badge-mobile" hx-swap-oob="innerHTML">%s</span>`, badge)
	return e.HTML(http.StatusOK, out)
}

// MarkRead marks a single notification as read.
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

	return redirectHX(e, notificationLink(record))
}

// MarkAllRead marks all of the user's notifications as read.
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

	out := `<span id="notif-badge" hx-swap-oob="innerHTML"></span>`
	out += `<span id="notif-badge-mobile" hx-swap-oob="innerHTML"></span>`
	out += fmt.Sprintf(`<div id="notif-list" hx-swap-oob="innerHTML">%s</div>`, notificationListItems(nil))
	return e.HTML(http.StatusOK, out)
}

// History renders the full notification history page.
func (h *NotificationHandler) History(e *core.RequestEvent) error {
	records, err := h.app.FindRecordsByFilter("notifications",
		"user = {:uid}",
		"-created", 50, 0,
		map[string]any{"uid": e.Auth.Id})
	if err != nil {
		records = []*core.Record{}
	}

	return h.renderPage(e, "notification-history.html", map[string]any{
		"PageTitle":     "Notificaciones",
		"Notifications": records,
	})
}

// Prefs renders the notification preferences page.
func (h *NotificationHandler) Prefs(e *core.RequestEvent) error {
	prefs := notify.NotificationPrefs(e.Auth)

	return h.renderPage(e, "notification-prefs.html", map[string]any{
		"PageTitle": "Preferencias",
		"Prefs":     prefs,
	})
}

// PrefsSave handles POST to update the user's notification preferences.
func (h *NotificationHandler) PrefsSave(e *core.RequestEvent) error {
	prefs := map[string]any{
		"quorum_request": e.Request.FormValue("quorum_request") == "on",
		"dispute":        e.Request.FormValue("dispute") == "on",
		"match_assigned": e.Request.FormValue("match_assigned") == "on",
		"general":        e.Request.FormValue("general") == "on",
		"scheduling":     e.Request.FormValue("scheduling") == "on",
		"match_progress": e.Request.FormValue("match_progress") == "on",
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

// notificationLink resolves where a notification points: its own link,
// otherwise its related match, otherwise home.
func notificationLink(r *core.Record) string {
	if link := r.GetString("link"); link != "" {
		return link
	}
	if related := r.GetString("related_match"); related != "" {
		return "/match/" + related
	}
	return "/"
}
