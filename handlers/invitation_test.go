package handlers

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateInviteToken(t *testing.T) {
	token, err := generateInviteToken()
	require.NoError(t, err)
	assert.Len(t, token, 32)
	_, err = hex.DecodeString(token)
	assert.NoError(t, err)

	token2, _ := generateInviteToken()
	assert.NotEqual(t, token, token2)
}

func TestIsInviteExpired(t *testing.T) {
	col := &core.Collection{}
	col.Fields.Add(
		&core.DateField{Name: "expires_at"},
	)

	expired := core.NewRecord(col)
	expired.Set("expires_at", time.Now().Add(-1*time.Hour).UTC().Format(time.RFC3339))
	assert.True(t, isInviteExpired(expired))

	valid := core.NewRecord(col)
	valid.Set("expires_at", time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339))
	assert.False(t, isInviteExpired(valid))
}

func TestInvitationMaxUses(t *testing.T) {
	app := newTestApp(t)

	admin := makeUser(t, app, "Admin", "")

	col, err := app.FindCollectionByNameOrId("invitations")
	require.NoError(t, err)

	invite := core.NewRecord(col)
	invite.Set("token", "abcdef1234567890abcdef1234567890")
	invite.Set("max_uses", 2)
	invite.Set("use_count", 0)
	invite.Set("status", "pending")
	invite.Set("created_by", admin.Id)
	invite.Set("expires_at", time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339))
	require.NoError(t, app.Save(invite))

	for i := 0; i < 2; i++ {
		fresh, err := app.FindRecordById("invitations", invite.Id)
		require.NoError(t, err)

		maxUses := int(fresh.GetFloat("max_uses"))
		if maxUses < 1 {
			maxUses = 1
		}
		useCount := int(fresh.GetFloat("use_count"))
		require.Less(t, useCount, maxUses, "use %d should be allowed", i+1)

		fresh.Set("use_count", useCount+1)
		if useCount+1 >= maxUses {
			fresh.Set("status", "used")
		}
		require.NoError(t, app.Save(fresh))
	}

	fresh, err := app.FindRecordById("invitations", invite.Id)
	require.NoError(t, err)
	assert.Equal(t, 2, int(fresh.GetFloat("use_count")))
	assert.Equal(t, "used", fresh.GetString("status"))

	maxUses := int(fresh.GetFloat("max_uses"))
	if maxUses < 1 {
		maxUses = 1
	}
	useCount := int(fresh.GetFloat("use_count"))
	assert.GreaterOrEqual(t, useCount, maxUses, "third use should be rejected")
}
