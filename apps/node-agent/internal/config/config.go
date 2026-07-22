package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

type Config struct {
	PanelURL          string
	ServerCode        string
	AgentToken        string
	AgentAPITokenSalt string
	HMACSecret        string
	GRPCAddr          string
	RuntimeType       string
	ListenHost        string
	ListenPort        int
	ConfigDir         string
	LogDir            string
	LogLevel          string
	RuntimePath       string
	WARPMode          string
	WARPServer        string
	WARPService       string
	WARPToken         string
	WARPInterval      int
	WARPParent        string
	// BootstrapEnabled 标识当前配置是否来自 bootstrap 模式（CLI flags + 面板下发）。
	// 为 true 时表示 Agent 通过 --endpoint/--token 零配置启动，配置优先级为：
	// CLI flags > 面板下发 (bootstrap) > 环境变量 > 默认值。
	BootstrapEnabled bool

	// XrayAPIEndpoint 是 Machine 模式下分配的 xray gRPC API 端点（由 Orchestrator 填充）。
	// Node 模式为空，走默认值 127.0.0.1:10085。
	XrayAPIEndpoint string
	// SingboxClashEndpoint 是 Machine 模式下分配的 sing-box Clash API 端点（由 Orchestrator 填充）。
	// Node 模式为空，走默认值。
	SingboxClashEndpoint string
}

func Load() *Config {
	cfg := &Config{
		PanelURL:          getEnv("PANEL_URL", ""),
		ServerCode:        getEnv("SERVER_CODE", ""),
		AgentToken:        getEnv("AGENT_TOKEN", ""),
		AgentAPITokenSalt: getEnv("AGENT_API_TOKEN_SALT", "node-agent-default-salt-change-me"),
		HMACSecret:        getEnv("HMAC_SECRET", "node-agent-hmac-default-secret-change-me"),
		GRPCAddr:          getEnv("GRPC_ADDR", ""),
		RuntimeType:       getEnv("RUNTIME_TYPE", "xray"),
		ListenHost:        getEnv("LISTEN_HOST", "0.0.0.0"),
		ListenPort:        getEnvInt("LISTEN_PORT", 10000),
		ConfigDir:         getEnv("CONFIG_DIR", "/etc/yundu"),
		LogDir:            getEnv("LOG_DIR", "/var/log/yundu"),
		LogLevel:          getEnv("LOG_LEVEL", "info"),
		RuntimePath:       getEnv("RUNTIME_PATH", ""),
		WARPMode:          getEnv("WARP_MODE", "mock"),
		WARPServer:        getEnv("WARP_SERVER", "0.0.0.0:8787"),
		WARPService:       getEnv("WARP_SERVICE", ""),
		WARPToken:         getEnv("WARP_TOKEN", ""),
		WARPInterval:      getEnvInt("WARP_INTERVAL", 10),
		WARPParent:        getEnv("WARP_PARENT", ""),
	}

	cfg.PanelURL = strings.TrimRight(cfg.PanelURL, "/")
	return cfg
}

// CLIFlags 保存从命令行解析的原始 flag 值。
type CLIFlags struct {
	Endpoint  string // --endpoint / -e: 面板 URL
	Token     string // --token / -t: Agent token
	Runtime   string // --runtime / -r: xray / sing-box
	ConfigDir string // --config-dir / -c: 配置目录
}

// ParseCLIFlags 解析命令行参数，返回 CLIFlags。
// 支持 --endpoint/-e、--token/-t、--runtime/-r、--config-dir/-c。
// 解析后 flag.CommandLine 已消费完所有参数，不影响后续逻辑。
func ParseCLIFlags() *CLIFlags {
	flags := &CLIFlags{}
	// 同时注册长/短形式，两者指向同一变量
	flag.StringVar(&flags.Endpoint, "endpoint", "", "面板 URL，例如 https://panel.example.com")
	flag.StringVar(&flags.Endpoint, "e", "", "面板 URL (简写)")
	flag.StringVar(&flags.Token, "token", "", "Agent 认证 token")
	flag.StringVar(&flags.Token, "t", "", "Agent 认证 token (简写)")
	flag.StringVar(&flags.Runtime, "runtime", "xray", "运行时内核类型: xray / sing-box")
	flag.StringVar(&flags.Runtime, "r", "xray", "运行时内核类型 (简写)")
	flag.StringVar(&flags.ConfigDir, "config-dir", "/etc/yundu", "配置目录")
	flag.StringVar(&flags.ConfigDir, "c", "/etc/yundu", "配置目录 (简写)")
	flag.Parse()
	return flags
}

// LoadFromCLI 从命令行参数加载配置。
// 仅填充 CLI flags 提供的字段，其余字段保持零值。调用方需自行合并环境变量。
func LoadFromCLI() *Config {
	flags := ParseCLIFlags()
	cfg := &Config{
		PanelURL:    strings.TrimRight(flags.Endpoint, "/"),
		AgentToken:  flags.Token,
		RuntimeType: flags.Runtime,
		ConfigDir:   flags.ConfigDir,
	}
	if cfg.RuntimeType == "" {
		cfg.RuntimeType = "xray"
	}
	if cfg.ConfigDir == "" {
		cfg.ConfigDir = "/etc/yundu"
	}
	return cfg
}

// BootstrapConfigResponse 是面板 Bootstrap API 返回的运行时配置。
// 与 node-service 的 service.BootstrapConfig 结构对齐。
type BootstrapConfigResponse struct {
	AgentID           string          `json:"agent_id"`
	RuntimeType       string          `json:"runtime_type"`
	RuntimeBin        string          `json:"runtime_bin"`
	ConfigDir         string          `json:"config_dir"`
	HeartbeatInterval int             `json:"heartbeat_interval"`
	GRPCAddr          string          `json:"grpc_addr,omitempty"`
	NginxEnabled      bool            `json:"nginx_enabled"`
	CertMode          string          `json:"cert_mode"`
	WARPEnabled       bool            `json:"warp_enabled"`
	Nodes             []BootstrapNode `json:"nodes"`
}

// BootstrapNode 是 Bootstrap 下发的节点摘要。
type BootstrapNode struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
}

// fetchBootstrapConfig 调用面板 Bootstrap API 获取运行时配置。
// 端点: GET {endpoint}/api/v1/agent/bootstrap?token={token}
func fetchBootstrapConfig(endpoint, token string) (*BootstrapConfigResponse, error) {
	url := fmt.Sprintf("%s/api/v1/agent/bootstrap?token=%s",
		strings.TrimRight(endpoint, "/"), token)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("bootstrap request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read bootstrap response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("bootstrap API error %d: %s", resp.StatusCode, string(body))
	}

	// 面板返回统一封装 {code, message, data, request_id}，需解包 data 字段
	var wrapper struct {
		Code      int                     `json:"code"`
		Message   string                  `json:"message"`
		Data      BootstrapConfigResponse `json:"data"`
		RequestID string                  `json:"request_id"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, fmt.Errorf("decode bootstrap wrapper: %w, body: %s", err, string(body))
	}
	if wrapper.Code != 0 {
		return nil, fmt.Errorf("bootstrap API error code=%d: %s", wrapper.Code, wrapper.Message)
	}
	return &wrapper.Data, nil
}

// LoadWithBootstrap 先从 CLI flags 加载，如果指定了 --endpoint 和 --token，
// 则调用 Bootstrap API 获取完整配置，并按优先级合并：
//
//	CLI flags > 面板下发 (bootstrap) > 环境变量 > 默认值
//
// 若未指定 --endpoint/--token，则回退到原有环境变量模式（Load()）。
func LoadWithBootstrap() (*Config, error) {
	flags := ParseCLIFlags()

	// 未指定 endpoint + token，走原有环境变量模式
	if flags.Endpoint == "" || flags.Token == "" {
		return Load(), nil
	}

	logger := slog.Default().With("component", "config-bootstrap")
	endpoint := strings.TrimRight(flags.Endpoint, "/")

	// 1. 调用 Bootstrap API 获取面板下发的配置
	bsCfg, err := fetchBootstrapConfig(endpoint, flags.Token)
	if err != nil {
		return nil, fmt.Errorf("bootstrap from panel %s: %w", endpoint, err)
	}
	logger.Info("bootstrap config fetched from panel",
		"agent_id", bsCfg.AgentID,
		"runtime_type", bsCfg.RuntimeType,
		"runtime_bin", bsCfg.RuntimeBin,
		"node_count", len(bsCfg.Nodes))

	// 2. 以环境变量配置为基底
	cfg := Load()

	// 3. 标记 bootstrap 模式
	cfg.BootstrapEnabled = true

	// 4. 合并：CLI flags > bootstrap > env（env 已在 Load 中填充）
	// PanelURL: CLI flags (endpoint) 优先
	cfg.PanelURL = endpoint

	// AgentToken: CLI flags (token) 优先
	cfg.AgentToken = flags.Token

	// ServerCode: 来自 bootstrap 的 agent_id（面板用 server.code 作为 agent 标识）
	if bsCfg.AgentID != "" {
		cfg.ServerCode = bsCfg.AgentID
	}

	// RuntimeType: CLI flags > bootstrap > env
	if flags.Runtime != "" && flags.Runtime != "xray" {
		cfg.RuntimeType = flags.Runtime
	} else if bsCfg.RuntimeType != "" {
		cfg.RuntimeType = bsCfg.RuntimeType
	}

	// ConfigDir: CLI flags > bootstrap > env
	if flags.ConfigDir != "" && flags.ConfigDir != "/etc/yundu" {
		cfg.ConfigDir = flags.ConfigDir
	} else if bsCfg.ConfigDir != "" {
		cfg.ConfigDir = bsCfg.ConfigDir
	}

	// RuntimePath: 来自 bootstrap 的 runtime_bin
	if bsCfg.RuntimeBin != "" {
		cfg.RuntimePath = bsCfg.RuntimeBin
	}

	// GRPCAddr: 来自 bootstrap（为空则 agent 从 PanelURL 推导）
	if bsCfg.GRPCAddr != "" {
		cfg.GRPCAddr = bsCfg.GRPCAddr
	}

	// HMACSecret / AgentAPITokenSalt: bootstrap 模式下保持环境变量默认值
	// （面板不下发密钥，agent 使用与面板一致的默认 salt 即可通过 token 校验）

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.PanelURL == "" {
		return fmt.Errorf("PANEL_URL is required")
	}
	if c.ServerCode == "" {
		return fmt.Errorf("SERVER_CODE is required")
	}
	if c.AgentToken == "" {
		return fmt.Errorf("AGENT_TOKEN is required")
	}
	return nil
}

func (c *Config) ConfigFilePath() string {
	switch c.RuntimeType {
	case "xray":
		return c.ConfigDir + "/config/xray.json"
	case "sing-box":
		return c.ConfigDir + "/config/sing-box.json"
	default:
		return c.ConfigDir + "/config.json"
	}
}

func (c *Config) BackupConfigFilePath() string {
	return c.ConfigFilePath() + ".bak"
}

func (c *Config) VersionFilePath() string {
	return c.ConfigDir + "/version.txt"
}

func (c *Config) CertsDir() string {
	return c.ConfigDir + "/certs"
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return fallback
}
