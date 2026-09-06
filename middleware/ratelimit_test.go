package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLimiter(max int) (*RateLimiter, *time.Time) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	rl := NewRateLimiter(max, time.Minute)
	rl.now = func() time.Time { return now }
	return rl, &now
}

func TestRateLimiterAllowsMaxThenBlocks(t *testing.T) {
	t.Parallel()
	rl, _ := newTestLimiter(3)
	for i := range 3 {
		assert.True(t, rl.Allow("a"), "hit %d must be allowed", i+1)
	}
	assert.False(t, rl.Allow("a"))
	assert.True(t, rl.Allow("b"), "other keys are independent")
}

func TestRateLimiterUnblocksAfterWindow(t *testing.T) {
	t.Parallel()
	rl, now := newTestLimiter(2)
	require.True(t, rl.Allow("a"))
	*now = now.Add(30 * time.Second)
	require.True(t, rl.Allow("a"))
	require.False(t, rl.Allow("a"))

	*now = now.Add(30*time.Second + time.Millisecond) // first hit just expired
	assert.True(t, rl.Allow("a"))
	assert.False(t, rl.Allow("a"), "second hit still inside the window")
}

func TestRateLimiterRejectedHitsAreNotCounted(t *testing.T) {
	t.Parallel()
	rl, now := newTestLimiter(1)
	require.True(t, rl.Allow("a"))
	for range 5 {
		require.False(t, rl.Allow("a"))
	}
	*now = now.Add(time.Minute + time.Millisecond)
	assert.True(t, rl.Allow("a"), "hammering while blocked must not extend the block")
}

func TestRateLimiterSweepDropsExpiredKeys(t *testing.T) {
	t.Parallel()
	rl, now := newTestLimiter(5)
	require.True(t, rl.Allow("stale"))
	*now = now.Add(2 * time.Minute)
	require.True(t, rl.Allow("fresh"))
	rl.mu.Lock()
	defer rl.mu.Unlock()
	assert.NotContains(t, rl.hits, "stale")
	assert.Contains(t, rl.hits, "fresh")
}

func TestLimitByClientIP(t *testing.T) {
	t.Parallel()
	rl, _ := newTestLimiter(1)
	mw := LimitByClientIP(rl, "Demasiados intentos.")

	call := func(ip string, htmx bool) (*httptest.ResponseRecorder, bool) {
		req := httptest.NewRequest("POST", "/login", nil)
		req.Header.Set("X-Forwarded-For", ip)
		if htmx {
			req.Header.Set("HX-Request", "true")
		}
		rec := httptest.NewRecorder()
		e := &core.RequestEvent{}
		e.Request = req
		e.Response = rec
		require.NoError(t, mw(e))
		// Pass-through (e.Next with no chained handler) writes nothing;
		// a block writes the alert fragment.
		return rec, rec.Body.Len() == 0
	}

	_, next := call("203.0.113.1", true)
	assert.True(t, next, "first hit passes through")

	rec, next := call("203.0.113.1", true)
	assert.False(t, next, "second hit is blocked")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "#flash", rec.Header().Get("HX-Retarget"))
	assert.Equal(t, "innerHTML", rec.Header().Get("HX-Reswap"))
	assert.Equal(t, `<div class="alert-error text-sm py-2">Demasiados intentos.</div>`, rec.Body.String())

	rec, next = call("203.0.113.1", false)
	assert.False(t, next)
	assert.Empty(t, rec.Header().Get("HX-Retarget"), "non-htmx response carries no HX headers")

	_, next = call("203.0.113.2", true)
	assert.True(t, next, "a different client IP has its own budget")
}
