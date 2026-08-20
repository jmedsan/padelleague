package handlers

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

func TestGenerateInviteToken(t *testing.T) {
	token, err := generateInviteToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(token) != 32 {
		t.Errorf("expected 32-char token, got %d chars: %s", len(token), token)
	}
	if _, err := hex.DecodeString(token); err != nil {
		t.Errorf("token is not valid hex: %s", token)
	}

	token2, _ := generateInviteToken()
	if token == token2 {
		t.Error("two generated tokens should not be equal")
	}
}

func TestIsInviteExpired(t *testing.T) {
	col := &core.Collection{}
	col.Fields.Add(
		&core.DateField{Name: "expires_at"},
	)

	expired := core.NewRecord(col)
	expired.Set("expires_at", time.Now().Add(-1*time.Hour).UTC().Format(time.RFC3339))
	if !isInviteExpired(expired) {
		t.Error("expected expired invite to return true")
	}

	valid := core.NewRecord(col)
	valid.Set("expires_at", time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339))
	if isInviteExpired(valid) {
		t.Error("expected valid invite to return false")
	}
}
