// Package inboundgroup 实现 Xray 多 inbound 合并架构（InboundGroup）。
//
// 核心思想：将多个共享 443 端口的节点合并为一个 primary inbound（带 fallbacks）
// + N 个 internal inbound（unix socket），避免每个节点独占一个端口。
//
// 社区避坑点：
//   - fallbacks 的 dest 支持 unix socket 格式，@开头代表 abstract 类型
//   - v25.7.26 之后只填端口号的 dest 默认指向 localhost 而非 127.0.0.1，
//     本实现统一显式写 127.0.0.1:port，不依赖隐式补全
//   - fallback-by-path 是精确字符串匹配，非前缀匹配，path 冲突必须严格检测
package inboundgroup

import (
	"fmt"
	"strings"

	"github.com/airport-panel/subscription/nodespec"
)

// FallbackRule 表示 Xray inbound 的 fallback 规则。
// Dest 支持三种格式：
//   - "@xhttp-internal"（abstract unix socket，internal inbound）
//   - "127.0.0.1:8080"（显式 IPv4 地址，推荐写法，不依赖隐式补全）
//   - "@vless-internal" 等（其他 internal inbound）
type FallbackRule struct {
	Path string `json:"path,omitempty"`
	Dest string `json:"dest"` // "@xhttp-internal" 或 "127.0.0.1:8080"
	Xver int    `json:"xver,omitempty"`
}

// InboundUnit 表示一个独立的 inbound 单元。
// Primary inbound 监听 0.0.0.0:443，带 fallbacks；
// Internal inbound 监听 abstract unix socket（@name），被 primary fallback 命中。
type InboundUnit struct {
	Listen    string                 `json:"listen,omitempty"` // "@name"=abstract unix socket（internal 用）
	Port      int                    `json:"port"`             // 443（primary 用）
	Protocol  nodespec.Protocol      `json:"protocol"`
	Tag       string                 `json:"tag"`
	Settings  map[string]interface{} `json:"settings"`
	Stream    map[string]interface{} `json:"stream,omitempty"`
	Sniffing  bool                   `json:"sniffing,omitempty"`
	Fallbacks []FallbackRule         `json:"fallbacks,omitempty"`
}

// InboundGroup 表示一组共享端口的 inbound 集合。
// 一个 Group 对应一个 VPS 上的 443 端口（或独立端口节点）。
type InboundGroup struct {
	ID       string         `json:"id"`                 // 通常是 VPS ID 或端口标识
	Port     int            `json:"port"`               // 443 或独立端口
	Listen   string         `json:"listen"`             // "0.0.0.0" 或 ""（默认 0.0.0.0）
	Primary  *InboundUnit   `json:"primary"`            // 主 inbound（TCP+REALITY+Vision，带 fallbacks）
	Internal []*InboundUnit `json:"internal,omitempty"` // 内部 inbound（unix socket）
}

// UserInfo 表示从 NodeSpec 提取的用户信息（用于 clients 合并）。
type UserInfo struct {
	UUID  string
	Email string
}

// ExtractUsersFromSpec 从 NodeSpec 的 Credentials 中提取用户信息。
// 只有 TCP+Vision 协议的节点会贡献到 primary clients（其他协议走 internal inbound）。
func ExtractUsersFromSpec(spec *nodespec.NodeSpec) []UserInfo {
	if spec == nil {
		return nil
	}
	// 只有 TCP 协议（Vision 直连）的节点用户进入 primary clients
	if spec.Transport.Type != nodespec.TransportTCP {
		return nil
	}
	switch c := spec.Credentials.(type) {
	case nodespec.VLESSCredentials:
		if c.UUID == "" {
			return nil
		}
		return []UserInfo{{UUID: c.UUID, Email: spec.Code + "@yundu"}}
	case *nodespec.VLESSCredentials:
		if c == nil || c.UUID == "" {
			return nil
		}
		return []UserInfo{{UUID: c.UUID, Email: spec.Code + "@yundu"}}
	case nodespec.VMessCredentials:
		if c.UUID == "" {
			return nil
		}
		return []UserInfo{{UUID: c.UUID, Email: spec.Code + "@yundu"}}
	case map[string]interface{}:
		uuid, _ := c["uuid"].(string)
		if uuid == "" {
			return nil
		}
		return []UserInfo{{UUID: uuid, Email: spec.Code + "@yundu"}}
	}
	return nil
}

// NormalizePath 统一 path 格式用于冲突检测。
// 规则：去掉末尾斜杠，空值返回 "/"。
// 这样 /path 和 /path/ 会被视为冲突（社区反复踩过的坑）。
func NormalizePath(p string) string {
	p = strings.TrimRight(p, "/")
	if p == "" {
		return "/"
	}
	return p
}

// extractFallbackPath 从 NodeSpec 提取走 fallback 的 path（XHTTP 或 WS）。
// 非 fallback 协议（TCP/gRPC/KCP/QUIC）返回空。
func extractFallbackPath(spec *nodespec.NodeSpec) string {
	if spec == nil {
		return ""
	}
	switch spec.Transport.Type {
	case nodespec.TransportXHTTP:
		if spec.Transport.XHTTP != nil {
			return spec.Transport.XHTTP.Path
		}
	case nodespec.TransportWS:
		if spec.Transport.WS != nil {
			return spec.Transport.WS.Path
		}
	case nodespec.TransportHTTPUpgrade:
		if spec.Transport.HTTPUpgrade != nil {
			return spec.Transport.HTTPUpgrade.Path
		}
	}
	return ""
}

// validatePathEdgeCases 检查 path 是否包含非法字符（query string / fragment）。
// Xray fallback-by-path 是精确匹配，path 中不应包含 ? 或 #。
func validatePathEdgeCases(path string) error {
	if strings.ContainsAny(path, "?#") {
		return fmt.Errorf("path %q 包含非法字符（?或#），fallback path 不应包含 query string 或 fragment", path)
	}
	return nil
}
