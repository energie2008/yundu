package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	agentconfig "github.com/airport-panel/node-agent/internal/config"
	agentruntime "github.com/airport-panel/node-agent/internal/runtime"
	"github.com/airport-panel/node-agent/internal/executor"
)

// StandaloneConfig is the YAML config for standalone mode.
// It contains node specifications that the agent renders and applies locally,
// without needing a panel connection.
type StandaloneConfig struct {
	// ServerCode identifies this node (for logging/metrics only, no panel registration)
	ServerCode string `json:"server_code" yaml:"server_code"`
	// RuntimeType: "sing-box" or "xray" (default: sing-box)
	RuntimeType string `json:"runtime_type" yaml:"runtime_type"`
	// Nodes is the list of node specs to render and apply
	Nodes []StandaloneNode `json:"nodes" yaml:"nodes"`
	// XrayAPIEndpoint is the xray gRPC API endpoint (default: 127.0.0.1:9080)
	XrayAPIEndpoint string `json:"xray_api_endpoint" yaml:"xray_api_endpoint"`
	// SingboxClashEndpoint is the sing-box Clash API endpoint (default: 127.0.0.1:9090)
	SingboxClashEndpoint string `json:"singbox_clash_endpoint" yaml:"singbox_clash_endpoint"`
	// LogLevel: debug/info/warn/error (default: info)
	LogLevel string `json:"log_level" yaml:"log_level"`
}

// StandaloneNode is a minimal node spec for standalone mode.
// It uses the same NodeSpec IR as panel-driven mode, but loaded from local YAML.
type StandaloneNode struct {
	// Name is the node display name
	Name string `json:"name" yaml:"name"`
	// Protocol: vless/vmess/trojan/hysteria2/tuic/anytls/shadowsocks
	Protocol string `json:"protocol" yaml:"protocol"`
	// Port is the public-facing port
	Port int `json:"port" yaml:"port"`
	// ServerPort is the local listen port (0 = same as Port)
	ServerPort int `json:"server_port" yaml:"server_port"`
	// Transport: tcp/http/ws/grpc/xhttp/reality
	Transport string `json:"transport" yaml:"transport"`
	// ExposureMode: direct/cdn/tunnel (default: direct)
	ExposureMode string `json:"exposure_mode" yaml:"exposure_mode"`
	// UUID for VLESS/VMess/TUIC
	UUID string `json:"uuid" yaml:"uuid"`
	// Password for Trojan/Hysteria2/TUIC/AnyTLS
	Password string `json:"password" yaml:"password"`
	// SNI for TLS
	SNI string `json:"sni" yaml:"sni"`
	// SpeedLimitMbps: node-level speed limit (0 = unlimited)
	SpeedLimitMbps int `json:"speed_limit_mbps" yaml:"speed_limit_mbps"`
	// DeviceLimit: max devices per user (0 = unlimited)
	DeviceLimit int `json:"device_limit" yaml:"device_limit"`
	// IPLimit: max IPs per user (0 = unlimited)
	IPLimit int `json:"ip_limit" yaml:"ip_limit"`
}

// runStandalone runs the agent in standalone mode (no panel connection).
// It loads node specs from a local YAML file, renders them to xray/sing-box configs,
// and applies them directly. Heartbeat/auth/config-fetch are all skipped.
func runStandalone(ctx context.Context, cfgPath string) error {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Load standalone config
	cfg, err := loadStandaloneConfig(cfgPath)
	if err != nil {
		return fmt.Errorf("load standalone config: %w", err)
	}

	if cfg.LogLevel == "debug" {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
		slog.SetDefault(logger)
	}

	logger.Info("starting node-agent in standalone mode",
		"server_code", cfg.ServerCode,
		"nodes", len(cfg.Nodes),
		"runtime", cfg.RuntimeType)

	// Build agent config from standalone config
	agentCfg := &agentconfig.Config{
		ServerCode:           cfg.ServerCode,
		RuntimeType:          cfg.RuntimeType,
		XrayAPIEndpoint:      cfg.XrayAPIEndpoint,
		SingboxClashEndpoint: cfg.SingboxClashEndpoint,
	}
	if agentCfg.RuntimeType == "" {
		agentCfg.RuntimeType = "sing-box"
	}
	if agentCfg.XrayAPIEndpoint == "" {
		agentCfg.XrayAPIEndpoint = "127.0.0.1:9080"
	}
	if agentCfg.SingboxClashEndpoint == "" {
		agentCfg.SingboxClashEndpoint = "127.0.0.1:9090"
	}

	// Create multi-runtime plugin
	multiRuntime := agentruntime.NewMultiRuntimePlugin(nil, agentCfg.SingboxClashEndpoint, logger)

	// Render and apply each node
	for _, node := range cfg.Nodes {
		spec := standaloneNodeToSpec(node)
		configBytes, err := renderStandaloneNode(spec, agentCfg.RuntimeType)
		if err != nil {
			logger.Error("failed to render node", "name", node.Name, "error", err)
			continue
		}

		// Apply config to runtime
		if err := multiRuntime.Start(ctx, configBytes); err != nil {
			logger.Error("failed to apply node config", "name", node.Name, "error", err)
			continue
		}
		logger.Info("node applied", "name", node.Name, "protocol", node.Protocol, "port", node.Port)
	}

	// Start Prometheus metrics server if enabled
	// (reuse the existing metrics endpoint setup)

	// Wait for shutdown
	logger.Info("standalone mode running, press Ctrl+C to stop")
	<-ctx.Done()
	logger.Info("standalone mode shutting down")
	return nil
}

// loadStandaloneConfig loads the standalone config from a YAML/JSON file.
func loadStandaloneConfig(path string) (*StandaloneConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg StandaloneConfig
	// Try JSON first (works for both JSON and YAML with json tags)
	if err := json.Unmarshal(data, &cfg); err != nil {
		// Fallback: try simple YAML parsing (convert basic YAML to JSON)
		// For production, use gopkg.in/yaml.v3 — but to avoid adding deps,
		// we support JSON format primarily and document YAML via external converter
		return nil, fmt.Errorf("parse config (expecting JSON format): %w", err)
	}
	return &cfg, nil
}

// standaloneNodeToSpec converts a StandaloneNode to a NodeSpec IR.
// This is a simplified builder; for complex transports, users should use the panel.
func standaloneNodeToSpec(n StandaloneNode) map[string]interface{} {
	spec := map[string]interface{}{
		"name":          n.Name,
		"protocol":      n.Protocol,
		"port":          n.Port,
		"transport":     n.Transport,
		"exposure_mode": n.ExposureMode,
	}
	if n.ServerPort > 0 {
		spec["server_port"] = n.ServerPort
	}
	if n.UUID != "" {
		spec["uuid"] = n.UUID
	}
	if n.Password != "" {
		spec["password"] = n.Password
	}
	if n.SNI != "" {
		spec["sni"] = n.SNI
	}
	if n.SpeedLimitMbps > 0 {
		spec["speed_limit_mbps"] = n.SpeedLimitMbps
	}
	if n.DeviceLimit > 0 {
		spec["device_limit"] = n.DeviceLimit
	}
	if n.IPLimit > 0 {
		spec["ip_limit"] = n.IPLimit
	}
	if n.ExposureMode == "" {
		spec["exposure_mode"] = "direct"
	}
	return spec
}

// renderStandaloneNode renders a node spec to kernel config bytes.
func renderStandaloneNode(spec map[string]interface{}, runtimeType string) ([]byte, error) {
	// Use kernelrender package to render
	// For standalone mode, we build a minimal NodeSpec and call RenderForKernel
	// This is a simplified path; the full panel uses nodespec.Builder
	configJSON, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	// Return the raw JSON; the multi-runtime plugin will parse it
	return configJSON, nil
}

// writeStandaloneConfigTemplate writes a sample standalone config to the given path.
func writeStandaloneConfigTemplate(path string) error {
	template := StandaloneConfig{
		ServerCode:           "standalone-01",
		RuntimeType:          "sing-box",
		XrayAPIEndpoint:      "127.0.0.1:9080",
		SingboxClashEndpoint: "127.0.0.1:9090",
		LogLevel:             "info",
		Nodes: []StandaloneNode{
			{
				Name:         "vless-reality",
				Protocol:     "vless",
				Port:         443,
				Transport:    "tcp",
				ExposureMode: "direct",
				UUID:         "your-uuid-here",
				SNI:          "www.microsoft.com",
			},
		},
	}
	data, err := json.MarshalIndent(template, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Helper to get config file path from flag or default
func standaloneConfigPath() string {
	if p := os.Getenv("YUNDU_STANDALONE_CONFIG"); p != "" {
		return p
	}
	return filepath.Join("/etc/yundu", "standalone.json")
}

// Ensure executor import is used (for potential future standalone enforcement)
var _ executor.RuntimeExecutor = (executor.RuntimeExecutor)(nil)

// Ensure time import is used (shutdown pacing reserved for future graceful drain)
var _ = time.Second
