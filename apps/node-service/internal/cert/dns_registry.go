package cert

import (
	"fmt"
	"sort"
	"sync"

	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/providers/dns/alidns"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/providers/dns/dnspod"
	"github.com/go-acme/lego/v4/providers/dns/gandi"
	"github.com/go-acme/lego/v4/providers/dns/namesilo"
)

// DNSProviderVar 描述一个 DNS provider 所需的凭证字段元信息
type DNSProviderVar struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	IsSecret    bool   `json:"is_secret"`
	Required    bool   `json:"required"`
}

// DNSProviderMeta 描述一个 DNS provider 的元信息（供前端渲染凭证表单）
type DNSProviderMeta struct {
	Name         string           `json:"name"`
	DisplayName  string           `json:"display_name"`
	Challenge    string           `json:"challenge"` // "dns-01"
	RequiredVars []DNSProviderVar `json:"required_vars"`
	OptionalVars []DNSProviderVar `json:"optional_vars,omitempty"`
}

// DNSProviderFactory 接收显式凭证 map，返回 lego challenge.Provider
// 凭证从 DB 解密后传入，不依赖环境变量
type DNSProviderFactory func(credentials map[string]string) (challenge.Provider, error)

// dnsProviderRegistry 全局注册表：provider name → meta
var dnsProviderRegistry = map[string]DNSProviderMeta{
	"cloudflare": {
		Name:        "cloudflare",
		DisplayName: "Cloudflare",
		Challenge:   "dns-01",
		RequiredVars: []DNSProviderVar{
			{Key: "api_token", Label: "API Token", IsSecret: true, Required: true, Description: "Cloudflare API Token（推荐，仅需 Zone.DNS 编辑权限）"},
		},
		OptionalVars: []DNSProviderVar{
			{Key: "zone_id", Label: "Zone Token", IsSecret: false, Required: false, Description: "可选，若使用独立 Zone Token"},
		},
	},
	"alidns": {
		Name:        "alidns",
		DisplayName: "阿里云 DNS (Alidns)",
		Challenge:   "dns-01",
		RequiredVars: []DNSProviderVar{
			{Key: "access_key_id", Label: "AccessKey ID", IsSecret: false, Required: true},
			{Key: "access_key_secret", Label: "AccessKey Secret", IsSecret: true, Required: true},
		},
	},
	"dnspod": {
		Name:        "dnspod",
		DisplayName: "腾讯云 DNSPod",
		Challenge:   "dns-01",
		RequiredVars: []DNSProviderVar{
			{Key: "api_token", Label: "API Token", IsSecret: true, Required: true, Description: "格式：ID,Token（如 12345,abcdef）"},
		},
	},
	"gandi": {
		Name:        "gandi",
		DisplayName: "Gandi",
		Challenge:   "dns-01",
		RequiredVars: []DNSProviderVar{
			{Key: "api_key", Label: "API Key", IsSecret: true, Required: true},
		},
	},
	"namesilo": {
		Name:        "namesilo",
		DisplayName: "NameSilo",
		Challenge:   "dns-01",
		RequiredVars: []DNSProviderVar{
			{Key: "api_key", Label: "API Key", IsSecret: true, Required: true},
		},
	},
	// === 阶段 B3: 补齐与 node-agent dnsproviders 对齐的 16 个 provider 元信息 ===
	// 以下 provider 仅有元信息（前端可渲染表单），工厂函数需添加 lego v4 依赖后启用。
	"acmedns": {
		Name:        "acmedns",
		DisplayName: "ACME-DNS",
		Challenge:   "dns-01",
		RequiredVars: []DNSProviderVar{
			{Key: "server_url", Label: "Server URL", IsSecret: false, Required: true, Description: "ACME-DNS 服务器地址（如 https://auth.acme-dns.io）"},
			{Key: "username", Label: "Username", IsSecret: false, Required: true},
			{Key: "password", Label: "Password", IsSecret: true, Required: true},
			{Key: "subdomain", Label: "Subdomain", IsSecret: false, Required: true},
		},
	},
	"tencentcloud": {
		Name:        "tencentcloud",
		DisplayName: "腾讯云 DNS（TC API）",
		Challenge:   "dns-01",
		RequiredVars: []DNSProviderVar{
			{Key: "secret_id", Label: "Secret ID", IsSecret: false, Required: true},
			{Key: "secret_key", Label: "Secret Key", IsSecret: true, Required: true},
		},
	},
	"route53": {
		Name:        "route53",
		DisplayName: "AWS Route53",
		Challenge:   "dns-01",
		RequiredVars: []DNSProviderVar{
			{Key: "access_key_id", Label: "Access Key ID", IsSecret: false, Required: true},
			{Key: "secret_access_key", Label: "Secret Access Key", IsSecret: true, Required: true},
			{Key: "region", Label: "Region", IsSecret: false, Required: true, Description: "如 us-east-1"},
		},
	},
	"azure": {
		Name:        "azure",
		DisplayName: "Azure DNS",
		Challenge:   "dns-01",
		RequiredVars: []DNSProviderVar{
			{Key: "client_id", Label: "Client ID", IsSecret: false, Required: true},
			{Key: "client_secret", Label: "Client Secret", IsSecret: true, Required: true},
			{Key: "tenant_id", Label: "Tenant ID", IsSecret: false, Required: true},
			{Key: "subscription_id", Label: "Subscription ID", IsSecret: false, Required: true},
		},
	},
	"digitalocean": {
		Name:        "digitalocean",
		DisplayName: "DigitalOcean",
		Challenge:   "dns-01",
		RequiredVars: []DNSProviderVar{
			{Key: "auth_token", Label: "Auth Token", IsSecret: true, Required: true},
		},
	},
	"googlecloud": {
		Name:        "googlecloud",
		DisplayName: "Google Cloud DNS",
		Challenge:   "dns-01",
		RequiredVars: []DNSProviderVar{
			{Key: "project", Label: "Project ID", IsSecret: false, Required: true},
			{Key: "service_account_json", Label: "Service Account JSON", IsSecret: true, Required: true, Description: "GCP 服务账号 JSON 密钥"},
		},
	},
	"hetzner": {
		Name:        "hetzner",
		DisplayName: "Hetzner",
		Challenge:   "dns-01",
		RequiredVars: []DNSProviderVar{
			{Key: "api_key", Label: "API Key", IsSecret: true, Required: true},
		},
	},
	"linode": {
		Name:        "linode",
		DisplayName: "Linode",
		Challenge:   "dns-01",
		RequiredVars: []DNSProviderVar{
			{Key: "token", Label: "API Token", IsSecret: true, Required: true},
		},
	},
	"ovh": {
		Name:        "ovh",
		DisplayName: "OVH",
		Challenge:   "dns-01",
		RequiredVars: []DNSProviderVar{
			{Key: "endpoint", Label: "Endpoint", IsSecret: false, Required: true, Description: "如 ovh-eu"},
			{Key: "application_key", Label: "Application Key", IsSecret: false, Required: true},
			{Key: "application_secret", Label: "Application Secret", IsSecret: true, Required: true},
			{Key: "consumer_key", Label: "Consumer Key", IsSecret: false, Required: true},
		},
	},
	"rfc2136": {
		Name:        "rfc2136",
		DisplayName: "RFC 2136 (BIND)",
		Challenge:   "dns-01",
		RequiredVars: []DNSProviderVar{
			{Key: "nameserver", Label: "Nameserver", IsSecret: false, Required: true, Description: "如 192.168.1.1:53"},
			{Key: "tsig_key", Label: "TSIG Key Name", IsSecret: false, Required: true},
			{Key: "tsig_algorithm", Label: "TSIG Algorithm", IsSecret: false, Required: true, Description: "如 hmac-sha256."},
			{Key: "tsig_secret", Label: "TSIG Secret", IsSecret: true, Required: true},
		},
	},
	"vultr": {
		Name:        "vultr",
		DisplayName: "Vultr",
		Challenge:   "dns-01",
		RequiredVars: []DNSProviderVar{
			{Key: "api_key", Label: "API Key", IsSecret: true, Required: true},
		},
	},
	"namecheap": {
		Name:        "namecheap",
		DisplayName: "Namecheap",
		Challenge:   "dns-01",
		RequiredVars: []DNSProviderVar{
			{Key: "api_user", Label: "API User", IsSecret: false, Required: true},
			{Key: "api_key", Label: "API Key", IsSecret: true, Required: true},
		},
	},
	"powerdns": {
		Name:        "powerdns",
		DisplayName: "PowerDNS",
		Challenge:   "dns-01",
		RequiredVars: []DNSProviderVar{
			{Key: "api_url", Label: "API URL", IsSecret: false, Required: true, Description: "如 http://127.0.0.1:8081"},
			{Key: "api_key", Label: "API Key", IsSecret: true, Required: true},
		},
	},
	"transip": {
		Name:        "transip",
		DisplayName: "TransIP",
		Challenge:   "dns-01",
		RequiredVars: []DNSProviderVar{
			{Key: "account_name", Label: "Account Name", IsSecret: false, Required: true},
			{Key: "private_key_path", Label: "Private Key Path", IsSecret: false, Required: true, Description: "TransIP 私钥文件路径"},
		},
	},
	"loopia": {
		Name:        "loopia",
		DisplayName: "Loopia",
		Challenge:   "dns-01",
		RequiredVars: []DNSProviderVar{
			{Key: "api_user", Label: "API User", IsSecret: false, Required: true},
			{Key: "api_password", Label: "API Password", IsSecret: true, Required: true},
		},
	},
	"netcup": {
		Name:        "netcup",
		DisplayName: "Netcup",
		Challenge:   "dns-01",
		RequiredVars: []DNSProviderVar{
			{Key: "customer_number", Label: "Customer Number", IsSecret: false, Required: true},
			{Key: "api_key", Label: "API Key", IsSecret: true, Required: true},
			{Key: "api_password", Label: "API Password", IsSecret: true, Required: true},
		},
	},
	"scaleway": {
		Name:        "scaleway",
		DisplayName: "Scaleway",
		Challenge:   "dns-01",
		RequiredVars: []DNSProviderVar{
			{Key: "access_key", Label: "Access Key", IsSecret: false, Required: true},
			{Key: "secret_key", Label: "Secret Key", IsSecret: true, Required: true},
		},
	},
}

// dnsProviderFactories 全局工厂表：provider name → factory
var dnsProviderFactories = map[string]DNSProviderFactory{
	"cloudflare": func(c map[string]string) (challenge.Provider, error) {
		// 无显式凭证时退化为环境变量模式（兼容旧版 ACME_DNS_PROVIDER=cloudflare 部署）
		if c["api_token"] == "" && c["zone_id"] == "" {
			return cloudflare.NewDNSProvider()
		}
		cfg := cloudflare.NewDefaultConfig()
		if tok := c["api_token"]; tok != "" {
			cfg.AuthToken = tok
		}
		if zid := c["zone_id"]; zid != "" {
			cfg.ZoneToken = zid
		}
		return cloudflare.NewDNSProviderConfig(cfg)
	},
	"alidns": func(c map[string]string) (challenge.Provider, error) {
		cfg := alidns.NewDefaultConfig()
		cfg.APIKey = c["access_key_id"]
		cfg.SecretKey = c["access_key_secret"]
		return alidns.NewDNSProviderConfig(cfg)
	},
	"dnspod": func(c map[string]string) (challenge.Provider, error) {
		cfg := dnspod.NewDefaultConfig()
		cfg.LoginToken = c["api_token"]
		return dnspod.NewDNSProviderConfig(cfg)
	},
	"gandi": func(c map[string]string) (challenge.Provider, error) {
		cfg := gandi.NewDefaultConfig()
		cfg.APIKey = c["api_key"]
		return gandi.NewDNSProviderConfig(cfg)
	},
	"namesilo": func(c map[string]string) (challenge.Provider, error) {
		cfg := namesilo.NewDefaultConfig()
		cfg.APIKey = c["api_key"]
		return namesilo.NewDNSProviderConfig(cfg)
	},
}

// registryMu 保护并发读取注册表快照
var registryMu sync.RWMutex

// ListDNSProviders 返回注册表中所有 DNS provider 元信息（按 name 排序）
func ListDNSProviders() []DNSProviderMeta {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]DNSProviderMeta, 0, len(dnsProviderRegistry))
	for _, m := range dnsProviderRegistry {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// GetDNSProviderMeta 查询单个 provider 元信息
func GetDNSProviderMeta(name string) (DNSProviderMeta, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	m, ok := dnsProviderRegistry[name]
	return m, ok
}

// CreateDNSProvider 按 provider name 创建 challenge.Provider
// credentials 为明文凭证 map（调用方负责解密）
func CreateDNSProvider(name string, credentials map[string]string) (challenge.Provider, error) {
	registryMu.RLock()
	factory, ok := dnsProviderFactories[name]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("dns provider %q 的工厂未实现（面板仅支持 cloudflare/alidns/dnspod/gandi/namesilo 的 ACME 签发；其余 provider 请通过 agent 端 certmagic 签发或手动上传 PEM）", name)
	}
	return factory(credentials)
}

// ValidateDNSProviderCredentials 校验凭证是否满足 provider 的必填字段
func ValidateDNSProviderCredentials(name string, credentials map[string]string) error {
	meta, ok := GetDNSProviderMeta(name)
	if !ok {
		return fmt.Errorf("unknown dns provider: %s", name)
	}
	for _, v := range meta.RequiredVars {
		if credentials[v.Key] == "" {
			return fmt.Errorf("missing required credential '%s' for provider %s", v.Key, name)
		}
	}
	return nil
}
