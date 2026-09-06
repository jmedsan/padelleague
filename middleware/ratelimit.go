package middleware

import (
	"html"
	"net/http"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// RateLimiter is an in-process sliding-window counter keyed by an arbitrary
// string (typically a client IP). Safe for concurrent use.
type RateLimiter struct {
	max    int
	window time.Duration

	mu        sync.Mutex
	hits      map[string][]time.Time
	lastSweep time.Time
	now       func() time.Time // test seam
}

// NewRateLimiter allows at most max hits per key within window.
func NewRateLimiter(max int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		max:    max,
		window: window,
		hits:   make(map[string][]time.Time),
		now:    time.Now,
	}
}

// Allow records a hit for key and reports whether it stays within budget.
// A rejected hit is not recorded, so a blocked client is unblocked once its
// earlier hits age out of the window.
func (rl *RateLimiter) Allow(key string) bool {
	now := rl.now()
	cutoff := now.Add(-rl.window)

	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.sweep(now, cutoff)

	fresh := pruneBefore(rl.hits[key], cutoff)
	if len(fresh) >= rl.max {
		rl.hits[key] = fresh
		return false
	}
	rl.hits[key] = append(fresh, now)
	return true
}

// sweep drops keys whose hits have all expired, at most once per window, so
// the map does not accumulate one entry per IP ever seen. Caller holds mu.
func (rl *RateLimiter) sweep(now, cutoff time.Time) {
	if now.Sub(rl.lastSweep) < rl.window {
		return
	}
	rl.lastSweep = now
	for key, times := range rl.hits {
		if fresh := pruneBefore(times, cutoff); len(fresh) == 0 {
			delete(rl.hits, key)
		} else {
			rl.hits[key] = fresh
		}
	}
}

// LimitByClientIP returns a route middleware that keys rl by ClientIP and
// answers with blockedMsg as a flash alert once the budget is exceeded.
func LimitByClientIP(rl *RateLimiter, blockedMsg string) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if !rl.Allow(ClientIP(e)) {
			return flashAlertError(e, blockedMsg)
		}
		return e.Next()
	}
}

// pruneBefore keeps the times after cutoff, reusing the backing array.
func pruneBefore(times []time.Time, cutoff time.Time) []time.Time {
	fresh := times[:0]
	for _, t := range times {
		if t.After(cutoff) {
			fresh = append(fresh, t)
		}
	}
	return fresh
}

// flashAlertError mirrors handlers.alertError (same fragment and HX headers);
// duplicated because middleware cannot import handlers.
func flashAlertError(e *core.RequestEvent, msg string) error {
	fragment := `<div class="alert-error text-sm py-2">` + html.EscapeString(msg) + `</div>`
	if e.Request.Header.Get("HX-Request") == "true" {
		e.Response.Header().Set("HX-Retarget", "#flash")
		e.Response.Header().Set("HX-Reswap", "innerHTML")
	}
	return e.HTML(http.StatusOK, fragment)
}
