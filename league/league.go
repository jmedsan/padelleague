// Package league implements domain logic for scoring, standings, fixtures, and awards.
package league

import (
	"math/rand/v2"
	"sync"

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

	topUpLocks   map[string]*sync.Mutex // competition id -> lock serializing TopUpAssignments
	topUpLocksMu sync.Mutex             // guards topUpLocks itself
}

// New creates a Service with the given PocketBase app and notifier.
func New(app core.App, notifier Notifier) *Service {
	return &Service{
		app:        app,
		notifier:   notifier,
		shuffle:    rand.Shuffle,
		topUpLocks: make(map[string]*sync.Mutex),
	}
}

// lockTopUp returns the mutex serializing TopUpAssignments calls for compID,
// creating one on first use. TopUpAssignments reads state, plans in memory,
// and saves in a separate transaction; two calls for the same competition
// racing between the read and the save can each act on the same stale
// snapshot, so every call must hold this lock for its full read-plan-save
// sequence.
func (svc *Service) lockTopUp(compID string) *sync.Mutex {
	svc.topUpLocksMu.Lock()
	defer svc.topUpLocksMu.Unlock()
	l, ok := svc.topUpLocks[compID]
	if !ok {
		l = &sync.Mutex{}
		svc.topUpLocks[compID] = l
	}
	return l
}

// SetShuffle replaces the shuffle seam. Tests call this to get deterministic
// leveled-assignment ordering; production code always uses rand.Shuffle.
func (s *Service) SetShuffle(fn func(n int, swap func(i, j int))) {
	s.shuffle = fn
}
