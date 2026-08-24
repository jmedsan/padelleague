package handlers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIsInviteExpired_Expired(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	invite := makeInvitation(t, app, time.Now().Add(-1*time.Hour))
	assert.True(t, isInviteExpired(invite))
}

func TestIsInviteExpired_Valid(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	invite := makeInvitation(t, app, time.Now().Add(1*time.Hour))
	assert.False(t, isInviteExpired(invite))
}

func TestIsInviteExpired_ZeroDate(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	invite := makeInvitation(t, app, time.Time{})
	assert.True(t, isInviteExpired(invite), "zero date should be treated as expired")
}
