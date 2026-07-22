package warp

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"time"
)

// MockManager 是 Manager 接口的假实现，用于本地开发与沙箱环境。
// 不调用任何真实 warp-cli / exec，所有返回值由字段预设。
// 启用方式：WARP_MODE=mock（由 factory.go 的 NewManager 读取）。
type MockManager struct {
	// FakeStatus 控制状态采集返回值；nil 时使用默认 running 状态。
	FakeStatus *WarpStatus
	// FakeOutboundIP 控制 ProbeOutboundIP 返回值。
	FakeOutboundIP string
	// Socks5Addr 控制 SocksAddr / GetSocks5Outbound 使用的 SOCKS5 监听地址。
	Socks5Addr string
	// 错误注入（nil 表示成功）
	InstallErr   error
	ConnectErr   error
	DisconnectErr error
	CheckErr     error
	ReportErr    error
}

// NewMockManager 返回一个带默认假状态的 MockManager。
// 默认：running / warp_ip=104.28.1.1 / latency=22ms / socks5=127.0.0.1:40000。
func NewMockManager() *MockManager {
	return &MockManager{
		FakeOutboundIP: "104.28.1.1",
		Socks5Addr:     "127.0.0.1:40000",
	}
}

// defaultStatus 返回 MockManager 的默认假状态（running）。
func (m *MockManager) defaultStatus() *WarpStatus {
	return &WarpStatus{
		Status:      "running",
		WarpIP:      m.FakeOutboundIP,
		LatencyMs:   22,
		LastChecked: time.Now(),
	}
}

// DetectWarp 假装已安装 warp-cli。
func (m *MockManager) DetectWarp() bool { return true }

// GetStatus 返回预设或默认的 running 状态。
func (m *MockManager) GetStatus() *WarpStatus {
	if m.FakeStatus != nil {
		return m.FakeStatus
	}
	return m.defaultStatus()
}

// Install 假装安装成功。
func (m *MockManager) Install() error { return m.InstallErr }

// Connect 假装连接成功。
func (m *MockManager) Connect() error { return m.ConnectErr }

// Disconnect 假装断开成功。
func (m *MockManager) Disconnect() error { return m.DisconnectErr }

// Check 返回预设状态（不真正连接）。
func (m *MockManager) Check() (*WarpStatus, error) {
	if m.CheckErr != nil {
		return nil, m.CheckErr
	}
	return m.GetStatus(), nil
}

// GetSocks5Outbound 渲染 WARP SOCKS5 outbound 配置段，与真实 WarpManager 行为一致。
// runtimeType 取值为 "xray" 或 "sing-box"。
func (m *MockManager) GetSocks5Outbound(runtimeType string) (string, error) {
	host, portStr, err := net.SplitHostPort(m.Socks5Addr)
	if err != nil {
		return "", fmt.Errorf("invalid socks addr %q: %w", m.Socks5Addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", fmt.Errorf("invalid socks port %q: %w", portStr, err)
	}

	var out interface{}
	switch runtimeType {
	case "xray":
		out = map[string]interface{}{
			"tag":      "warp-out",
			"protocol": "socks",
			"settings": map[string]interface{}{
				"servers": []map[string]interface{}{
					{"address": host, "port": port},
				},
			},
		}
	case "sing-box":
		out = map[string]interface{}{
			"type":        "socks",
			"tag":         "warp-out",
			"server":      host,
			"server_port": port,
		}
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedRuntimeType, runtimeType)
	}

	b, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("marshal warp outbound: %w", err)
	}
	return string(b), nil
}

// ReportToPanel 假装上报成功。
func (m *MockManager) ReportToPanel(ctx context.Context, status *WarpStatus) error {
	return m.ReportErr
}

// SocksAddr 返回预设的 SOCKS5 监听地址。
func (m *MockManager) SocksAddr() string { return m.Socks5Addr }
