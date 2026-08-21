package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/pocketbase/pocketbase/core"
)

type PushHandler struct {
	app        core.App
	publicKey  string
	privateKey string
}

func NewPushHandler(app core.App) *PushHandler {
	return &PushHandler{
		app:        app,
		publicKey:  os.Getenv("VAPID_PUBLIC_KEY"),
		privateKey: os.Getenv("VAPID_PRIVATE_KEY"),
	}
}

func (h *PushHandler) Enabled() bool {
	return h.publicKey != "" && h.privateKey != ""
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

func sendPush(app core.App, userID, title, body, relatedMatchID string) {
	publicKey := os.Getenv("VAPID_PUBLIC_KEY")
	privateKey := os.Getenv("VAPID_PRIVATE_KEY")
	if publicKey == "" || privateKey == "" {
		return
	}

	subs, err := app.FindRecordsByFilter("push_subscriptions", "user = {:user}", "", 0, 0, map[string]any{"user": userID})
	if err != nil || len(subs) == 0 {
		return
	}

	url := "/"
	if relatedMatchID != "" {
		url = "/match/" + relatedMatchID
	}

	payload, _ := json.Marshal(map[string]string{
		"title": title,
		"body":  body,
		"url":   url,
	})

	subscriber := app.Settings().Meta.SenderAddress
	if subscriber == "" {
		subscriber = "noreply@padelleague.com"
	}
	subscriber = "mailto:" + subscriber

	for _, sub := range subs {
		s := &webpush.Subscription{
			Endpoint: sub.GetString("endpoint"),
			Keys: webpush.Keys{
				P256dh: sub.GetString("p256dh"),
				Auth:   sub.GetString("auth"),
			},
		}

		resp, err := webpush.SendNotification(payload, s, &webpush.Options{
			Subscriber:      subscriber,
			VAPIDPublicKey:  publicKey,
			VAPIDPrivateKey: privateKey,
		})
		if err != nil {
			log.Printf("sendPush: failed to send to %s: %v", sub.GetString("endpoint"), err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusGone {
			if err := app.Delete(sub); err != nil {
				log.Printf("sendPush: failed to delete gone subscription: %v", err)
			}
		}
	}
}
