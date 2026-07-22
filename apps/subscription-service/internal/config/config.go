package config

import (
	"os"

	"github.com/airport-panel/config"
)

const (
	ServiceName = "subscription-service"
	DefaultPort = "8083"
)

type ServiceConfig struct {
	*config.Config
	Port              string
	AgentAPITokenSalt string
}

func Load() *ServiceConfig {
	shared := config.Load()
	port := os.Getenv("SUBSCRIPTION_SERVICE_PORT")
	if port == "" {
		port = DefaultPort
	}
	agentSalt := os.Getenv("AGENT_API_TOKEN_SALT")
	if agentSalt == "" {
		agentSalt = "node-agent-default-salt-change-me"
	}
	return &ServiceConfig{
		Config:            shared,
		Port:              port,
		AgentAPITokenSalt: agentSalt,
	}
}
