package warpreg

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"
)

// Registrar 封装 WARP 注册业务逻辑，包含频率控制
type Registrar struct {
	client  *Client
	logger  *slog.Logger
	rateCh  chan struct{} // 频率控制：每 90s 一个 token
	stopCh  chan struct{}
}

// NewRegistrar 创建注册器，rateInterval 控制注册频率（默认 90s）
func NewRegistrar(logger *slog.Logger) *Registrar {
	r := &Registrar{
		client: NewClient(),
		logger: logger,
		rateCh: make(chan struct{}, 1),
		stopCh: make(chan struct{}),
	}
	// 启动频率控制 ticker：每 90s 发送一个 token
	r.rateCh <- struct{}{} // 初始允许立即注册第一次
	go r.runRateLimiter(90 * time.Second)
	return r
}

func (r *Registrar) runRateLimiter(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			select {
			case r.rateCh <- struct{}{}:
			default: // channel 满，跳过
			}
		case <-r.stopCh:
			return
		}
	}
}

// Stop 停止频率控制
func (r *Registrar) Stop() {
	close(r.stopCh)
}

// RegisterResult 是注册结果，用于写入 warp_profiles 表
type RegisterResult struct {
	DeviceID    string
	AccessToken string
	PrivateKey  string
	PublicKey   string // 对端公钥（Cloudflare 返回）
	ClientID    string
	IPv4Address string
	IPv6Address string
	Endpoint    string
	LicenseKey  string
}

// Register 注册一个新 WARP 账户（受频率限制）
func (r *Registrar) Register(ctx context.Context) (*RegisterResult, error) {
	// 1. 等待频率 token
	select {
	case <-r.rateCh:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(120 * time.Second):
		return nil, fmt.Errorf("rate limit wait timeout")
	}

	// 2. 生成密钥对
	privateKey, publicKey, err := GenerateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("generate keypair: %w", err)
	}

	// 3. 注册
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "yundu-panel"
	}
	resp, err := r.client.Register(ctx, publicKey, hostname)
	if err != nil {
		return nil, fmt.Errorf("register: %w", err)
	}

	// 4. 解析结果
	result := &RegisterResult{
		DeviceID:    resp.ID,
		AccessToken: resp.Token,
		PrivateKey:  privateKey,
		ClientID:    resp.Config.ClientID,
		LicenseKey:  resp.Account.License,
	}
	if len(resp.Config.Peers) > 0 {
		result.PublicKey = resp.Config.Peers[0].PublicKey
		result.Endpoint = resp.Config.Peers[0].Endpoint.Host
		if result.Endpoint == "" {
			result.Endpoint = "engage.cloudflareclient.com:2408"
		}
	} else {
		result.PublicKey = "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo="
		result.Endpoint = "engage.cloudflareclient.com:2408"
	}
	result.IPv4Address = resp.Config.Interface.Addresses.V4
	result.IPv6Address = resp.Config.Interface.Addresses.V6

	r.logger.Info("warp account registered",
		"device_id", result.DeviceID,
		"ipv4", result.IPv4Address,
		"ipv6", result.IPv6Address,
	)
	return result, nil
}

// RotateIP 轮换 IP（重新注册新账户，保留旧 license）
func (r *Registrar) RotateIP(ctx context.Context, oldDeviceID, oldAccessToken, oldLicense string) (*RegisterResult, error) {
	// 1. 注册新账户
	result, err := r.Register(ctx)
	if err != nil {
		return nil, err
	}

	// 2. 重新应用旧 license（WARP+ 用户）
	if len(oldLicense) >= 26 {
		if err := r.client.SetLicense(ctx, result.DeviceID, result.AccessToken, oldLicense); err != nil {
			r.logger.Warn("re-apply license failed (non-blocking)", "error", err)
		} else {
			result.LicenseKey = oldLicense
		}
	}

	// 3. 删除旧账户
	if err := r.client.DeleteDevice(ctx, oldDeviceID, oldAccessToken); err != nil {
		r.logger.Warn("delete old device failed (non-blocking)", "device_id", oldDeviceID, "error", err)
	}
	return result, nil
}

// HealthCheck 检查账户健康状态
func (r *Registrar) HealthCheck(ctx context.Context, deviceID, accessToken string) error {
	_, err := r.client.GetConfig(ctx, deviceID, accessToken)
	return err
}
