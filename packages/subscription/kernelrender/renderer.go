// Package kernelrender 实现双内核（Xray/Sing-box）服务端配置的统一渲染。
//
// 与现有 renderer 包（客户端订阅格式渲染，outbound 视角）不同，
// 本包专注于生成服务端 inbound 配置，用于 DualKernelValidator 的真实 dry-run 校验。
//
// 设计对齐：yundu项目核心节点设计/YunDu 双核渲染器统一化 + 校验层完整实现.md
package kernelrender

import (
	"fmt"

	"github.com/airport-panel/subscription/nodespec"
)

// KernelType 标识内核类型
type KernelType string

const (
	KernelXray    KernelType = "xray"
	KernelSingBox KernelType = "sing_box"
)

// Renderer 是双内核渲染的统一契约，任何新增内核只需实现这一个接口。
// 返回的 map[string]interface{} 是完整的服务端配置 JSON 结构（可直接写文件用于 dry-run）。
type Renderer interface {
	// Render 渲染单个 NodeSpec 为对应内核的完整服务端配置
	Render(spec *nodespec.NodeSpec) (map[string]interface{}, error)
	// KernelType 返回内核类型标识
	KernelType() KernelType
}

// UnsupportedFeatureError 表示内核完全不支持该协议特性（阻断性错误）
type UnsupportedFeatureError struct {
	Feature string
	Kernel  KernelType
	Hint    string // 可选：修复建议，供调用方展示给用户
}

func (e *UnsupportedFeatureError) Error() string {
	if e.Hint != "" {
		return fmt.Sprintf("内核 %s 不支持协议特性: %s（提示: %s）", e.Kernel, e.Feature, e.Hint)
	}
	return fmt.Sprintf("内核 %s 不支持协议特性: %s", e.Kernel, e.Feature)
}

// UnsupportedFeatureWarning 表示内核将忽略某特性（非阻断性警告）
type UnsupportedFeatureWarning struct {
	Feature string
	Kernel  KernelType
}

func (e *UnsupportedFeatureWarning) Error() string {
	return fmt.Sprintf("内核 %s 将忽略特性 %s（非阻断）", e.Kernel, e.Feature)
}

// Registry 是渲染器注册中心，新增内核只需在此登记，其余代码零改动
var Registry = map[KernelType]Renderer{}

// Register 注册渲染器到全局注册中心
func Register(r Renderer) {
	Registry[r.KernelType()] = r
}

// RenderForKernel 按内核类型渲染配置
func RenderForKernel(kernel KernelType, spec *nodespec.NodeSpec) (map[string]interface{}, error) {
	r, ok := Registry[kernel]
	if !ok {
		return nil, fmt.Errorf("未注册的内核类型: %s", kernel)
	}
	return r.Render(spec)
}

// extractUUID 从 NodeSpec.Credentials 提取 UUID（VLESS/VMess/TUIC 通用）
func extractUUID(spec *nodespec.NodeSpec) string {
	switch c := spec.Credentials.(type) {
	case nodespec.VLESSCredentials:
		return c.UUID
	case nodespec.VMessCredentials:
		return c.UUID
	case nodespec.TUICCredentials:
		return c.UUID
	}
	return ""
}

// extractFlow 从 NodeSpec.Credentials 提取 Flow（VLESS 专用）
func extractFlow(spec *nodespec.NodeSpec) string {
	if c, ok := spec.Credentials.(nodespec.VLESSCredentials); ok {
		return string(c.Flow)
	}
	return ""
}

// extractPassword 从 NodeSpec.Credentials 提取密码（Trojan/Hysteria2/TUIC/AnyTLS 通用）
func extractPassword(spec *nodespec.NodeSpec) string {
	switch c := spec.Credentials.(type) {
	case nodespec.TrojanCredentials:
		return c.Password
	case nodespec.Hysteria2Credentials:
		return c.Password
	case nodespec.TUICCredentials:
		return c.Password
	case nodespec.AnyTLSCredentials:
		return c.Password
	}
	return ""
}

// hasMultiClients 判断 NodeSpec 是否携带多用户凭证（P0-4）。
// 渲染器优先使用 Clients；为空时回退到 Credentials（单用户，向后兼容）。
func hasMultiClients(spec *nodespec.NodeSpec) bool {
	return spec != nil && len(spec.Clients) > 0
}

// clientFlowFor 返回多用户场景下单个 client 的 flow 值。
// 规则与单用户保持一致（RenderClientFlow）：TCP→vision，其他→空。
// 若 client 显式指定 Flow 则优先使用。
func clientFlowFor(c nodespec.CredentialSpec, transportType nodespec.Transport) string {
	if c.Flow != "" {
		return string(c.Flow)
	}
	return string(nodespec.RenderClientFlow(transportType))
}

// renderLimiterMeta 渲染限速器元数据（供 node-agent 解析初始化 SpeedLimiter/DeviceLimiter）。
//
// 返回的结构以 "_limiter" 字段嵌入配置 JSON，Agent 在 Apply 前剥离
// （与 _nginx_vhosts 同机制，不污染内核配置）。
// 包含节点级限速/设备限制/IP限制 + 每用户（uuid/email）的限速/设备限制/IP限制信息。
// 仅在任一限制 > 0 时生成，否则返回 nil。
// 该函数为 Xray/Sing-box 双内核共享，因为限速元数据结构内核无关。
func renderLimiterMeta(spec *nodespec.NodeSpec) map[string]interface{} {
	// 判断是否需要渲染：节点级或任一用户级有限速/设备限制/IP限制
	nodeSpeed := spec.SpeedLimitMbps
	nodeDevice := spec.DeviceLimit
	nodeIP := spec.IPLimit
	hasPerUserLimit := false
	if hasMultiClients(spec) {
		for _, c := range spec.Clients {
			if c.SpeedLimit > 0 || c.DeviceLimit > 0 || c.IPLimit > 0 {
				hasPerUserLimit = true
				break
			}
		}
	}
	if nodeSpeed <= 0 && nodeDevice <= 0 && nodeIP <= 0 && !hasPerUserLimit {
		return nil
	}
	users := make([]map[string]interface{}, 0)
	if hasMultiClients(spec) {
		for _, c := range spec.Clients {
			u := map[string]interface{}{}
			if c.Email != "" {
				u["email"] = c.Email
			}
			if c.UUID != "" {
				u["uuid"] = c.UUID
			}
			// P0: 优先使用 per-user 限速，回退到节点级
			speedLimit := c.SpeedLimit
			if speedLimit <= 0 {
				speedLimit = nodeSpeed
			}
			if speedLimit > 0 {
				u["speed_limit_mbps"] = speedLimit
			}
			// P0: 优先使用 per-user 设备限制，回退到节点级
			deviceLimit := c.DeviceLimit
			if deviceLimit <= 0 {
				deviceLimit = nodeDevice
			}
			if deviceLimit > 0 {
				u["device_limit"] = deviceLimit
			}
			// P0: 优先使用 per-user IP 限制，回退到节点级
			ipLimit := c.IPLimit
			if ipLimit <= 0 {
				ipLimit = nodeIP
			}
			if ipLimit > 0 {
				u["ip_limit"] = ipLimit
			}
			users = append(users, u)
		}
	} else {
		// 单用户路径
		u := map[string]interface{}{}
		if uuid := extractUUID(spec); uuid != "" {
			u["uuid"] = uuid
		}
		if nodeSpeed > 0 {
			u["speed_limit_mbps"] = nodeSpeed
		}
		if nodeDevice > 0 {
			u["device_limit"] = nodeDevice
		}
		if nodeIP > 0 {
			u["ip_limit"] = nodeIP
		}
		users = append(users, u)
	}
	meta := map[string]interface{}{
		"users": users,
	}
	if nodeSpeed > 0 {
		meta["node_speed_limit_mbps"] = nodeSpeed
	}
	if nodeDevice > 0 {
		meta["node_device_limit"] = nodeDevice
	}
	if nodeIP > 0 {
		meta["node_ip_limit"] = nodeIP
	}
	return meta
}

// resolveInboundPort 解析 xray/sing-box inbound 的实际监听端口。
//
// 端口选择规则（P8 端口语义显式分离）：
//   - ServerPort > 0：使用 ServerPort（CDN/Tunnel/SaaS 节点的 xray 本地监听端口）
//     CDN 节点：nginx 占用 443 终止 TLS，xray 监听 ServerPort（如 8445/8446）
//     Tunnel 节点：cloudflared 回源到 ServerPort（如 20530/20960）
//     SaaS 节点：CF 边缘 → nginx → xray ServerPort
//   - ServerPort == 0：使用 Port（直连节点的对外监听端口，如 REALITY 9450）
//
// spec.Port（443）仅用于订阅链接的用户连接端口（renderer 包，ClientPort 优先）
// 和 nginx proxy_pass 目标端口（buildNginxVhosts，cdn_port 优先），不影响 inbound 实际监听。
func resolveInboundPort(spec *nodespec.NodeSpec) int {
	if spec.ServerPort > 0 {
		return spec.ServerPort
	}
	return spec.Port
}

// resolveListenAddress 解析 xray/sing-box inbound 的监听地址。
//
// 监听地址规则（标准架构：nginx 443 + SNI 分流）：
//   - ServerPort > 0 且 ServerPort != Port（CDN/Tunnel/Direct-TCP）：
//     客户端口=443（nginx stream 监听），服务端口=高位端口（xray 监听）
//     流量由 nginx/cloudflared 转发，xray 绑 127.0.0.1，不对外暴露
//   - ServerPort == 0 或 ServerPort == Port（UDP 直连 / 未设服务端口）：
//     客户端口=服务端口=高位端口（如 Hysteria2/TUIC 40020）
//     xray/sing-box 直接接受公网连接，绑 0.0.0.0
//
// 未来扩展：NodeSpec 增加 BindAddress 字段后，DIRECT 节点可指定特定网卡 IP（§2.6）
func resolveListenAddress(spec *nodespec.NodeSpec) string {
	if spec.ServerPort > 0 && spec.ServerPort != spec.Port {
		return "127.0.0.1"
	}
	return "0.0.0.0"
}
