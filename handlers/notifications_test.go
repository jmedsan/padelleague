package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationCount_UnreadOnly(t *testing.T) {
	app := newTestApp(t)
	user := makeUser(t, app, "Notif User", "")

	makeNotification(t, app, user.Id, "Unread 1", "body1", false)
	makeNotification(t, app, user.Id, "Unread 2", "body2", false)
	makeNotification(t, app, user.Id, "Read 1", "body3", true)

	unread, err := app.FindRecordsByFilter("notifications",
		"user = {:uid} && read = false",
		"", 0, 0,
		map[string]any{"uid": user.Id})
	require.NoError(t, err)
	assert.Equal(t, 2, len(unread))
}

func TestNotificationList_OrderAndLimit(t *testing.T) {
	app := newTestApp(t)
	user := makeUser(t, app, "List User", "")

	for i := 0; i < 12; i++ {
		makeNotification(t, app, user.Id, "Notif", "body", false)
	}

	records, err := app.FindRecordsByFilter("notifications",
		"user = {:uid}",
		"", 10, 0,
		map[string]any{"uid": user.Id})
	require.NoError(t, err)
	assert.Equal(t, 10, len(records), "should return at most 10")
}

func TestNotificationCount_OtherUserExcluded(t *testing.T) {
	app := newTestApp(t)
	user1 := makeUser(t, app, "User 1", "")
	user2 := makeUser(t, app, "User 2", "")

	makeNotification(t, app, user1.Id, "For user1", "body", false)
	makeNotification(t, app, user2.Id, "For user2", "body", false)

	unread, err := app.FindRecordsByFilter("notifications",
		"user = {:uid} && read = false",
		"", 0, 0,
		map[string]any{"uid": user1.Id})
	require.NoError(t, err)
	assert.Equal(t, 1, len(unread))
}
