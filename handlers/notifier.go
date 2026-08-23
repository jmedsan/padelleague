package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/pocketbase/pocketbase/core"
)

type Notifier struct {
	app             core.App
	vapidPublicKey  string
	vapidPrivateKey string
	httpClient      *http.Client
}

func NewNotifier(app core.App, vapidPublicKey, vapidPrivateKey string) *Notifier {
	return &Notifier{
		app:             app,
		vapidPublicKey:  vapidPublicKey,
		vapidPrivateKey: vapidPrivateKey,
		httpClient:      &http.Client{Timeout: 10 * time.Second},
	}
}

func (n *Notifier) NotifyPlayers(playerUserIDs []string, notifType, title, body, relatedMatchID string) {
	notifCol, err := n.app.FindCollectionByNameOrId("notifications")
	if err != nil {
		slog.Error("notifications collection not found", "err", err)
		return
	}
	for _, userID := range playerUserIDs {
		user, err := n.app.FindRecordById("users", userID)
		if err != nil {
			continue
		}
		prefs := getNotificationPrefs(user)
		if enabled, ok := prefs[notifType]; ok {
			if b, ok := enabled.(bool); ok && !b {
				continue
			}
		}
		notif := core.NewRecord(notifCol)
		notif.Set("user", userID)
		notif.Set("type", notifType)
		notif.Set("title", title)
		notif.Set("body", body)
		if relatedMatchID != "" {
			notif.Set("related_match", relatedMatchID)
		}
		if err := n.app.Save(notif); err != nil {
			slog.Error("notify player failed", "user", userID, "err", err)
		}
		go n.sendPush(userID, title, body, relatedMatchID)
	}
}

func (n *Notifier) NotifyAdmins(notifType, title, body, relatedMatchID string) error {
	notifCol, err := n.app.FindCollectionByNameOrId("notifications")
	if err != nil {
		return err
	}
	admins, err := n.app.FindRecordsByFilter("users", "role = 'admin'", "", 0, 0, nil)
	if err != nil {
		return err
	}
	for _, admin := range admins {
		notif := core.NewRecord(notifCol)
		notif.Set("user", admin.Id)
		notif.Set("type", notifType)
		notif.Set("title", title)
		notif.Set("body", body)
		if relatedMatchID != "" {
			notif.Set("related_match", relatedMatchID)
		}
		if err := n.app.Save(notif); err != nil {
			slog.Error("notify admin failed", "admin", admin.Id, "err", err)
		}
		go n.sendPush(admin.Id, title, body, relatedMatchID)
	}
	return nil
}

func (n *Notifier) sendPush(userID, title, body, relatedMatchID string) {
	if n.vapidPublicKey == "" || n.vapidPrivateKey == "" {
		return
	}

	subs, err := n.app.FindRecordsByFilter("push_subscriptions", "user = {:user}", "", 0, 0, map[string]any{"user": userID})
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

	subscriber := n.app.Settings().Meta.SenderAddress
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
			VAPIDPublicKey:  n.vapidPublicKey,
			VAPIDPrivateKey: n.vapidPrivateKey,
			HTTPClient:      n.httpClient,
		})
		if err != nil {
			slog.Error("push send failed", "endpoint", sub.GetString("endpoint"), "err", err)
			continue
		}
		if err := resp.Body.Close(); err != nil {
			slog.Warn("close push response", "err", err)
		}

		if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
			if err := n.app.Delete(sub); err != nil {
				slog.Error("push delete subscription failed", "err", err)
			}
		}
	}
}
