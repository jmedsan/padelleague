package handlers

import "github.com/pocketbase/pocketbase/core"

// NotificationView is a view-model for rendering a notification across
// surfaces (bell dropdown, history page).
type NotificationView struct {
	Record *core.Record
	Link   string
	Mode   Mode
}

// NewNotificationView builds a NotificationView from a notification record.
func NewNotificationView(r *core.Record, mode Mode) NotificationView {
	return NotificationView{Record: r, Link: notificationLink(r), Mode: mode}
}

// NewNotificationViews builds NotificationViews for a slice of records.
func NewNotificationViews(records []*core.Record, mode Mode) []NotificationView {
	views := make([]NotificationView, len(records))
	for i, r := range records {
		views[i] = NewNotificationView(r, mode)
	}
	return views
}
