package config

import (
	"os"

	"github.com/airport-panel/config"
)

const (
	ServiceName = "traffic-service"
	DefaultPort = "8084"
)

type ServiceConfig struct {
	*config.Config
	Port              string
	AgentAPITokenSalt string
	HMACSecret        string
}

func Load() *ServiceConfig {
	shared := config.Load()
	port := os.Getenv("TRAFFIC_SERVICE_PORT")
	if port == "" {
		port = DefaultPort
	}
	agentSalt := os.Getenv("AGENT_API_TOKEN_SALT")
	if agentSalt == "" {
		agentSalt = "node-agent-default-salt-change-me"
	}
	hmacSecret := os.Getenv("HMAC_SECRET")
	if hmacSecret == "" {
		hmacSecret = "node-agent-hmac-default-secret-change-me"
	}
	return &ServiceConfig{
		Config:            shared,
		Port:              port,
		AgentAPITokenSalt: agentSalt,
		HMACSecret:        hmacSecret,
	}
}
