package warpreg

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	warpAPIBase     = "https://api.cloudflareclient.com/v0a4005"
	warpClientVer   = "a-6.30-3596"
	maxResponseSize = 10 << 20 // 10MB
)

// Client 调用 Cloudflare WARP API 注册/查询/删除账户
type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// RegisterResponse 是 Cloudflare 注册返回的完整响应
type RegisterResponse struct {
	ID      string      `json:"id"`      // device_id
	Token   string      `json:"token"`   // access_token
	Account AccountInfo `json:"account"`
	Config  WarpConfig  `json:"config"`
}

type AccountInfo struct {
	License string `json:"license"`
}

type WarpConfig struct {
	ClientID  string        `json:"client_id"`
	Interface InterfaceInfo `json:"interface"`
	Peers     []PeerInfo    `json:"peers"`
}

type InterfaceInfo struct {
	Addresses AddressesInfo `json:"addresses"`
}

type AddressesInfo struct {
	V4 string `json:"v4"`
	V6 string `json:"v6"`
}

type PeerInfo struct {
	PublicKey string       `json:"public_key"`
	Endpoint  EndpointInfo `json:"endpoint"`
}

type EndpointInfo struct {
	V4   string `json:"v4"`
	V6   string `json:"v6"`
	Host string `json:"host"`
}

// Register 注册新 WARP 账户
func (c *Client) Register(ctx context.Context, publicKey, hostname string) (*RegisterResponse, error) {
	body := map[string]string{
		"key":   publicKey,
		"tos":   time.Now().UTC().Format(time.RFC3339),
		"type":  "PC",
		"model": "yundu-panel",
		"name":  hostname,
	}
	payload, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", warpAPIBase+"/reg", strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("CF-Client-Version", warpClientVer)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, parseWarpError(resp)
	}

	limitedReader := io.LimitReader(resp.Body, maxResponseSize)
	var result RegisterResponse
	if err := json.NewDecoder(limitedReader).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode register response: %w", err)
	}
	return &result, nil
}

// GetConfig 查询已有账户配置（健康检查）
func (c *Client) GetConfig(ctx context.Context, deviceID, accessToken string) (*RegisterResponse, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", warpAPIBase+"/reg/"+deviceID, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("CF-Client-Version", warpClientVer)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, parseWarpError(resp)
	}

	limitedReader := io.LimitReader(resp.Body, maxResponseSize)
	var result RegisterResponse
	if err := json.NewDecoder(limitedReader).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SetLicense 应用 WARP+ License
func (c *Client) SetLicense(ctx context.Context, deviceID, accessToken, license string) error {
	payload, _ := json.Marshal(map[string]string{"license": license})
	req, _ := http.NewRequestWithContext(ctx, "PUT", warpAPIBase+"/reg/"+deviceID+"/account", strings.NewReader(string(payload)))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("CF-Client-Version", warpClientVer)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return parseWarpError(resp)
	}
	return nil
}

// DeleteDevice 删除账户
func (c *Client) DeleteDevice(ctx context.Context, deviceID, accessToken string) error {
	req, _ := http.NewRequestWithContext(ctx, "DELETE", warpAPIBase+"/reg/"+deviceID, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("CF-Client-Version", warpClientVer)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func parseWarpError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var errResp struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if json.Unmarshal(body, &errResp) == nil && len(errResp.Errors) > 0 {
		return fmt.Errorf("warp api error %d: %s", resp.StatusCode, errResp.Errors[0].Message)
	}
	return fmt.Errorf("warp api error %d: %s", resp.StatusCode, string(body))
}
