// Package league implements domain logic for scoring, standings, fixtures, and awards.
package league

import (
	"math/rand/v2"

	"github.com/pocketbase/pocketbase/core"
)

// Notification bundles the fields for a player notification.
type Notification struct {
	Type     string
	Title    string
	Body     string
	MatchID  string
	Link     string // overrides the "/match/{MatchID}" default when set
	CompName string // competition display name, rendered as a secondary line
}

// Notifier sends player notifications; implemented by notify.Notifier.
type Notifier interface {
	NotifyPlayers(playerUserIDs []string, n Notification)
}

// Service provides domain operations for competitions, matches, and standings.
type Service struct {
	app      core.App
	notifier Notifier

	// Test seam: initialized to rand.Shuffle in New; tests override per instance.
	shuffle func(n int, swap func(i, j int))
}

// New creates a Service with the given PocketBase app and notifier.
func New(app core.App, notifier Notifier) *Service {
	return &Service{
		app:      app,
		notifier: notifier,
		shuffle:  rand.Shuffle,
	}
}
