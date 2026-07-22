// Package dnsproviders 实现 ACME DNS-01 challenge 的 DNS provider 注册表。
//
// 移植自 Xboard-Node internal/cert/dnsproviders（commit 68ca2d2716a4a680eeb95f4b140f5a281d6614bc）。
// 当前阶段（A1）仅实现注册表框架 + Cloudflare provider，
// 阶段 B 将补全其余 20 个 provider（alidns/tencentcloud/route53 等）。
//
// 设计：
//   - 每个 provider 一个文件，init() 时注册到全局 registry
//   - Build 函数接收 env map[string]string，返回 certmagic.DNSProvider
//   - 调用方通过 Get(name) 查找 provider
package dnsproviders

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/caddyserver/certmagic"
)

// Provider 描述一个 DNS provider 的元信息与工厂。
type Provider struct {
	Name    string
	Aliases []string
	EnvVars []string
	Build   func(env map[string]string) (certmagic.DNSProvider, error)
}

var (
	registryMu sync.RWMutex
	registry   = map[string]*Provider{}
)

// Register 注册一个 DNS provider（通常在 init() 中调用）。
func Register(p *Provider) {
	if p == nil || p.Name == "" || p.Build == nil {
		return
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[p.Name] = p
	for _, alias := range p.Aliases {
		registry[strings.ToLower(alias)] = p
	}
}

// Get 按名称或别名查找 provider。
// 名称匹配大小写不敏感。
func Get(name string) (*Provider, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	p, ok := registry[strings.ToLower(strings.TrimSpace(name))]
	return p, ok
}

// CanonicalNames 返回所有 provider 的规范名称（排序后）。
func CanonicalNames() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	seen := map[string]bool{}
	names := make([]string, 0, len(registry))
	for _, p := range registry {
		if !seen[p.Name] {
			names = append(names, p.Name)
			seen[p.Name] = true
		}
	}
	sort.Strings(names)
	return names
}

// MustBuild 按名称查找并构建 provider，未找到时返回错误。
func MustBuild(name string, env map[string]string) (certmagic.DNSProvider, error) {
	p, ok := Get(name)
	if !ok {
		return nil, fmt.Errorf("unsupported dns_provider: %q (supported: %s)",
			name, strings.Join(CanonicalNames(), ", "))
	}
	return p.Build(env)
}
