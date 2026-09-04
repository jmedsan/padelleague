package handlers

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/notify"
)

// NotificationHandler handles notification listing, reading, and preferences.
type NotificationHandler struct {
	app           core.App
	renderPage    RenderFunc
	renderPartial RenderFunc
}

// NewNotificationHandler creates a NotificationHandler with the given dependencies.
func NewNotificationHandler(app core.App, renderPage, renderPartial RenderFunc) *NotificationHandler {
	return &NotificationHandler{app: app, renderPage: renderPage, renderPartial: renderPartial}
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

	return h.renderPartial(e, "notification-bell.html", map[string]any{
		"Notifications": NewNotificationViews(records, PlayerRow),
	})
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

	return h.renderPartial(e, "notification-badges.html", map[string]any{
		"UnreadCount": len(remaining),
	})
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

	return h.renderPartial(e, "notification-list-oob.html", map[string]any{
		"Notifications": []NotificationView{},
	})
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
		"Notifications": NewNotificationViews(records, PlayerReadOnly),
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
