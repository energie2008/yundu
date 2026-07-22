package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// MachineClient 使用server_token认证拉取Machine级别的聚合配置（共享Nginx vhosts等）
type MachineClient struct {
	baseURL     string
	serverToken string
	httpClient  *http.Client
}

func NewMachineClient(baseURL, serverToken string) *MachineClient {
	return &MachineClient{
		baseURL:     baseURL,
		serverToken: serverToken,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
	}
}

// FetchCDNVhosts 实现 VhostFetcher 接口，拉取Machine聚合的nginx vhosts（所有节点合并）
func (c *MachineClient) FetchCDNVhosts(ctx context.Context) (*CDNVhostResponse, error) {
	return c.FetchMachineCDNVhosts(ctx)
}

// FetchCloudflaredTunnels 实现 TunnelFetcher 接口（T05），拉取Machine聚合的cloudflared隧道配置。
// 调用 /api/v1/agent/machine/cloudflared-tunnels?server_token=xxx 端点，返回聚合后的隧道列表。
func (c *MachineClient) FetchCloudflaredTunnels(ctx context.Context) (*CloudflaredTunnelConfig, error) {
	u := fmt.Sprintf("%s/api/v1/agent/machine/cloudflared-tunnels?server_token=%s",
		c.baseURL, url.QueryEscape(c.serverToken))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result struct {
		Code  int                    `json:"code"`
		Data  CloudflaredTunnelConfig `json:"data"`
		Error string                 `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("machine cloudflared-tunnels API error: %s", result.Error)
	}
	return &result.Data, nil
}

// FetchMachineCDNVhosts 拉取Machine聚合的nginx vhosts（所有节点合并）
func (c *MachineClient) FetchMachineCDNVhosts(ctx context.Context) (*CDNVhostResponse, error) {
	u := fmt.Sprintf("%s/api/v1/agent/machine/cdn-vhosts?server_token=%s",
		c.baseURL, url.QueryEscape(c.serverToken))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result struct {
		Code  int              `json:"code"`
		Data  CDNVhostResponse `json:"data"`
		Error string           `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("machine cdn-vhosts API error: %s", result.Error)
	}
	return &result.Data, nil
}
