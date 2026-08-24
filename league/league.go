// Package league implements domain logic for scoring, standings, fixtures, and awards.
package league

import "github.com/pocketbase/pocketbase/core"

type Notifier interface {
	NotifyPlayers(playerUserIDs []string, notifType, title, body, relatedMatchID string)
}

type Service struct {
	app      core.App
	notifier Notifier
}

func New(app core.App, notifier Notifier) *Service {
	return &Service{app: app, notifier: notifier}
}
