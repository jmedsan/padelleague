package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("VAPID_PUBLIC_KEY", "")
	t.Setenv("VAPID_PRIVATE_KEY", "")
	t.Setenv("PB_ADMIN_EMAIL", "")
	t.Setenv("PB_ADMIN_PASSWORD", "")
	t.Setenv("APP_ADMIN1_EMAIL", "")
	t.Setenv("APP_ADMIN1_PASSWORD", "")
	t.Setenv("APP_ADMIN2_EMAIL", "")
	t.Setenv("APP_ADMIN2_PASSWORD", "")
	t.Setenv("APP_PLAYER_EMAIL", "")
	t.Setenv("APP_PLAYER_PASSWORD", "")
	t.Setenv("APP_PLAYER2_EMAIL", "")
	t.Setenv("APP_PLAYER2_PASSWORD", "")
	t.Setenv("APP_ENV", "")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "dev", cfg.AppEnv, "AppEnv should default to dev")
	assert.Empty(t, cfg.VAPIDPublicKey)
	assert.Empty(t, cfg.PBAdminEmail)
}

func TestLoad_CustomValues(t *testing.T) {
	t.Setenv("VAPID_PUBLIC_KEY", "test-pub")
	t.Setenv("VAPID_PRIVATE_KEY", "test-priv")
	t.Setenv("PB_ADMIN_EMAIL", "pb@test.local")
	t.Setenv("PB_ADMIN_PASSWORD", "secret")
	t.Setenv("APP_ENV", "production")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "test-pub", cfg.VAPIDPublicKey)
	assert.Equal(t, "test-priv", cfg.VAPIDPrivateKey)
	assert.Equal(t, "pb@test.local", cfg.PBAdminEmail)
	assert.Equal(t, "secret", cfg.PBAdminPassword)
	assert.Equal(t, "production", cfg.AppEnv)
}

func TestLoad_BothAdmins(t *testing.T) {
	t.Setenv("APP_ADMIN1_EMAIL", "admin1@test.local")
	t.Setenv("APP_ADMIN1_PASSWORD", "pass1")
	t.Setenv("APP_ADMIN2_EMAIL", "admin2@test.local")
	t.Setenv("APP_ADMIN2_PASSWORD", "pass2")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "admin1@test.local", cfg.AppAdmin1Email)
	assert.Equal(t, "pass1", cfg.AppAdmin1Password)
	assert.Equal(t, "admin2@test.local", cfg.AppAdmin2Email)
	assert.Equal(t, "pass2", cfg.AppAdmin2Password)
}
