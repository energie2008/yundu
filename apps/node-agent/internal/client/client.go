package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/airport-panel/config/auth"
)

type Client struct {
	baseURL    string
	serverCode string
	agentToken string
	hmacSecret string
	runtimeRef string
	// payloadKey 用于解密面板下发的加密 Payload Manifest（边缘自治全链路）。
	payloadKey string
	httpClient *http.Client
	logger     *slog.Logger
	// configETag 上次成功获取配置的 ETag，用于 If-None-Match 缓存协商。
	configETag string
}

var ErrNotModified = errors.New("config not modified (304)")

func New(baseURL, serverCode, agentToken, hmacSecret string, logger *slog.Logger) *Client {
	payloadKey := os.Getenv("YUNDU_PAYLOAD_KEY")
	if payloadKey == "" {
		payloadKey = "yundu-default-payload-key-v1"
	}
	return &Client{
		baseURL:    baseURL,
		serverCode: serverCode,
		agentToken: agentToken,
		hmacSecret: hmacSecret,
		payloadKey: payloadKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		logger:     logger.With("component", "agent-client"),
	}
}

// PayloadKey 返回当前 payload 加密密钥（供 pipeline 解密 payload 时使用）
func (c *Client) PayloadKey() string {
	return c.payloadKey
}

func (c *Client) SetRuntimeRef(ref string) {
	c.runtimeRef = ref
}

type apiResponse struct {
	Code      int             `json:"code"`
	Message   string          `json:"message"`
	Data      json.RawMessage `json:"data"`
	RequestID string          `json:"request_id"`
}

type RegisterRequest struct {
	RuntimeType         string                 `json:"runtime_type"`
	RuntimeVersion      *string                `json:"runtime_version,omitempty"`
	ListenHost          *string                `json:"listen_host,omitempty"`
	APIPort             *int                   `json:"api_port,omitempty"`
	XrayAPIPort         *int                   `json:"xray_api_port,omitempty"`
	SingboxClashPort    *int                   `json:"singbox_clash_port,omitempty"`
	Capabilities        map[string]interface{} `json:"capabilities,omitempty"`
	ConfigSchemaVersion string                 `json:"config_schema_version,omitempty"`
	Metadata            map[string]interface{} `json:"metadata,omitempty"`
	Hostname            string                 `json:"-"`
	PublicIP            string                 `json:"-"`
	OS                  string                 `json:"-"`
	Arch                string                 `json:"-"`
	AgentVersion        string                 `json:"-"`
}

type RegisterResponse struct {
	NodeID   string `json:"node_id"`
	ServerID string `json:"server_id"`
}

type HeartbeatRequest struct {
	ServerCode     string                 `json:"server_code"`
	Timestamp      time.Time              `json:"timestamp"`
	ConfigVersion  string                 `json:"config_version_current,omitempty"`
	RTTMs          *int                   `json:"rtt_ms,omitempty"`
	LossRatio      *float64               `json:"loss_ratio,omitempty"`
	OnlineUsers    int                    `json:"online_users,omitempty"`
	CPUPercent     *float64               `json:"cpu_percent,omitempty"`
	MemPercent     *float64               `json:"mem_percent,omitempty"`
	DiskPercent    *float64               `json:"disk_percent,omitempty"`
	XrayAPIPort    *int                   `json:"xray_api_port,omitempty"`
	SingboxClashPort *int                 `json:"singbox_clash_port,omitempty"`
	Metrics        map[string]interface{} `json:"metrics,omitempty"`
	ErrorMessage   *string                `json:"error_message,omitempty"`
	// 通道健康（三通道降级状态）
	ChannelHealth *ChannelHealthReport `json:"channel_health,omitempty"`
	OS             string                 `json:"os,omitempty"`
	Arch           string                 `json:"arch,omitempty"`
	AgentVersion   string                 `json:"agent_version,omitempty"`
	RuntimeStatus  string                 `json:"runtime_status,omitempty"`
	RuntimeVersion string                 `json:"runtime_version,omitempty"`
	Pid            int                    `json:"pid,omitempty"`
}

// ChannelHealthReport 心跳上报的通道健康数据
type ChannelHealthReport struct {
	ActiveChannel string           `json:"active_channel"`
	ChannelState  string           `json:"channel_state"`
	RTTMs         *int             `json:"rtt_ms,omitempty"`
	FailCount1h   int              `json:"fail_count_1h,omitempty"`
	OnlineUsers   int              `json:"online_users,omitempty"`
	LastError     *string          `json:"last_error,omitempty"`
	Failover      *ChannelFailover `json:"failover,omitempty"`
}

// ChannelFailover 心跳中携带的降级事件
type ChannelFailover struct {
	FromChannel string `json:"from_channel"`
	ToChannel   string `json:"to_channel"`
	Reason      string `json:"reason"`
}

type HeartbeatResponse struct {
	Status              string  `json:"status"`
	CurrentTime         int64   `json:"current_time"`
	TargetConfigVersion *string `json:"target_config_version,omitempty"`
	ConfigURL           *string `json:"config_url,omitempty"`
	ConfigSignature     *string `json:"config_signature,omitempty"`
	Action              *string `json:"action,omitempty"`
	// ExtraActions 附加动作列表（与 Action 并行执行）
	ExtraActions        []string `json:"extra_actions,omitempty"`
	NeedUpgrade         *bool   `json:"need_upgrade,omitempty"`
	UpgradeURL          *string `json:"upgrade_url,omitempty"`
	UpgradeVersion      *string `json:"upgrade_version,omitempty"`
	NeedReboot          *bool   `json:"need_reboot,omitempty"`
}

type ConfigResponse struct {
	Version   string                 `json:"version"`
	Config    map[string]interface{} `json:"config"`
	Signature string                 `json:"signature"`
	AppliedAt string                 `json:"applied_at,omitempty"`
}

type ConfigResult struct {
	Version         string `json:"version"`
	Success         bool   `json:"success"`
	Message         string `json:"message"`
	RollbackVersion string `json:"rollback_version,omitempty"`
	DurationMs      int64  `json:"duration_ms"`
}

// PayloadManifestResponse 是面板下发的加密 Payload Manifest 响应。
//
// 边缘自治全链路：Agent 通过 FetchPayload 拉取加密的配置 payload，
// 使用 payloadKey 在本地解密后交由 pipeline 执行预检/激活/测活/回滚。
type PayloadManifestResponse struct {
	VersionNo        int64           `json:"version_no"`
	DeploymentID     string          `json:"deployment_id,omitempty"`
	SHA256           string          `json:"sha256"`
	Timestamp        int64           `json:"timestamp"`
	Kernel           string          `json:"kernel"`
	RollbackStrategy string          `json:"rollback_strategy"`
	PayloadEncrypted bool            `json:"payload_encrypted"`
	Content          json.RawMessage `json:"content"`
}

// DeploymentResultRequest 是 Agent 上报部署结果（ACK/NACK）的请求体。
//
// Status: "ack" 表示部署成功，"nack" 表示部署失败（已触发回滚）。
// Phase: "precheck" / "activate" / "healthcheck" 标识失败发生在哪个阶段。
type DeploymentResultRequest struct {
	Version    string `json:"version"`
	Status     string `json:"status"` // ack / nack
	Phase      string `json:"phase"`  // precheck / activate / healthcheck
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"duration_ms"`
}

func generateNonce() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}, extraHeaders ...http.Header) (*http.Response, error) {
	var bodyBytes []byte
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyBytes = data
		bodyReader = bytes.NewReader(data)
	}

	url := fmt.Sprintf("%s%s", c.baseURL, path)
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	timestamp := time.Now().Unix()
	nonce := generateNonce()
	signature := auth.Sign(method, path, string(bodyBytes), timestamp, nonce, c.hmacSecret)

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Server-Code", c.serverCode)
	req.Header.Set("X-Agent-Token", c.agentToken)
	req.Header.Set(auth.HeaderTimestamp, fmt.Sprintf("%d", timestamp))
	req.Header.Set(auth.HeaderNonce, nonce)
	req.Header.Set(auth.HeaderSignature, signature)
	if c.runtimeRef != "" {
		req.Header.Set("X-Runtime-Ref", c.runtimeRef)
	}
	for _, h := range extraHeaders {
		for k, vals := range h {
			for _, v := range vals {
				req.Header.Set(k, v)
			}
		}
	}

	c.logger.Debug("sending request", "method", method, "url", url)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *Client) doRequestWithHeaders(ctx context.Context, method, path string, body interface{}, extraHeaders http.Header) (*http.Response, error) {
	return c.doRequest(ctx, method, path, body, extraHeaders)
}

func (c *Client) decodeResponse(resp *http.Response, result interface{}) error {
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var apiResp apiResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return fmt.Errorf("decode response wrapper: %w, body: %s", err, string(bodyBytes))
	}

	if apiResp.Code != 0 {
		return fmt.Errorf("API error code=%d: %s (request_id=%s)", apiResp.Code, apiResp.Message, apiResp.RequestID)
	}

	if result != nil && len(apiResp.Data) > 0 {
		if err := json.Unmarshal(apiResp.Data, result); err != nil {
			return fmt.Errorf("decode response data: %w, data: %s", err, string(apiResp.Data))
		}
	}

	return nil
}

func (c *Client) Register(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/agent/register", req)
	if err != nil {
		return nil, err
	}

	var regResp RegisterResponse
	if err := c.decodeResponse(resp, &regResp); err != nil {
		return nil, err
	}
	return &regResp, nil
}

func (c *Client) Heartbeat(ctx context.Context, req *HeartbeatRequest) (*HeartbeatResponse, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/agent/heartbeat", req)
	if err != nil {
		return nil, err
	}

	var hbResp HeartbeatResponse
	if err := c.decodeResponse(resp, &hbResp); err != nil {
		return nil, err
	}
	return &hbResp, nil
}

func (c *Client) FetchConfig(ctx context.Context, version string) (*ConfigResponse, error) {
	path := "/api/v1/agent/config"
	if version != "" {
		path = fmt.Sprintf("%s?version=%s", path, version)
	}

	extraHeaders := make(http.Header)
	if c.configETag != "" {
		extraHeaders.Set("If-None-Match", c.configETag)
	}

	resp, err := c.doRequestWithHeaders(ctx, http.MethodGet, path, nil, extraHeaders)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusNotModified {
		resp.Body.Close()
		return nil, ErrNotModified
	}

	var cfgResp ConfigResponse
	if err := c.decodeResponse(resp, &cfgResp); err != nil {
		return nil, err
	}

	if etag := resp.Header.Get("ETag"); etag != "" {
		c.configETag = etag
	}
	return &cfgResp, nil
}

func (c *Client) ReportResult(ctx context.Context, result *ConfigResult) error {
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/agent/config/result", result)
	if err != nil {
		return err
	}
	return c.decodeResponse(resp, nil)
}

// CDNVhostResponse CDN vhost 同步响应，包含渲染好的 nginx snippet。
// node-agent 的 nginx reconciler 拉取此内容后，用 nginx.Sync 原子写入并 reload。
type CDNVhostResponse struct {
	HTTPSSnippet  string   `json:"https_snippet"`
	StreamSnippet string   `json:"stream_snippet"`
	ListenPort    int      `json:"listen_port"`
	Domains       []string `json:"domains"` // 需要证书的域名列表，供 agent ACME 签发
}

// VhostFetcher 是拉取 Nginx CDN vhost 配置的接口抽象。
// 由 *Client（单节点模式）和 *MachineClient（Machine 模式聚合）实现。
type VhostFetcher interface {
	FetchCDNVhosts(ctx context.Context) (*CDNVhostResponse, error)
}

// TunnelFetcher 是拉取 cloudflared 隧道配置的接口抽象。
// 由 *Client（单节点模式）和 *MachineClient（Machine 模式聚合）实现。
// T05: 让 CloudflaredReconciler 在 Machine 模式下也能复用。
type TunnelFetcher interface {
	FetchCloudflaredTunnels(ctx context.Context) (*CloudflaredTunnelConfig, error)
}

// FetchCDNVhosts 拉取当前 server 的 CDN 节点 nginx vhost snippet。
// 独立于 xray config_versions 版本管理，供 nginx reconciler 定期轮询调用。
func (c *Client) FetchCDNVhosts(ctx context.Context) (*CDNVhostResponse, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/agent/cdn-vhosts", nil)
	if err != nil {
		return nil, err
	}

	var result CDNVhostResponse
	if err := c.decodeResponse(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// CloudflaredTunnelConfig 面板下发的 cloudflared 隧道配置
type CloudflaredTunnelConfig struct {
	Tunnels []CloudflaredTunnel `json:"tunnels"`
}

// CloudflaredTunnel 单条 cloudflared 隧道配置
type CloudflaredTunnel struct {
	Token    string                 `json:"token"`           // CF tunnel token（优先使用）
	TunnelID string                 `json:"tunnel_id"`       // 或 tunnel ID + credentials
	Ingress  []IngressRule          `json:"ingress"`         // ingress 规则
	Config   map[string]interface{} `json:"config,omitempty"` // 额外 config.yml 字段
}

// IngressRule cloudflared ingress 路由规则
type IngressRule struct {
	Hostname      string                 `json:"hostname"`
	Service       string                 `json:"service"` // 如 https://127.0.0.1:8080
	OriginRequest map[string]interface{} `json:"originRequest,omitempty"` // 方案1: noTLSVerify/http2Origin 等
}

// FetchCloudflaredTunnels 拉取当前 server 的 cloudflared 隧道配置。
// 独立于 xray config_versions 版本管理，供 cloudflared reconciler 定期轮询调用。
func (c *Client) FetchCloudflaredTunnels(ctx context.Context) (*CloudflaredTunnelConfig, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/agent/cloudflared-tunnels", nil)
	if err != nil {
		return nil, err
	}

	var result CloudflaredTunnelConfig
	if err := c.decodeResponse(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// BinarySpec P2-4: 期望的二进制规格（从面板拉取）
type BinarySpec struct {
	RuntimeType string `json:"runtime_type"` // xray / sing-box
	Version     string `json:"version"`      // 目标版本号
	DownloadURL string `json:"download_url"` // 下载地址
	Checksum    string `json:"checksum"`     // SHA-256 校验和（hex 编码）
	Strategy    string `json:"strategy"`     // canary / rolling / all_at_once
	Force       bool   `json:"force"`        // 强制升级（即使版本号相同）
}

// FetchBinarySpec P2-4: 拉取当前 server 的期望二进制规格。
// 返回 nil+nil 表示无升级任务（版本已是最新）。
func (c *Client) FetchBinarySpec(ctx context.Context) (*BinarySpec, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/agent/binary-spec", nil)
	if err != nil {
		return nil, err
	}

	var result BinarySpec
	if err := c.decodeResponse(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// FetchPayload 通过 HTTP 拉取加密的 Payload Manifest。
//
// 边缘自治全链路：Agent 在 Jitter Pull 后调用此方法获取配置 payload，
// 返回的 Content 若 PayloadEncrypted 为 true 则需使用 payloadKey 本地解密。
// version 为空时拉取最新版本。
func (c *Client) FetchPayload(ctx context.Context, version string) (*PayloadManifestResponse, error) {
	path := "/api/v1/agent/payload"
	if version != "" {
		path = fmt.Sprintf("%s?version=%s", path, version)
	}
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var result PayloadManifestResponse
	if err := c.decodeResponse(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ReportDeploymentResult 上报部署结果 (ACK/NACK)。
//
// 边缘自治全链路：Agent 在部署流水线结束后调用此方法，
// 向面板汇报本次部署是否成功及失败阶段，供面板追踪部署状态与灰度进度。
func (c *Client) ReportDeploymentResult(ctx context.Context, req *DeploymentResultRequest) error {
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/agent/deployment-result", req)
	if err != nil {
		return err
	}
	return c.decodeResponse(resp, nil)
}

// ===== 设备状态上报 API =====

// DeviceReportRequest 设备状态上报请求体。
//
// Agent 每 30s 调用 POST /api/v1/agent/devices/report 上报本节点观测到的
// 在线设备 IP 列表（uuid -> []ip）。面板据此汇总跨节点全局设备态，
// 再通过 GET /api/v1/agent/devices/alive 或 WS sync.devices 下发给各节点。
type DeviceReportRequest struct {
	Devices map[string][]string `json:"devices"` // uuid -> []ip（本节点去重后的在线 IP）
}

// AliveDevicesResponse 全局设备态响应。
//
// Agent 每 60s 调用 GET /api/v1/agent/devices/alive 拉取面板汇总的
// 跨节点全局设备数（uuid -> 全局在线设备总数）。用于 DeviceLimiter
// 的跨节点设备数合并判定。
type AliveDevicesResponse struct {
	Devices map[string]int `json:"devices"` // uuid -> global device count
}

// ReportDevices 上报本节点在线设备 IP 列表到面板。
//
// POST /api/v1/agent/devices/report
// 供 Agent 的 30s 定时设备上报循环调用。面板据此汇总全局设备态。
func (c *Client) ReportDevices(ctx context.Context, req *DeviceReportRequest) error {
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/agent/devices/report", req)
	if err != nil {
		return err
	}
	return c.decodeResponse(resp, nil)
}

// FetchAliveDevices 拉取面板汇总的跨节点全局设备态。
//
// GET /api/v1/agent/devices/alive
// 供 Agent 的 60s 定时全局设备态拉取循环调用。返回的设备数
// 传入 DeviceLimiter.UpdateGlobalDevices 更新本地全局设备态。
// WS 断连时作为降级手段补充 WS sync.devices 事件。
func (c *Client) FetchAliveDevices(ctx context.Context) (*AliveDevicesResponse, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/agent/devices/alive", nil)
	if err != nil {
		return nil, err
	}
	var result AliveDevicesResponse
	if err := c.decodeResponse(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ===== 流量上报 API =====

// TrafficReportItemReq 单条流量上报项。
// Credential 为用户 UUID 或 email，traffic-service 据此反查 user_id。
type TrafficReportItemReq struct {
	UserID        string `json:"user_id,omitempty"`
	Credential    string `json:"credential,omitempty"`
	UploadBytes   int64  `json:"upload_bytes"`
	DownloadBytes int64  `json:"download_bytes"`
	Timestamp     string `json:"timestamp,omitempty"`
}

// TrafficReportRequest 流量上报请求体。
type TrafficReportRequest struct {
	ServerCode string                  `json:"server_code"`
	Reports    []TrafficReportItemReq  `json:"reports"`
}

// ReportTraffic 上报 per-user 流量统计到 traffic-service。
//
// POST /api/v1/agent/traffic/report
// 供 Agent 的 60s 定时流量上报循环调用。流量数据来自 xray StatsService.QueryStats
// （Reset=true 增量读取）。面板据此累加用户流量、检查配额、更新在线状态。
func (c *Client) ReportTraffic(ctx context.Context, req *TrafficReportRequest) error {
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/agent/traffic/report", req)
	if err != nil {
		return err
	}
	return c.decodeResponse(resp, nil)
}
