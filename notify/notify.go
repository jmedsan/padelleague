package notify

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Notifier delivers in-app and push notifications to players.
type Notifier struct {
	app             core.App
	vapidPublicKey  string
	vapidPrivateKey string
	httpClient      *http.Client
}

// NewNotifier creates a Notifier with the given VAPID keys for web push.
func NewNotifier(app core.App, vapidPublicKey, vapidPrivateKey string) *Notifier {
	return &Notifier{
		app:             app,
		vapidPublicKey:  vapidPublicKey,
		vapidPrivateKey: vapidPrivateKey,
		httpClient:      &http.Client{Timeout: 10 * time.Second},
	}
}

// PushEnabled reports whether VAPID keys are configured for web push.
func (n *Notifier) PushEnabled() bool {
	return n.vapidPublicKey != "" && n.vapidPrivateKey != ""
}

// NotifyPlayers creates an in-app notification and sends a push for each player.
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
		prefs := GetNotificationPrefs(user)
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

// NotifyAdmins creates an in-app notification and sends a push for each admin user.
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

	targetURL := "/"
	if relatedMatchID != "" {
		targetURL = "/match/" + relatedMatchID
	}

	payload, _ := json.Marshal(map[string]string{
		"title": title,
		"body":  body,
		"url":   targetURL,
	})

	subscriber := n.app.Settings().Meta.SenderAddress
	if subscriber == "" {
		subscriber = "noreply@padelleague.com"
	}
	subscriber = "mailto:" + subscriber

	for _, sub := range subs {
		n.deliverPush(sub, payload, subscriber)
	}
}

func (n *Notifier) deliverPush(sub *core.Record, payload []byte, subscriber string) {
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
		return
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

// GetNotificationPrefs returns the user's notification preferences with defaults applied.
func GetNotificationPrefs(user *core.Record) map[string]any {
	defaults := map[string]any{
		"quorum_request": true,
		"dispute":        true,
		"match_assigned": true,
		"general":        true,
		"scheduling":     true,
	}
	prefs, ok := decodePrefs(user.Get("notification_prefs"))
	if !ok {
		return defaults
	}
	for k, v := range defaults {
		if _, exists := prefs[k]; !exists {
			prefs[k] = v
		}
	}
	return prefs
}

// decodePrefs reads the notification_prefs record value. PocketBase stores a
// JSONField as types.JSONRaw, so the stored bytes have to be unmarshaled; a
// plain map only shows up for a record set in memory and not yet saved.
func decodePrefs(raw any) (map[string]any, bool) {
	switch v := raw.(type) {
	case map[string]any:
		return v, true
	case types.JSONRaw:
		if len(v) == 0 {
			return nil, false
		}
		var prefs map[string]any
		if err := json.Unmarshal(v, &prefs); err != nil {
			slog.Error("decode notification prefs failed", "err", err)
			return nil, false
		}
		return prefs, prefs != nil
	default:
		return nil, false
	}
}
