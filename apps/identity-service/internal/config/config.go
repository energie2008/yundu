package config

import (
	"os"

	"github.com/airport-panel/config"
)

const (
	ServiceName = "identity-service"
	DefaultPort = "8081"
)

type ServiceConfig struct {
	*config.Config
	Port       string
	Argon2Salt string
}

func Load() *ServiceConfig {
	shared := config.Load()
	port := os.Getenv("IDENTITY_SERVICE_PORT")
	if port == "" {
		port = DefaultPort
	}
	argon2Salt := os.Getenv("ARGON2_SALT")
	return &ServiceConfig{
		Config:     shared,
		Port:       port,
		Argon2Salt: argon2Salt,
	}
}
