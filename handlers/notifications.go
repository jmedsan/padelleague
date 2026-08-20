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
		"", 10, 0,
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
	record, err := h.app.FindRecordById("notifications", id)
	if err != nil {
		return e.NoContent(http.StatusNoContent)
	}

	if record.GetString("user") != e.Auth.Id {
		return e.NoContent(http.StatusNoContent)
	}

	record.Set("read", true)
	h.app.Save(record)

	redirect := "/"
	if related := record.GetString("related_match"); related != "" {
		redirect = "/match/" + related
	}

	e.Response.Header().Set("HX-Redirect", redirect)
	return e.NoContent(http.StatusNoContent)
}

func (h *NotificationHandler) MarkAllRead(e *core.RequestEvent) error {
	records, _ := h.app.FindRecordsByFilter("notifications",
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
		"scheduling":     e.Request.FormValue("scheduling") == "on",
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
	stale, err := app.FindRecordsByFilter("matches",
		"status = 'confirmed'", "", 0, 0, nil)
	if err != nil || len(stale) == 0 {
		return
	}

	compCache := map[string]*core.Record{}
	for _, m := range stale {
		compID := m.GetString("competition")
		if _, ok := compCache[compID]; !ok {
			comp, err := app.FindRecordById("competitions", compID)
			if err != nil {
				compCache[compID] = nil
				continue
			}
			compCache[compID] = comp
		}
	}

	for _, m := range stale {
		comp := compCache[m.GetString("competition")]
		if comp == nil {
			continue
		}

		timeoutHours := int(comp.GetFloat("quorum_timeout_hours"))
		if timeoutHours == 0 {
			continue
		}

		submittedAt := m.GetString("submitted_at")
		if submittedAt == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, submittedAt)
		if err != nil {
			continue
		}
		if time.Since(t) < time.Duration(timeoutHours)*time.Hour {
			continue
		}

		fresh, err := app.FindRecordById("matches", m.Id)
		if err != nil || fresh.GetString("status") != "confirmed" {
			continue
		}

		score := fresh.GetString("scores")
		winnerID, err := determineWinner(fresh, score)
		if err != nil {
			continue
		}

		fresh.Set("status", "final")
		fresh.Set("winner", winnerID)
		fresh.Set("confirmed_by", "")
		fresh.Set("dispute_notes", "Auto-confirmado por tiempo de espera")
		if err := app.Save(fresh); err != nil {
			continue
		}

		pairIDs := []string{fresh.GetString("pair1"), fresh.GetString("pair2")}
		for _, pid := range pairIDs {
			players := getPlayersForPair(app, pid)
			notifyPlayers(app, players, "general",
				"Resultado confirmado automaticamente",
				"El resultado ha sido confirmado por tiempo de espera.",
				fresh.Id)
		}
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
