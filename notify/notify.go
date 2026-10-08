package notify

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"padelleague/league"
)

// Notifier delivers in-app, push, and email notifications to players.
type Notifier struct {
	app             core.App
	vapidPublicKey  string
	vapidPrivateKey string
	httpClient      *http.Client
	save            func(*core.Record) error
	delete          func(*core.Record) error

	// pushMu guards pushPending and pushClosed. In-flight sendPush goroutines
	// fired from deliver are counted so callers can drain them (WaitPush)
	// before tearing down the app; without that, a push goroutine can still
	// be running FindRecordsByFilter against app after the app has shut down
	// (tests) or is shutting down (production OnTerminate), segfaulting or
	// logging into a closed DB. A sync.WaitGroup cannot do this job: deliver
	// keeps adding pushes from request goroutines while the drain waits, and
	// WaitGroup forbids an Add from zero concurrent with Wait (it panics).
	pushMu      sync.Mutex
	pushIdle    *sync.Cond // signaled when pushPending drops to zero
	pushPending int
	pushClosed  bool // set by WaitPushTimeout: shutdown has begun, start no more pushes
}

// NewNotifier creates a Notifier with the given VAPID keys for web push.
func NewNotifier(app core.App, vapidPublicKey, vapidPrivateKey string) *Notifier {
	n := &Notifier{
		app:             app,
		vapidPublicKey:  vapidPublicKey,
		vapidPrivateKey: vapidPrivateKey,
		httpClient:      &http.Client{Timeout: 10 * time.Second},
		save:            func(rec *core.Record) error { return app.Save(rec) },
		delete:          func(rec *core.Record) error { return app.Delete(rec) },
	}
	n.pushIdle = sync.NewCond(&n.pushMu)
	return n
}

// startPush registers one in-flight push. It reports false once shutdown has
// begun (WaitPushTimeout), when starting a push would outlive the app.
func (n *Notifier) startPush() bool {
	n.pushMu.Lock()
	defer n.pushMu.Unlock()
	if n.pushClosed {
		return false
	}
	n.pushPending++
	return true
}

// endPush marks one in-flight push as finished.
func (n *Notifier) endPush() {
	n.pushMu.Lock()
	defer n.pushMu.Unlock()
	if n.pushPending--; n.pushPending == 0 {
		n.pushIdle.Broadcast()
	}
}

// pendingPushes returns the number of pushes still in flight.
func (n *Notifier) pendingPushes() int {
	n.pushMu.Lock()
	defer n.pushMu.Unlock()
	return n.pushPending
}

// SetHTTPClient overrides the HTTP client used to deliver web push requests.
// Production never calls this (the constructor's 10s-timeout client is
// correct); tests use it to route push sends through a fake RoundTripper
// instead of dialing a subscription's real endpoint host.
func (n *Notifier) SetHTTPClient(c *http.Client) {
	n.httpClient = c
}

// WaitPush blocks until every in-flight push goroutine fired by deliver has
// finished. Tests call it before asserting or before the TestApp closes, so
// a background sendPush can never observe a torn-down app.
func (n *Notifier) WaitPush() {
	n.pushMu.Lock()
	defer n.pushMu.Unlock()
	for n.pushPending > 0 {
		n.pushIdle.Wait()
	}
}

// WaitPushTimeout blocks until every in-flight push goroutine fired by
// deliver has finished, or timeout elapses, whichever comes first. It
// returns the number of pushes still in flight when it returned (0 means
// every push finished in time). Called from hooks.Register's OnTerminate so
// shutdown doesn't hang forever on a stalled webpush.SendNotification call,
// but still gives in-flight pushes a bounded chance to complete instead of
// being cut off immediately.
func (n *Notifier) WaitPushTimeout(timeout time.Duration) int {
	n.pushMu.Lock()
	n.pushClosed = true
	n.pushMu.Unlock()
	done := make(chan struct{})
	go func() {
		n.WaitPush()
		close(done)
	}()
	select {
	case <-done:
		return 0
	case <-time.After(timeout):
		return n.pendingPushes()
	}
}

// PushEnabled reports whether VAPID keys are configured for web push.
func (n *Notifier) PushEnabled() bool {
	return n.vapidPublicKey != "" && n.vapidPrivateKey != ""
}

// NotifyPlayers creates an in-app notification, sends a push, and emails each player.
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
		n.deliver(notifCol, user, notif, "notify player failed")
	}
}

// NotifyAdmins creates an in-app notification, sends a push, and emails each admin user.
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
		n.deliver(notifCol, admin, notif, "notify admin failed")
	}
	return nil
}

// deliver saves the in-app notification record for user and fires its push
// (unless the user has push disabled in notification_prefs). saveFailMsg is
// the slog message logged when the save fails, so callers keep a distinct
// diagnostic per recipient class (player vs admin).
func (n *Notifier) deliver(notifCol *core.Collection, user *core.Record, notif league.Notification, saveFailMsg string) {
	rec := core.NewRecord(notifCol)
	rec.Set("user", user.Id)
	rec.Set("type", notif.Type)
	rec.Set("title", notif.Title)
	rec.Set("body", notif.Prefix+notif.Body)
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
		slog.Error(saveFailMsg, "user", user.Id, "err", err)
	}
	if PushChannelEnabled(user) && n.startPush() {
		go func() {
			defer n.endPush()
			n.sendPush(user.Id, notif.Title, pushBody(notif), link)
		}()
	}
	n.emailNotification(user, notif, link)
}

// pushBody is the push text: the prefixed body, plus the competition name
// when the body does not already mention it (push has no separate
// competition line).
func pushBody(notif league.Notification) string {
	body := notif.Prefix + notif.Body
	if notif.CompName == "" || strings.Contains(body, notif.CompName) {
		return body
	}
	return body + " · " + notif.CompName
}

// emailNotification sends notif as an email to user, gated on SMTP being
// configured, the user having a verified email, and the email channel being
// enabled in their prefs. Callers run it in a goroutine; it does not signal
// completion back.
func (n *Notifier) emailNotification(user *core.Record, notif league.Notification, link string) {
	if !IsMailerConfigured(n.app) || user.Email() == "" {
		return
	}
	if !user.Verified() {
		slog.Info("skip email to unverified user", "to", maskEmail(user.Email()))
		return
	}
	if !EmailChannelEnabled(user) {
		return
	}

	if strings.HasPrefix(link, "/") {
		if baseURL := strings.TrimRight(n.app.Settings().Meta.AppURL, "/"); baseURL != "" {
			link = baseURL + link
		}
	}

	subject := SubjectPrefix() + notif.Title
	displayName := user.GetString("display_name")
	content := emailContent{Body: notif.Prefix + notif.Body, Author: notif.Author, Text: notif.Text, CompName: notif.CompName}
	if notif.MatchID != "" {
		if info, ok := loadMatchInfo(n.app, notif.MatchID); ok {
			content.Match = &info
			content.Body = notif.Body // the context card shows the prefix
		}
	}
	htmlBody := RenderEmail(n.app, "", buildNotificationEmail(displayName, content, link))
	SendEmail(n.app, user.Email(), subject, htmlBody)
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

// EmailChannelEnabled reports whether user has the email channel enabled in
// notification_prefs. Callers must separately check user.Verified() — the
// channel toggle and verification status are independent gates.
func EmailChannelEnabled(user *core.Record) bool {
	return channelEnabled(user, "email")
}

// PushChannelEnabled reports whether user has the push channel enabled in
// notification_prefs.
func PushChannelEnabled(user *core.Record) bool {
	return channelEnabled(user, "push")
}

func channelEnabled(user *core.Record, channel string) bool {
	enabled, ok := NotificationPrefs(user)[channel]
	if !ok {
		return true
	}
	b, ok := enabled.(bool)
	return !ok || b
}

// EventTypes lists every notification event type a user can individually
// toggle (excludes the "email"/"push" delivery channels, which have their
// own verified/subscribed gating). NotificationPrefs defaults and
// handlers.PrefsSave both iterate this list so a new type only needs adding
// here.
var EventTypes = []string{
	"quorum_request", "dispute", "general", "message",
	"scheduling", "match_progress", "admin_message", "user_joined",
	"announcement", "penalty", "payment", "calendar_published",
	"match_reminder", "match_assigned",
}

// NotificationPrefs returns the user's notification preferences with defaults applied.
func NotificationPrefs(user *core.Record) map[string]any {
	defaults := map[string]any{
		"email": true,
		"push":  true,
	}
	for _, t := range EventTypes {
		defaults[t] = true
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
