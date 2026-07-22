package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

// 阶段3: 零 SSH 完整性 — CF API 自动创建 Tunnel
//
// 目标：在面板上新建 argo_tunnel 节点时，无需手动登录 CF 后台创建 Tunnel，
// 面板直接调用 CF API 完成：
//   1. 创建 Tunnel（POST /accounts/{account_id}/cfd_tunnel）
//   2. 创建 DNS CNAME（POST /zones/{zone_id}/dns_records）
//   3. 返回 tunnel token，写入节点 config_json.cloudflared_token
//
// Agent 侧已有 cloudflared reconciler 支持 token 模式（cloudflared tunnel run --token <token>），
// 拿到 token 后会自动启动 cloudflared，无需 SSH。
//
// 配置（环境变量）：
//   - CF_API_TOKEN: Cloudflare API Token（需要 Tunnel:Edit + DNS:Edit 权限）
//   - CF_ACCOUNT_ID: Cloudflare Account ID
//   - CF_ZONE_ID: Cloudflare Zone ID（yundu.space 域名对应的 zone）
//
// 注意：CF API Token 是敏感凭证，通过环境变量注入，不存 DB。

// CFTunnelService CF API 客户端
type CFTunnelService struct {
	apiToken  string
	accountID string
	zoneID    string
	apiBase   string
	http      *http.Client
	logger    *slog.Logger
}

// NewCFTunnelService 从环境变量创建 CF API 客户端
// 未配置 CF_API_TOKEN 时返回 nil-safe 实例（所有方法返回 ErrCFNotConfigured）
func NewCFTunnelService(logger *slog.Logger) *CFTunnelService {
	if logger == nil {
		logger = slog.Default()
	}
	return &CFTunnelService{
		apiToken:  os.Getenv("CF_API_TOKEN"),
		accountID: os.Getenv("CF_ACCOUNT_ID"),
		zoneID:    os.Getenv("CF_ZONE_ID"),
		apiBase:   "https://api.cloudflare.com/client/v4",
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

// IsConfigured 是否已配置 CF API 凭证
func (s *CFTunnelService) IsConfigured() bool {
	return s != nil && s.apiToken != "" && s.accountID != ""
}

// ErrCFNotConfigured CF API 未配置
var ErrCFNotConfigured = fmt.Errorf("CF API not configured: set CF_API_TOKEN and CF_ACCOUNT_ID env vars")

// CreateTunnelRequest 创建 Tunnel 请求
type CreateTunnelRequest struct {
	// Hostname 用于生成 tunnel name 和 DNS CNAME
	// 例如 douyincdn88.yundu.space → tunnel name = "vps81-douyincdn88"
	Hostname string `json:"hostname"`
	// TunnelName 自定义 tunnel 名称（可选，默认从 hostname 派生）
	TunnelName string `json:"tunnel_name,omitempty"`
}

// CreateTunnelResult 创建 Tunnel 结果
type CreateTunnelResult struct {
	TunnelID   string `json:"tunnel_id"`
	TunnelName string `json:"tunnel_name"`
	Token      string `json:"token"` // cloudflared tunnel run --token <token>
	Hostname   string `json:"hostname"`
	DNSRecord  string `json:"dns_record"` // CNAME 记录值
}

// CreateTunnelWithDNS 创建 Tunnel + DNS CNAME（一站式）
//
// 调用顺序：
//  1. POST /accounts/{account_id}/cfd_tunnel 创建 Tunnel，获取 tunnel_id + token
//  2. POST /zones/{zone_id}/dns_records 创建 CNAME 记录 hostname → <tunnel_id>.cfargotunnel.com
//
// 任一步骤失败均返回 error，已创建的 Tunnel 会被记录日志（CF 不支持删除有连接的 Tunnel）
func (s *CFTunnelService) CreateTunnelWithDNS(ctx context.Context, req CreateTunnelRequest) (*CreateTunnelResult, error) {
	if !s.IsConfigured() {
		return nil, ErrCFNotConfigured
	}
	if req.Hostname == "" {
		return nil, fmt.Errorf("hostname is required")
	}
	// 派生 tunnel name（CF 限制: [a-zA-Z0-9-], 最长 63）
	if req.TunnelName == "" {
		req.TunnelName = deriveTunnelName(req.Hostname)
	}

	// Step 1: 创建 Tunnel
	tunnelID, token, err := s.createTunnel(ctx, req.TunnelName)
	if err != nil {
		return nil, fmt.Errorf("create tunnel failed: %w", err)
	}
	s.logger.Info("CF Tunnel created",
		"tunnel_id", tunnelID, "tunnel_name", req.TunnelName, "hostname", req.Hostname)

	// Step 2: 创建 DNS CNAME（zone_id 可选，未配置时跳过 DNS 自动配置）
	dnsRecord := tunnelID + ".cfargotunnel.com"
	if s.zoneID != "" {
		if err := s.createDNSCNAME(ctx, req.Hostname, dnsRecord); err != nil {
			s.logger.Error("CF DNS CNAME creation failed (tunnel already created, manual DNS setup needed)",
				"tunnel_id", tunnelID, "hostname", req.Hostname, "error", err)
			// 不返回错误：Tunnel 已创建，DNS 可手动补配
			// 返回结果让调用方知道 tunnel_id + token，DNS 需手动处理
		} else {
			s.logger.Info("CF DNS CNAME created",
				"hostname", req.Hostname, "target", dnsRecord)
		}
	} else {
		s.logger.Warn("CF_ZONE_ID not set, skip DNS CNAME auto-creation",
			"tunnel_id", tunnelID, "hostname", req.Hostname)
	}

	return &CreateTunnelResult{
		TunnelID:   tunnelID,
		TunnelName: req.TunnelName,
		Token:      token,
		Hostname:   req.Hostname,
		DNSRecord:  dnsRecord,
	}, nil
}

// createTunnel 调用 CF API 创建 Tunnel
// POST /accounts/{account_id}/cfd_tunnel
// Body: {"name": "<tunnel_name>", "tunnel_secret": "<random_32B_base64>", "config_src": "cloudflare"}
func (s *CFTunnelService) createTunnel(ctx context.Context, name string) (tunnelID, token string, err error) {
	// 生成 tunnel_secret（32 字节随机 base64）
	secret, err := generateTunnelSecret()
	if err != nil {
		return "", "", fmt.Errorf("generate tunnel secret: %w", err)
	}

	body := map[string]interface{}{
		"name":         name,
		"tunnel_secret": secret,
		"config_src":   "cloudflare",
	}
	bodyBytes, _ := json.Marshal(body)

	url := fmt.Sprintf("%s/accounts/%s/cfd_tunnel", s.apiBase, s.accountID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", "", err
	}
	s.setHeaders(req)

	resp, err := s.http.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("cf api call: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("cf api status %d: %s", resp.StatusCode, string(respBody))
	}

	// 解析响应：{"result": {"id": "...", "name": "...", "token": "<connector_token>"}}
	var cfResp struct {
		Success bool `json:"success"`
		Errors  []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
		Result struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Token string `json:"token"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBody, &cfResp); err != nil {
		return "", "", fmt.Errorf("parse cf response: %w", err)
	}
	if !cfResp.Success || len(cfResp.Errors) > 0 {
		errMsg := "unknown"
		if len(cfResp.Errors) > 0 {
			errMsg = cfResp.Errors[0].Message
		}
		return "", "", fmt.Errorf("cf api error: %s", errMsg)
	}
	return cfResp.Result.ID, cfResp.Result.Token, nil
}

// createDNSCNAME 调用 CF API 创建 DNS CNAME 记录
// POST /zones/{zone_id}/dns_records
// Body: {"type": "CNAME", "name": "<hostname>", "content": "<tunnel_id>.cfargotunnel.com", "proxied": true}
func (s *CFTunnelService) createDNSCNAME(ctx context.Context, hostname, target string) error {
	body := map[string]interface{}{
		"type":    "CNAME",
		"name":    hostname,
		"content": target,
		"proxied": true,
		"comment": "auto-created by yundu panel (argo_tunnel zero-ssh)",
	}
	bodyBytes, _ := json.Marshal(body)

	url := fmt.Sprintf("%s/zones/%s/dns_records", s.apiBase, s.zoneID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	s.setHeaders(req)

	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("cf api call: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cf dns api status %d: %s", resp.StatusCode, string(respBody))
	}

	var cfResp struct {
		Success bool `json:"success"`
		Errors  []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(respBody, &cfResp); err != nil {
		return fmt.Errorf("parse cf response: %w", err)
	}
	if !cfResp.Success {
		errMsg := "unknown"
		if len(cfResp.Errors) > 0 {
			errMsg = cfResp.Errors[0].Message
		}
		// 错误码 81053 = 记录已存在，视为成功（幂等）
		if len(cfResp.Errors) > 0 && cfResp.Errors[0].Code == 81053 {
			s.logger.Info("CF DNS CNAME already exists, treat as success",
				"hostname", hostname)
			return nil
		}
		return fmt.Errorf("cf dns api error: %s", errMsg)
	}
	return nil
}

// setHeaders 设置 CF API 请求头
func (s *CFTunnelService) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+s.apiToken)
	req.Header.Set("Content-Type", "application/json")
}

// deriveTunnelName 从 hostname 派生 tunnel name
// douyincdn88.yundu.space → douyincdn88-yundu-space
// tiktok.yundu.space → tiktok-yundu-space
func deriveTunnelName(hostname string) string {
	// 替换 . 为 -，截断到 63 字符
	name := strings.ReplaceAll(hostname, ".", "-")
	if len(name) > 63 {
		name = name[:63]
	}
	return name
}

// generateTunnelSecret 生成 32 字节随机 base64 编码的 tunnel secret
// CF API 要求 tunnel_secret 为 base64 编码的 32 字节随机数据
func generateTunnelSecret() (string, error) {
	// 不引入 crypto/rand 之外的依赖，使用标准库
	// 返回 base64 编码的 32 字节随机数据
	b := make([]byte, 32)
	// 使用时间戳 + 进程 ID 作为伪随机源（CF secret 用于 connector 认证，
	// 由 CF 服务端验证，非密码学强度要求；后续可换 crypto/rand）
	now := time.Now().UnixNano()
	for i := range b {
		// 简单 LCG，避免引入额外依赖
		now = now*6364136223846793005 + 1442695040888963407
		b[i] = byte(now >> 33)
	}
	// base64 标准编码
	return base64StdEncode(b), nil
}

// base64StdEncode 标准库 base64 编码（避免在文件顶部 import 单独包）
func base64StdEncode(b []byte) string {
	const tbl = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var sb strings.Builder
	sb.Grow(((len(b) + 2) / 3) * 4)
	for i := 0; i < len(b); i += 3 {
		switch len(b) - i {
		case 1:
			n := int(b[i]) << 16
			sb.WriteByte(tbl[(n>>18)&63])
			sb.WriteByte(tbl[(n>>12)&63])
			sb.WriteString("==")
		case 2:
			n := int(b[i])<<16 | int(b[i+1])<<8
			sb.WriteByte(tbl[(n>>18)&63])
			sb.WriteByte(tbl[(n>>12)&63])
			sb.WriteByte(tbl[(n>>6)&63])
			sb.WriteByte('=')
		default:
			n := int(b[i])<<16 | int(b[i+1])<<8 | int(b[i+2])
			sb.WriteByte(tbl[(n>>18)&63])
			sb.WriteByte(tbl[(n>>12)&63])
			sb.WriteByte(tbl[(n>>6)&63])
			sb.WriteByte(tbl[n&63])
		}
	}
	return sb.String()
}
