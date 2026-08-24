// Package config provides environment-based configuration for the application.
package config

import "github.com/caarlos0/env/v11"

type Config struct {
	VAPIDPublicKey  string `env:"VAPID_PUBLIC_KEY"`
	VAPIDPrivateKey string `env:"VAPID_PRIVATE_KEY"`

	PBAdminEmail    string `env:"PB_ADMIN_EMAIL"`
	PBAdminPassword string `env:"PB_ADMIN_PASSWORD"`

	AppAdminEmail    string `env:"APP_ADMIN_EMAIL"`
	AppAdminPassword string `env:"APP_ADMIN_PASSWORD"`

	AppPlayerEmail    string `env:"APP_PLAYER_EMAIL"`
	AppPlayerPassword string `env:"APP_PLAYER_PASSWORD"`

	AppPlayer2Email    string `env:"APP_PLAYER2_EMAIL"`
	AppPlayer2Password string `env:"APP_PLAYER2_PASSWORD"`

	AppEnv string `env:"APP_ENV" envDefault:"dev"`
}

func Load() (Config, error) {
	return env.ParseAs[Config]()
}
