package league

import "github.com/pocketbase/pocketbase/core"

type Notifier interface {
	NotifyPlayers(playerUserIDs []string, notifType, title, body, relatedMatchID string)
}

type Service struct {
	App      core.App
	Notifier Notifier
}

func New(app core.App, notifier Notifier) *Service {
	return &Service{App: app, Notifier: notifier}
}
