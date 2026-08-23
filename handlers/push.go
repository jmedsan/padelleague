package handlers

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/notify"
)

type PushHandler struct {
	app      core.App
	notifier *notify.Notifier
}

func NewPushHandler(app core.App, notifier *notify.Notifier) *PushHandler {
	return &PushHandler{app: app, notifier: notifier}
}

func (h *PushHandler) Enabled() bool {
	return h.notifier.VAPIDPublicKey != "" && h.notifier.VAPIDPrivateKey != ""
}

type pushSubscribeRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

func (h *PushHandler) Subscribe(e *core.RequestEvent) error {
	var req pushSubscribeRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("invalid request body", err)
	}

	epURL, err := url.Parse(req.Endpoint)
	if err != nil || epURL.Scheme != "https" {
		return e.BadRequestError("endpoint must use https", nil)
	}

	existing, _ := h.app.FindFirstRecordByFilter(
		"push_subscriptions",
		"user = {:user} && endpoint = {:endpoint}",
		map[string]any{"user": e.Auth.Id, "endpoint": req.Endpoint},
	)

	if existing != nil {
		existing.Set("p256dh", req.Keys.P256dh)
		existing.Set("auth", req.Keys.Auth)
		if err := h.app.Save(existing); err != nil {
			return fmt.Errorf("failed to update push subscription: %w", err)
		}
	} else {
		col, err := h.app.FindCollectionByNameOrId("push_subscriptions")
		if err != nil {
			return fmt.Errorf("push_subscriptions collection not found: %w", err)
		}
		record := core.NewRecord(col)
		record.Set("user", e.Auth.Id)
		record.Set("endpoint", req.Endpoint)
		record.Set("p256dh", req.Keys.P256dh)
		record.Set("auth", req.Keys.Auth)
		if err := h.app.Save(record); err != nil {
			return fmt.Errorf("failed to create push subscription: %w", err)
		}
	}

	return e.NoContent(http.StatusNoContent)
}

func (h *PushHandler) Unsubscribe(e *core.RequestEvent) error {
	var req struct {
		Endpoint string `json:"endpoint"`
	}
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("invalid request body", err)
	}

	existing, _ := h.app.FindFirstRecordByFilter(
		"push_subscriptions",
		"user = {:user} && endpoint = {:endpoint}",
		map[string]any{"user": e.Auth.Id, "endpoint": req.Endpoint},
	)

	if existing != nil {
		if err := h.app.Delete(existing); err != nil {
			return fmt.Errorf("failed to delete push subscription: %w", err)
		}
	}

	return e.NoContent(http.StatusNoContent)
}
