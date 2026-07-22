package config

import (
	"os"

	"github.com/airport-panel/config"
)

const (
	ServiceName    = "node-service"
	DefaultPort    = "8082"
	DefaultGRPCPort = "9082"
)

type ServiceConfig struct {
	*config.Config
	Port              string
	GRPCPort          string
	AgentAPITokenSalt string
	HMACSecret        string
	PublicURL         string
	TLSCertFile       string
	TLSKeyFile        string
}

func Load() *ServiceConfig {
	shared := config.Load()
	port := os.Getenv("NODE_SERVICE_PORT")
	if port == "" {
		port = DefaultPort
	}
	grpcPort := os.Getenv("NODE_SERVICE_GRPC_PORT")
	if grpcPort == "" {
		grpcPort = DefaultGRPCPort
	}
	agentSalt := os.Getenv("AGENT_API_TOKEN_SALT")
	if agentSalt == "" {
		agentSalt = "node-agent-default-salt-change-me"
	}
	hmacSecret := os.Getenv("HMAC_SECRET")
	if hmacSecret == "" {
		hmacSecret = "node-agent-hmac-default-secret-change-me"
	}
	publicURL := os.Getenv("PUBLIC_URL")
	if publicURL == "" {
		publicURL = "http://localhost:8080"
	}
	return &ServiceConfig{
		Config:            shared,
		Port:              port,
		GRPCPort:          grpcPort,
		AgentAPITokenSalt: agentSalt,
		HMACSecret:        hmacSecret,
		PublicURL:         publicURL,
		TLSCertFile:       os.Getenv("TLS_CERT_FILE"),
		TLSKeyFile:        os.Getenv("TLS_KEY_FILE"),
	}
}
