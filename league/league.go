// Package league implements domain logic for scoring, standings, fixtures, and awards.
package league

import "github.com/pocketbase/pocketbase/core"

// Notification bundles the fields for a player notification.
type Notification struct {
	Type    string
	Title   string
	Body    string
	MatchID string
}

// Notifier sends player notifications; implemented by notify.Notifier.
type Notifier interface {
	NotifyPlayers(playerUserIDs []string, n Notification)
	EmailPlayers(playerUserIDs []string, subject, body, link string)
}

// Service provides domain operations for competitions, matches, and standings.
type Service struct {
	app      core.App
	notifier Notifier
}

// New creates a Service with the given PocketBase app and notifier.
func New(app core.App, notifier Notifier) *Service {
	return &Service{app: app, notifier: notifier}
}
