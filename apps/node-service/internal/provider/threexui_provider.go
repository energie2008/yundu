package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ThreeXUIProvider 是 3X-UI 面板的 provider skeleton。
// 3X-UI 是一个基于 xray 的第三方面板，有自己的 HTTP API。
// 本实现仅包含 HTTP client 骨架和能力发现，具体 API 调用需要根据 3X-UI 版本适配。
type ThreeXUIProvider struct {
	baseURL  string
	username string
	password string
	client   *http.Client
	cookie   string // 登录后的 session cookie
}

func NewThreeXUIProvider(baseURL, username, password string) *ThreeXUIProvider {
	return &ThreeXUIProvider{
		baseURL:  baseURL,
		username: username,
		password: password,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (p *ThreeXUIProvider) Type() string { return "3x-ui" }

// login 登录 3X-UI 面板，获取 session cookie
func (p *ThreeXUIProvider) login(ctx context.Context) error {
	body, _ := json.Marshal(map[string]string{
		"username": p.username,
		"password": p.password,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/login", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("3x-ui: create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("3x-ui: login request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("3x-ui: login failed, status=%d", resp.StatusCode)
	}
	// 3X-UI 登录成功后返回 cookie，用于后续请求
	if cookies := resp.Cookies(); len(cookies) > 0 {
		p.cookie = cookies[0].Value
	}
	return nil
}

func (p *ThreeXUIProvider) doWithAuth(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	if p.cookie == "" {
		if err := p.login(ctx); err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", "session="+p.cookie)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	// 如果 401，尝试重新登录一次
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		p.cookie = ""
		if err := p.login(ctx); err != nil {
			return nil, err
		}
		return p.doWithAuth(ctx, method, path, body)
	}
	return resp, nil
}

// RegisterRuntime 3X-UI 不支持通过 API 注册 runtime，runtime 是预先在面板中创建的。
// 这里返回一个基于 serverCode 的引用，实际使用时需要操作员手动在 3X-UI 中创建对应的 inbound。
func (p *ThreeXUIProvider) RegisterRuntime(ctx context.Context, spec RuntimeSpec) (string, error) {
	return "", fmt.Errorf("3x-ui: register runtime not supported, please create inbound manually in 3x-ui panel")
}

func (p *ThreeXUIProvider) PushConfig(ctx context.Context, runtimeRef string, config string) error {
	// 3X-UI 的配置通过 /panel/api/inbounds/add 或 /panel/api/inbounds/update 推送
	// 具体 payload 需要根据 3X-UI 版本的 API 适配
	return fmt.Errorf("3x-ui: push config not implemented (skeleton only, runtimeRef=%s)", runtimeRef)
}

func (p *ThreeXUIProvider) PullStats(ctx context.Context, runtimeRef string) (*RuntimeStats, error) {
	// 3X-UI 通过 /panel/api/inbounds/onlines 获取在线用户
	// 通过 /panel/api/inbounds/:id 获取流量统计
	resp, err := p.doWithAuth(ctx, "POST", "/panel/api/inbounds/onlines", nil)
	if err != nil {
		return nil, fmt.Errorf("3x-ui: pull stats: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("3x-ui: pull stats failed, status=%d", resp.StatusCode)
	}
	var result struct {
		Success bool              `json:"success"`
		Msg     string            `json:"msg"`
		Obj     []json.RawMessage `json:"obj"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("3x-ui: decode stats response: %w", err)
	}
	return &RuntimeStats{
		OnlineUsers: len(result.Obj),
		Status:      "running",
	}, nil
}

func (p *ThreeXUIProvider) Reload(ctx context.Context, runtimeRef string) error {
	// 3X-UI 不支持显式 reload，配置更新后自动生效
	return nil
}

func (p *ThreeXUIProvider) Rollback(ctx context.Context, runtimeRef string) error {
	return fmt.Errorf("3x-ui: rollback not supported")
}

func (p *ThreeXUIProvider) FetchCapabilities(ctx context.Context) ([]string, error) {
	// 3X-UI 能力有限：支持配置推送和统计拉取，不支持 WARP/dry-run/upgrade
	return []string{CapConfigPush, CapStatsPull}, nil
}
