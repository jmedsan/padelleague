package notify

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"padelleague/league"
)

// Notifier delivers in-app and push notifications to players.
type Notifier struct {
	app             core.App
	vapidPublicKey  string
	vapidPrivateKey string
	httpClient      *http.Client
	save            func(*core.Record) error
	delete          func(*core.Record) error
}

// NewNotifier creates a Notifier with the given VAPID keys for web push.
func NewNotifier(app core.App, vapidPublicKey, vapidPrivateKey string) *Notifier {
	return &Notifier{
		app:             app,
		vapidPublicKey:  vapidPublicKey,
		vapidPrivateKey: vapidPrivateKey,
		httpClient:      &http.Client{Timeout: 10 * time.Second},
		save:            func(rec *core.Record) error { return app.Save(rec) },
		delete:          func(rec *core.Record) error { return app.Delete(rec) },
	}
}

// PushEnabled reports whether VAPID keys are configured for web push.
func (n *Notifier) PushEnabled() bool {
	return n.vapidPublicKey != "" && n.vapidPrivateKey != ""
}

// NotifyPlayers creates an in-app notification and sends a push for each player.
func (n *Notifier) NotifyPlayers(playerUserIDs []string, notif league.Notification) {
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
		if !notificationEnabled(user, notif.Type) {
			continue
		}
		n.deliver(notifCol, userID, notif, "notify player failed")
	}
}

// NotifyAdmins creates an in-app notification and sends a push for each admin user.
// excludeUserIDs are skipped (e.g. participants already notified as players).
func (n *Notifier) NotifyAdmins(notif league.Notification, excludeUserIDs ...string) error {
	notifCol, err := n.app.FindCollectionByNameOrId("notifications")
	if err != nil {
		return err
	}
	admins, err := n.app.FindRecordsByFilter("users", "roles ~ 'admin'", "", 0, 0, nil)
	if err != nil {
		return err
	}
	for _, admin := range filterRecipients(admins, notif.Type, excludeUserIDs) {
		n.deliver(notifCol, admin.Id, notif, "notify admin failed")
	}
	return nil
}

// deliver saves the in-app notification record for userID and fires its push.
// saveFailMsg is the slog message logged when the save fails, so callers keep
// a distinct diagnostic per recipient class (player vs admin).
func (n *Notifier) deliver(notifCol *core.Collection, userID string, notif league.Notification, saveFailMsg string) {
	rec := core.NewRecord(notifCol)
	rec.Set("user", userID)
	rec.Set("type", notif.Type)
	rec.Set("title", notif.Title)
	rec.Set("body", notif.Body)
	if notif.MatchID != "" {
		rec.Set("related_match", notif.MatchID)
	}
	if notif.CompName != "" {
		rec.Set("comp_name", notif.CompName)
	}
	link := notificationLink(notif)
	if link != "" {
		rec.Set("link", link)
	}
	if err := n.save(rec); err != nil {
		slog.Error(saveFailMsg, "user", userID, "err", err)
	}
	go n.sendPush(userID, notif.Title, notif.Body, link)
}

func notificationEnabled(user *core.Record, notifType string) bool {
	enabled, ok := NotificationPrefs(user)[notifType]
	if !ok {
		return true
	}
	b, ok := enabled.(bool)
	return !ok || b
}

// notificationLink resolves the link stored on a notification record: the
// explicit Link when set, otherwise the match page for MatchID, otherwise
// empty (no link column write).
func notificationLink(notif league.Notification) string {
	if notif.Link != "" {
		return notif.Link
	}
	if notif.MatchID != "" {
		return "/match/" + notif.MatchID
	}
	return ""
}

func filterRecipients(users []*core.Record, notifType string, excludeIDs []string) []*core.Record {
	excludeSet := make(map[string]struct{}, len(excludeIDs))
	for _, id := range excludeIDs {
		excludeSet[id] = struct{}{}
	}
	var out []*core.Record
	for _, u := range users {
		if _, excluded := excludeSet[u.Id]; excluded {
			continue
		}
		prefs := NotificationPrefs(u)
		if enabled, ok := prefs[notifType]; ok {
			if b, ok := enabled.(bool); ok && !b {
				continue
			}
		}
		out = append(out, u)
	}
	return out
}

func (n *Notifier) sendPush(userID, title, body, targetURL string) {
	if n.vapidPublicKey == "" || n.vapidPrivateKey == "" {
		return
	}

	subs, err := n.app.FindRecordsByFilter("push_subscriptions", "user = {:user}", "", 0, 0, map[string]any{"user": userID})
	if err != nil || len(subs) == 0 {
		return
	}

	if targetURL == "" {
		targetURL = "/"
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
		slog.Error("push send failed", "user", sub.GetString("user"), "err", err)
		return
	}
	if err := resp.Body.Close(); err != nil {
		slog.Warn("close push response", "err", err)
	}
	if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
		if err := n.delete(sub); err != nil {
			slog.Error("push delete subscription failed", "err", err)
		}
	}
}

// NotificationPrefs returns the user's notification preferences with defaults applied.
func NotificationPrefs(user *core.Record) map[string]any {
	defaults := map[string]any{
		"quorum_request": true,
		"dispute":        true,
		"match_assigned": true,
		"general":        true,
		"scheduling":     true,
		"match_progress": true,
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
