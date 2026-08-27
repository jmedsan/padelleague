// Package config provides environment-based configuration for the application.
package config

import "github.com/caarlos0/env/v11"

// Config holds all environment-parsed settings for the application.
type Config struct {
	VAPIDPublicKey  string `env:"VAPID_PUBLIC_KEY"`
	VAPIDPrivateKey string `env:"VAPID_PRIVATE_KEY"`

	PBAdminEmail    string `env:"PB_ADMIN_EMAIL"`
	PBAdminPassword string `env:"PB_ADMIN_PASSWORD"`

	AppAdmin1Email    string `env:"APP_ADMIN1_EMAIL"`
	AppAdmin1Password string `env:"APP_ADMIN1_PASSWORD"`
	AppAdmin1Name     string `env:"APP_ADMIN1_NAME" envDefault:"Admin"`

	AppAdmin2Email    string `env:"APP_ADMIN2_EMAIL"`
	AppAdmin2Password string `env:"APP_ADMIN2_PASSWORD"`
	AppAdmin2Name     string `env:"APP_ADMIN2_NAME" envDefault:"Admin 2"`

	AppPlayerEmail    string `env:"APP_PLAYER_EMAIL"`
	AppPlayerPassword string `env:"APP_PLAYER_PASSWORD"`
	AppPlayerName     string `env:"APP_PLAYER_NAME" envDefault:"Jugador"`

	AppPlayer2Email    string `env:"APP_PLAYER2_EMAIL"`
	AppPlayer2Password string `env:"APP_PLAYER2_PASSWORD"`
	AppPlayer2Name     string `env:"APP_PLAYER2_NAME" envDefault:"Jugador 2"`

	AppEnv      string `env:"APP_ENV" envDefault:"dev"`
	AppDevTools bool   `env:"APP_DEV_TOOLS" envDefault:"false"`
}

// Load parses environment variables into a Config struct.
func Load() (Config, error) {
	return env.ParseAs[Config]()
}
