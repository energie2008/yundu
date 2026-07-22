package config

import (
	"os"

	"github.com/airport-panel/config"
)

const (
	ServiceName = "api-gateway"
	DefaultPort = "8080"
)

type ServiceConfig struct {
	*config.Config
	Port             string
	IdentityAddr     string
	NodeAddr         string
	SubscriptionAddr string
	TrafficAddr      string
	JWTSecret        string
}

func Load() *ServiceConfig {
	shared := config.Load()
	port := os.Getenv("API_GATEWAY_PORT")
	if port == "" {
		port = DefaultPort
	}
	return &ServiceConfig{
		Config:           shared,
		Port:             port,
		IdentityAddr:     getEnv("IDENTITY_SERVICE_ADDR", "localhost:8081"),
		NodeAddr:         getEnv("NODE_SERVICE_ADDR", "localhost:8082"),
		SubscriptionAddr: getEnv("SUBSCRIPTION_SERVICE_ADDR", "localhost:8083"),
		TrafficAddr:      getEnv("TRAFFIC_SERVICE_ADDR", "localhost:8084"),
		JWTSecret:        os.Getenv("JWT_SECRET"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
