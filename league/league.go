// Package league implements domain logic for scoring, standings, fixtures, and awards.
package league

import "github.com/pocketbase/pocketbase/core"

// Notifier sends player notifications; implemented by notify.Notifier.
type Notifier interface {
	NotifyPlayers(playerUserIDs []string, notifType, title, body, relatedMatchID string)
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
