package nodespec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type KernelCompatLevel string

const (
	CompatBoth         KernelCompatLevel = "both"
	CompatXrayOnly     KernelCompatLevel = "xray_only"
	CompatSingboxOnly  KernelCompatLevel = "singbox_only"
	CompatExperimental KernelCompatLevel = "experimental"
)

// EnhancementCompatLevel 对齐 v2 设计的 CompatLevel，
// 声明 uTLS/ECH/Mux 在某协议预设下的适用性。
type EnhancementCompatLevel string

const (
	// EnhRecommended 推荐启用（如 REALITY 场景下的 uTLS）
	EnhRecommended EnhancementCompatLevel = "recommended"
	// EnhOptional 可选启用（如 Trojan+TLS 下的 Mux）
	EnhOptional EnhancementCompatLevel = "optional"
	// EnhMandatory 强制必须启用（如 REALITY 依赖 uTLS 指纹）
	EnhMandatory EnhancementCompatLevel = "mandatory"
	// EnhNotApplicable 不适用（如 REALITY 下的 ECH、QUIC 协议下的 Mux）
	EnhNotApplicable EnhancementCompatLevel = "not_applicable"
	// EnhRequiresSingBox 仅 Sing-box 内核支持（如 Hysteria2/TUIC 的 ECH）
	EnhRequiresSingBox EnhancementCompatLevel = "requires_sing_box"
)

// EnhancementSpec 对齐 v2 设计的 EnhancementCompat，
// 声明协议预设的 uTLS/ECH/Mux 适用性（Layer 4 横切增强层）。
type EnhancementSpec struct {
	UTLS      EnhancementCompatLevel `json:"utls,omitempty" yaml:"utls,omitempty"`
	ECH       EnhancementCompatLevel `json:"ech,omitempty" yaml:"ech,omitempty"`
	Multiplex EnhancementCompatLevel `json:"multiplex,omitempty" yaml:"multiplex,omitempty"`
}

type PresetBadge string

const (
	PresetBadgeRecommended  PresetBadge = "主推"
	PresetBadgeBalanced     PresetBadge = "均衡"
	PresetBadgeCDN          PresetBadge = "CDN友好"
	PresetBadgeExperimental PresetBadge = "实验性"
	PresetBadgeDeprecated   PresetBadge = "已弃用"
	PresetBadgeNew          PresetBadge = "新协议"
)

// DeploymentProfile 对齐 v2 设计，描述协议预设的部署画像。
type DeploymentProfile string

const (
	ProfileDirect  DeploymentProfile = "direct"
	ProfileCFSaaS  DeploymentProfile = "cf_saas"
	ProfileCFArgo   DeploymentProfile = "cf_argo"
	ProfileHybrid   DeploymentProfile = "hybrid"
	ProfileOverlay  DeploymentProfile = "overlay"
)

type PresetTemplate struct {
	ID                  string            `json:"id" yaml:"id"`
	Name                string            `json:"name" yaml:"name"`
	Badge               PresetBadge       `json:"badge,omitempty" yaml:"badge,omitempty"`
	Description         string            `json:"description" yaml:"description"`
	Protocol            Protocol          `json:"protocol" yaml:"protocol"`
	Transport           Transport         `json:"transport" yaml:"transport"`
	Security            Security          `json:"security" yaml:"security"`
	MinXrayVersion      string            `json:"min_xray_version,omitempty" yaml:"min_xray_version,omitempty"`
	MinSingboxVersion   string            `json:"min_singbox_version,omitempty" yaml:"min_singbox_version,omitempty"`
	ClientSupport       []string          `json:"client_support" yaml:"client_support"`
	KernelCompat        KernelCompatLevel `json:"kernel_compat" yaml:"kernel_compat"`
	BaseSpec            NodeSpec          `json:"base_spec" yaml:"base_spec"`
	Recommendations     []string          `json:"recommendations,omitempty" yaml:"recommendations,omitempty"`
	Warnings            []string          `json:"warnings,omitempty" yaml:"warnings,omitempty"`
	DeprecatedAt        *time.Time        `json:"deprecated_at,omitempty" yaml:"deprecated_at,omitempty"`
	UpdatedFromUpstream time.Time         `json:"updated_from_upstream" yaml:"updated_from_upstream"`

	// v2 设计新增字段
	// DeploymentProfile 部署画像（direct/cf_saas/cf_argo/hybrid/overlay）
	DeploymentProfile DeploymentProfile `json:"deployment_profile,omitempty" yaml:"deployment_profile,omitempty"`
	// ForbiddenCombos 禁止的部署组合及原因（key=DeploymentProfile, value=原因）
	ForbiddenCombos map[DeploymentProfile]string `json:"forbidden_combos,omitempty" yaml:"forbidden_combos,omitempty"`
	// Enhancement v2 横切增强层声明（uTLS/ECH/Mux 适用性）
	Enhancement *EnhancementSpec `json:"enhancement,omitempty" yaml:"enhancement,omitempty"`
	// UIWarning 高危字段警告（如暴露 VPS 真实 IP）
	UIWarning string `json:"ui_warning,omitempty" yaml:"ui_warning,omitempty"`
}

func (p *PresetTemplate) IsDeprecated() bool {
	return p.DeprecatedAt != nil && p.DeprecatedAt.Before(time.Now())
}

func (p *PresetTemplate) IsStale(maxAgeDays int) bool {
	return time.Since(p.UpdatedFromUpstream) > time.Duration(maxAgeDays)*24*time.Hour
}

func (p *PresetTemplate) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("preset id is required")
	}
	if p.Name == "" {
		return fmt.Errorf("preset name is required")
	}
	if !ValidProtocols[p.Protocol] {
		return fmt.Errorf("preset %s: invalid protocol %s", p.ID, p.Protocol)
	}
	if !ValidTransports[p.Transport] {
		return fmt.Errorf("preset %s: invalid transport %s", p.ID, p.Transport)
	}
	if !ValidSecurity[p.Security] {
		return fmt.Errorf("preset %s: invalid security %s", p.ID, p.Security)
	}
	if len(p.ClientSupport) == 0 {
		return fmt.Errorf("preset %s: client_support is required", p.ID)
	}
	switch p.KernelCompat {
	case CompatBoth, CompatXrayOnly, CompatSingboxOnly, CompatExperimental:
	default:
		return fmt.Errorf("preset %s: invalid kernel_compat %s", p.ID, p.KernelCompat)
	}
	return nil
}

func (p *PresetTemplate) ApplyToSpec(target *NodeSpec) {
	if target == nil {
		return
	}
	if target.Protocol == "" {
		target.Protocol = p.BaseSpec.Protocol
	}
	if target.Transport.Type == "" {
		target.Transport = p.BaseSpec.Transport
	}
	if target.Security == "" {
		target.Security = p.BaseSpec.Security
	}
	if p.BaseSpec.TLS != nil && target.TLS == nil {
		target.TLS = p.BaseSpec.TLS
	}
	if p.BaseSpec.Reality != nil && target.Reality == nil {
		target.Reality = p.BaseSpec.Reality
	}
	if target.AllowUDP == false && p.BaseSpec.AllowUDP {
		target.AllowUDP = true
	}
	if target.TrafficRate == 0 {
		target.TrafficRate = 1.0
	}
	if target.PresetID == "" {
		target.PresetID = p.ID
	}
}

type PresetDiff map[string]PresetDiffEntry

type PresetDiffEntry struct {
	Field    string      `json:"field"`
	Preset   interface{} `json:"preset_value"`
	Current  interface{} `json:"current_value"`
	Modified bool        `json:"modified"`
}

func DiffFromPreset(current *NodeSpec, preset *PresetTemplate) PresetDiff {
	diff := make(PresetDiff)
	compareValues(diff, "protocol", string(preset.BaseSpec.Protocol), string(current.Protocol))
	compareValues(diff, "transport.type", string(preset.BaseSpec.Transport.Type), string(current.Transport.Type))
	compareValues(diff, "security", string(preset.BaseSpec.Security), string(current.Security))
	if preset.BaseSpec.TLS != nil && current.TLS != nil {
		compareValues(diff, "tls.sni", preset.BaseSpec.TLS.SNI, current.TLS.SNI)
		compareValues(diff, "tls.fingerprint", preset.BaseSpec.TLS.Fingerprint, current.TLS.Fingerprint)
		compareValues(diff, "tls.cert_mode", string(preset.BaseSpec.TLS.CertMode), string(current.TLS.CertMode))
	}
	if preset.BaseSpec.Transport.WS != nil && current.Transport.WS != nil {
		compareValues(diff, "transport.ws.path", preset.BaseSpec.Transport.WS.Path, current.Transport.WS.Path)
		compareValues(diff, "transport.ws.host", preset.BaseSpec.Transport.WS.Host, current.Transport.WS.Host)
	}
	if preset.BaseSpec.Transport.XHTTP != nil && current.Transport.XHTTP != nil {
		compareValues(diff, "transport.xhttp.mode", preset.BaseSpec.Transport.XHTTP.Mode, current.Transport.XHTTP.Mode)
	}
	if preset.BaseSpec.Transport.GRPC != nil && current.Transport.GRPC != nil {
		compareValues(diff, "transport.grpc.service_name", preset.BaseSpec.Transport.GRPC.ServiceName, current.Transport.GRPC.ServiceName)
	}
	if preset.BaseSpec.Transport.Mux != nil && current.Transport.Mux != nil {
		compareValues(diff, "transport.mux.enabled", preset.BaseSpec.Transport.Mux.Enabled, current.Transport.Mux.Enabled)
	}
	if preset.BaseSpec.Transport.TCPBrutal != nil && current.Transport.TCPBrutal != nil {
		compareValues(diff, "transport.tcp_brutal.enabled", preset.BaseSpec.Transport.TCPBrutal.Enabled, current.Transport.TCPBrutal.Enabled)
	}
	return diff
}

func compareValues(diff PresetDiff, field string, preset, current interface{}) {
	modified := !reflect.DeepEqual(preset, current)
	if !modified {
		return
	}
	diff[field] = PresetDiffEntry{
		Field:    field,
		Preset:   preset,
		Current:  current,
		Modified: modified,
	}
}

func (d PresetDiff) ModifiedFields() []string {
	var fields []string
	for k, v := range d {
		if v.Modified {
			fields = append(fields, k)
		}
	}
	return fields
}

func (d PresetDiff) HasModifications() bool {
	return len(d.ModifiedFields()) > 0
}

type PresetRegistry struct {
	presets map[string]*PresetTemplate
}

func NewPresetRegistry() *PresetRegistry {
	return &PresetRegistry{
		presets: make(map[string]*PresetTemplate),
	}
}

func (r *PresetRegistry) Register(p *PresetTemplate) error {
	if err := p.Validate(); err != nil {
		return err
	}
	r.presets[p.ID] = p
	return nil
}

func (r *PresetRegistry) Get(id string) (*PresetTemplate, bool) {
	p, ok := r.presets[id]
	return p, ok
}

func (r *PresetRegistry) List() []*PresetTemplate {
	result := make([]*PresetTemplate, 0, len(r.presets))
	for _, p := range r.presets {
		result = append(result, p)
	}
	return result
}

func (r *PresetRegistry) ListByProtocol(proto Protocol) []*PresetTemplate {
	var result []*PresetTemplate
	for _, p := range r.presets {
		if p.Protocol == proto {
			result = append(result, p)
		}
	}
	return result
}

func (r *PresetRegistry) ListCompatible(kernel string) []*PresetTemplate {
	var result []*PresetTemplate
	for _, p := range r.presets {
		if p.IsDeprecated() {
			continue
		}
		switch kernel {
		case "xray":
			if p.KernelCompat == CompatBoth || p.KernelCompat == CompatXrayOnly || p.KernelCompat == CompatExperimental {
				result = append(result, p)
			}
		case "singbox":
			if p.KernelCompat == CompatBoth || p.KernelCompat == CompatSingboxOnly {
				result = append(result, p)
			}
		}
	}
	return result
}

func (r *PresetRegistry) LoadFromDirectory(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read preset directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return fmt.Errorf("read preset %s: %w", entry.Name(), err)
		}
		var p PresetTemplate
		switch ext {
		case ".json":
			if err := json.Unmarshal(data, &p); err != nil {
				return fmt.Errorf("parse preset %s: %w", entry.Name(), err)
			}
		default:
			if err := yaml.Unmarshal(data, &p); err != nil {
				return fmt.Errorf("parse preset %s: %w", entry.Name(), err)
			}
		}
		if err := r.Register(&p); err != nil {
			return fmt.Errorf("register preset %s: %w", entry.Name(), err)
		}
	}
	return nil
}

// REALITY 默认伪装 SNI（用户规范：统一使用 mesu.apple.com，支持 TLS1.3+H2）
const defaultRealitySNI = "mesu.apple.com"

func BuildDefaultPresets() []*PresetTemplate {
	// B24: 使用当前日期替代硬编码时间戳。
	// 2026-07-13 重建：对齐三域名·单VPS·16协议完整服务端配置手册，
	// 补齐 P01-P17 标准 ID，修正 XHTTP mode 禁用 auto、REALITY SNI 默认值、
	// P06/P07 downloadSettings、P09/P17 XMUX 等历史遗留错误。
	now := time.Now()
	return []*PresetTemplate{
		// ===== P01: VLESS+TCP+REALITY+Vision（direct）=====
		{
			ID:                "P01-vless-reality-vision",
			Name:              "VLESS + TCP + REALITY + Vision",
			Badge:             PresetBadgeRecommended,
			Description:       "VLESS+TCP+REALITY+Vision 直连抗封锁最佳方案，REALITY伪装+Vision流控，无需域名证书",
			Protocol:          ProtocolVLESS,
			Transport:         TransportTCP,
			Security:          SecurityReality,
			MinXrayVersion:    "1.8.0",
			MinSingboxVersion: "1.8.0",
			ClientSupport:     []string{"v2rayNG", "Shadowrocket", "Clash Meta", "sing-box", "Nekoray"},
			KernelCompat:      CompatBoth,
			DeploymentProfile: ProfileDirect,
			ForbiddenCombos: map[DeploymentProfile]string{
				ProfileCFSaaS: "REALITY需完整TLS握手，CF会终结TLS破坏协议",
				ProfileCFArgo: "同上",
			},
			Enhancement: &EnhancementSpec{
				UTLS:      EnhMandatory,
				ECH:       EnhNotApplicable,
				Multiplex: EnhOptional,
			},
			BaseSpec: NodeSpec{
				Protocol: ProtocolVLESS,
				Port:     443,
				Transport: TransportConfig{
					Type: TransportTCP,
				},
				Security:    SecurityReality,
				AllowUDP:    true,
				TrafficRate: 1.0,
				IsVisible:   true,
				Reality: &RealityConfig{
					SNI:         defaultRealitySNI,
					Fingerprint: "chrome",
				},
				Credentials: VLESSCredentials{
					Flow: FlowXTLSRprxVision,
				},
			},
			Recommendations: []string{
				"REALITY无需域名和证书，dest域名建议选TLS1.3+H2+同ASN的大站",
				"short_id建议用openssl rand -hex 4生成",
				"Vision流控需客户端和服务端同时支持",
			},
			Warnings: []string{
				"REALITY dest 域名必须与 SNI 一致，否则握手失败",
				"不要使用国内CDN，REALITY必须直连或使用IPLC专线",
			},
			UpdatedFromUpstream: now,
		},
		// ===== P02: Trojan+TLS（direct）=====
		{
			ID:                "P02-trojan-tls",
			Name:              "Trojan + TLS",
			Badge:             PresetBadgeBalanced,
			Description:       "Trojan+TCP+TLS 标准直连协议，模拟 HTTPS 流量，简单稳定",
			Protocol:          ProtocolTrojan,
			Transport:         TransportTCP,
			Security:          SecurityTLS,
			MinXrayVersion:    "1.8.0",
			MinSingboxVersion: "1.8.0",
			ClientSupport:     []string{"v2rayNG", "Shadowrocket", "Clash Meta", "sing-box", "Nekoray", "Hiddify"},
			KernelCompat:      CompatBoth,
			DeploymentProfile: ProfileDirect,
			BaseSpec: NodeSpec{
				Protocol: ProtocolTrojan,
				Port:     443,
				Transport: TransportConfig{
					Type: TransportTCP,
				},
				Security:    SecurityTLS,
				AllowUDP:    false,
				TrafficRate: 1.0,
				IsVisible:   true,
				TLS: &TLSConfig{
					ALPN:        []string{"http/1.1"},
					Fingerprint: "chrome",
					CertMode:    TLSCertModeACME,
				},
				Credentials: TrojanCredentials{},
			},
			Recommendations: []string{
				"alpn 建议只用 http/1.1，避免 h2 特征暴露",
				"需配置真实证书（ACME 或 CF Origin CA）",
			},
			Warnings: []string{"Trojan 为 TCP 直连，VPS IP 会暴露"},
			UpdatedFromUpstream: now,
		},
		// ===== P03: VLESS+WS+TLS（cf_saas）=====
		{
			ID:                "P03-vless-ws-tls",
			Name:              "VLESS + WebSocket + TLS (CDN)",
			Badge:             PresetBadgeCDN,
			Description:       "CDN中转首选方案，WebSocket over TLS可过Cloudflare/CDN中转，支持Nginx反代",
			Protocol:          ProtocolVLESS,
			Transport:         TransportWS,
			Security:          SecurityTLS,
			MinXrayVersion:    "1.8.0",
			MinSingboxVersion: "1.8.0",
			ClientSupport:     []string{"v2rayNG", "Shadowrocket", "Clash Meta", "sing-box", "Nekoray", "V2RayN"},
			KernelCompat:      CompatBoth,
			DeploymentProfile: ProfileCFSaaS,
			BaseSpec: NodeSpec{
				Protocol:   ProtocolVLESS,
				Port:       443,
				ClientPort: 443,
				ServerPort: 8446,
				Transport: TransportConfig{
					Type: TransportWS,
					WS: &WSConfig{
						Path: "/ws-vless",
					},
				},
				Security:    SecurityTLS,
				AllowUDP:    false,
				TrafficRate: 1.0,
				IsVisible:   true,
				TLS: &TLSConfig{
					ALPN:        []string{"h2", "http/1.1"},
					Fingerprint: "chrome",
					CertMode:    TLSCertModeACME,
				},
			},
			Recommendations: []string{
				"CDN中转时 client_port=443，server_port=本地监听（8446+），Nginx反向代理到本机",
				"Path建议使用随机路径如/abc123xyz避免被探测",
			},
			Warnings: []string{
				"Path 必须改为随机字符串，避免被探测",
				"CF 面板需确认 WebSocket 开关为 ON",
			},
			UpdatedFromUpstream: now,
		},
		// ===== P04: Trojan+WS+TLS（cf_saas）=====
		{
			ID:                "P04-trojan-ws-tls",
			Name:              "Trojan + WebSocket + TLS (CDN)",
			Badge:             PresetBadgeBalanced,
			Description:       "兼容老客户端的CDN方案，Trojan协议广泛兼容各类客户端",
			Protocol:          ProtocolTrojan,
			Transport:         TransportWS,
			Security:          SecurityTLS,
			MinXrayVersion:    "1.8.0",
			MinSingboxVersion: "1.8.0",
			ClientSupport:     []string{"v2rayNG", "Shadowrocket", "Clash Meta", "sing-box", "Nekoray"},
			KernelCompat:      CompatBoth,
			DeploymentProfile: ProfileCFSaaS,
			BaseSpec: NodeSpec{
				Protocol:   ProtocolTrojan,
				Port:       443,
				ClientPort: 443,
				ServerPort: 8447,
				Transport: TransportConfig{
					Type: TransportWS,
					WS: &WSConfig{
						Path: "/ws-trojan",
					},
				},
				Security:    SecurityTLS,
				AllowUDP:    false,
				TrafficRate: 1.0,
				IsVisible:   true,
				TLS: &TLSConfig{
					ALPN:        []string{"h2", "http/1.1"},
					Fingerprint: "chrome",
					CertMode:    TLSCertModeACME,
				},
				Credentials: TrojanCredentials{},
			},
			Warnings: []string{
				"Path 必须改为随机字符串，避免被探测",
				"CF WAF/Bot Fight Mode 可能误判，建议为 path 加白名单",
			},
			UpdatedFromUpstream: now,
		},
		// ===== P05: AnyTLS（direct，sing-box only）=====
		{
			ID:                "P05-anytls",
			Name:              "AnyTLS",
			Badge:             PresetBadgeNew,
			Description:       "新TLS代理协议，专门设计用于对抗TLS指纹识别，sing-box 独有",
			Protocol:          ProtocolAnyTLS,
			Transport:         TransportTCP,
			Security:          SecurityTLS,
			MinSingboxVersion: "1.10.0",
			ClientSupport:     []string{"sing-box", "Hiddify"},
			KernelCompat:      CompatSingboxOnly,
			DeploymentProfile: ProfileDirect,
			BaseSpec: NodeSpec{
				Protocol:    ProtocolAnyTLS,
				Port:        443,
				Security:    SecurityTLS,
				TrafficRate: 1.0,
				IsVisible:   true,
				TLS: &TLSConfig{
					ALPN:        []string{"h2", "http/1.1"},
					Fingerprint: "chrome",
					CertMode:    TLSCertModeACME,
				},
				Credentials: AnyTLSCredentials{},
			},
			Warnings: []string{
				"AnyTLS目前仅Sing-box支持，Xray暂不兼容",
				"AnyTLS outbound 必须 TLS，不能与 TCP Fast Open 同用",
				"idle_session_timeout 默认 15s，idle_session_check_interval 默认 30s",
			},
			UpdatedFromUpstream: now,
		},
		// ===== P06: XHTTP 上行CDN | 下行REALITY（hybrid）=====
		{
			ID:                "P06-xhttp-up-cdn-down-reality",
			Name:              "XHTTP 上行CDN | 下行REALITY (异SNI)",
			Badge:             PresetBadgeRecommended,
			Description:       "XHTTP split模式：上行走CDN(明文stream-up)，下行直连REALITY，异SNI隔离。抗封锁能力最强",
			Protocol:          ProtocolVLESS,
			Transport:         TransportXHTTP,
			Security:          SecurityTLS,
			MinXrayVersion:    "26.3.27",
			ClientSupport:     []string{"v2rayNG", "Xray"},
			KernelCompat:      CompatXrayOnly,
			DeploymentProfile: ProfileHybrid,
			ForbiddenCombos: map[DeploymentProfile]string{
				ProfileCFSaaS: "hybrid 模式下行必须直连，无法整体套CDN",
				ProfileCFArgo: "同上",
			},
			Enhancement: &EnhancementSpec{
				UTLS:      EnhMandatory,
				ECH:       EnhOptional,
				Multiplex: EnhRecommended,
			},
			UIWarning: "此配置会暴露落地VPS真实IP，仅推荐用于高级用户自选线路",
			BaseSpec: NodeSpec{
				Protocol:    ProtocolVLESS,
				Port:        443,
				AllowUDP:    true,
				TrafficRate: 1.0,
				IsVisible:   true,
				Transport: TransportConfig{
					Type: TransportXHTTP,
					XHTTP: &XHTTPConfig{
						Path:         "/xh",
						Mode:         "stream-up",
						NoGRPCHeader: true,
						DownloadSettings: &XHTTPDownloadConfig{
							Address:  "",
							Port:     443,
							Network:  TransportXHTTP,
							Security: SecurityReality,
							Path:     "/dl",
							Mode:     "stream-down",
							Reality: &RealityConfig{
								SNI:         defaultRealitySNI,
								Fingerprint: "chrome",
							},
						},
					},
				},
				Security: SecurityTLS,
				TLS: &TLSConfig{
					Fingerprint: "chrome",
					CertMode:    TLSCertModeACME,
				},
				Credentials: VLESSCredentials{},
			},
			Recommendations: []string{
				"上行经CDN加速，下行直连REALITY保护，兼顾速度与抗封锁",
				"nginx需配置grpc_pass反代上行，ssl_preread分流下行",
				"CDN域名需开启小黄云(Proxied)",
			},
			Warnings: []string{
				"需nginx同时配置443(http2+grpc_pass)和8443(stream ssl_preread)两个端口",
				"上行和下行使用不同SNI，必须配置正确",
				"仅 Xray 内核支持，Sing-box 不支持 xhttp",
			},
			UpdatedFromUpstream: now,
		},
		// ===== P07: XHTTP 上行REALITY | 下行CDN（hybrid）=====
		{
			ID:                "P07-xhttp-up-reality-down-cdn",
			Name:              "XHTTP 上行REALITY | 下行CDN (异SNI)",
			Badge:             PresetBadgeRecommended,
			Description:       "XHTTP split模式反向：上行直连REALITY，下行走CDN，异SNI隔离。与 P06 互补",
			Protocol:          ProtocolVLESS,
			Transport:         TransportXHTTP,
			Security:          SecurityReality,
			MinXrayVersion:    "26.3.27",
			ClientSupport:     []string{"v2rayNG", "Xray"},
			KernelCompat:      CompatXrayOnly,
			DeploymentProfile: ProfileHybrid,
			ForbiddenCombos: map[DeploymentProfile]string{
				ProfileCFSaaS: "hybrid 模式上行必须直连，无法整体套CDN",
				ProfileCFArgo: "同上",
			},
			Enhancement: &EnhancementSpec{
				UTLS:      EnhMandatory,
				ECH:       EnhOptional,
				Multiplex: EnhRecommended,
			},
			UIWarning: "此配置上行暴露VPS真实IP，仅推荐用于高级用户自选线路",
			BaseSpec: NodeSpec{
				Protocol:    ProtocolVLESS,
				Port:        8443,
				AllowUDP:    true,
				TrafficRate: 1.0,
				IsVisible:   true,
				Transport: TransportConfig{
					Type: TransportXHTTP,
					XHTTP: &XHTTPConfig{
						Path:         "/xh-up",
						Mode:         "packet-up",
						NoGRPCHeader: true,
						DownloadSettings: &XHTTPDownloadConfig{
							Address:  "",
							Port:     443,
							Network:  TransportXHTTP,
							Security: SecurityTLS,
							Path:     "/dl",
							Mode:     "stream-down",
							TLS: &TLSConfig{
								Fingerprint: "chrome",
								CertMode:    TLSCertModeACME,
							},
						},
					},
				},
				Security: SecurityReality,
				Reality: &RealityConfig{
					SNI:         defaultRealitySNI,
					Fingerprint: "chrome",
				},
				Credentials: VLESSCredentials{},
			},
			Recommendations: []string{
				"与 P06 互补，上行 REALITY 抗封锁，下行 CDN 加速",
				"nginx 需同时配置 8443(stream ssl_preread 上行) 和 443(http2+grpc_pass 下行)",
			},
			Warnings: []string{
				"上行和下行使用不同SNI，必须配置正确",
				"仅 Xray 内核支持，Sing-box 不支持 xhttp",
			},
			UpdatedFromUpstream: now,
		},
		// ===== P08: XHTTP+TLS+CDN 纯CDN（cf_saas）=====
		{
			ID:                "P08-xhttp-tls-cdn",
			Name:              "XHTTP + TLS + CDN (纯CDN)",
			Badge:             PresetBadgeCDN,
			Description:       "VLESS+XHTTP+TLS 纯CDN模式，packet-up上行，适合 VPS IP 已被墙的兜底方案",
			Protocol:          ProtocolVLESS,
			Transport:         TransportXHTTP,
			Security:          SecurityTLS,
			MinXrayVersion:    "26.3.27",
			MinSingboxVersion: "1.10.0",
			ClientSupport:     []string{"v2rayNG", "Shadowrocket", "sing-box", "Xray"},
			KernelCompat:      CompatBoth,
			DeploymentProfile: ProfileCFSaaS,
			Enhancement: &EnhancementSpec{
				UTLS:      EnhOptional,
				ECH:       EnhRecommended,
				Multiplex: EnhRecommended,
			},
			BaseSpec: NodeSpec{
				Protocol:    ProtocolVLESS,
				Port:        443,
				AllowUDP:    true,
				TrafficRate: 1.0,
				IsVisible:   true,
				Transport: TransportConfig{
					Type: TransportXHTTP,
					XHTTP: &XHTTPConfig{
						Path:         "/xhttp-cdn",
						Mode:         "packet-up",
						NoGRPCHeader: true,
					},
				},
				Security: SecurityTLS,
				TLS: &TLSConfig{
					Fingerprint: "chrome",
					CertMode:    TLSCertModeACME,
				},
				Credentials: VLESSCredentials{},
			},
			Recommendations: []string{
				"VPS IP 被墙时的最终兜底方案，牺牲部分速度换绝对连通",
				"强制 packet-up 模式，禁止 auto（会导致连接不稳定）",
				"nginx 需关闭 proxy_buffering，CF 需关闭 http2_to_origin",
			},
			Warnings: []string{
				"禁止使用 auto 模式，会导致连接不稳定",
				"nginx 必须配置 proxy_buffering off",
				"CF 后台需关闭 HTTP/2 to Origin",
			},
			UpdatedFromUpstream: now,
		},
		// ===== P09: XHTTP stream-up+REALITY+XMUX（direct）=====
		{
			ID:                "P09-xhttp-stream-up-reality-xmux",
			Name:              "XHTTP stream-up + REALITY + XMUX (天花板)",
			Badge:             PresetBadgeExperimental,
			Description:       "XHTTP STREAM-UP+REALITY+XMUX 多路复用，追求极致速度的天花板配置，IP完全暴露",
			Protocol:          ProtocolVLESS,
			Transport:         TransportXHTTP,
			Security:          SecurityReality,
			MinXrayVersion:    "26.3.27",
			ClientSupport:     []string{"v2rayNG", "Xray"},
			KernelCompat:      CompatExperimental,
			DeploymentProfile: ProfileDirect,
			ForbiddenCombos: map[DeploymentProfile]string{
				ProfileCFSaaS: "stream-up+REALITY架构下行必须直连，无法整体套CDN",
				ProfileCFArgo: "同上",
			},
			Enhancement: &EnhancementSpec{
				UTLS:      EnhMandatory,
				ECH:       EnhNotApplicable,
				Multiplex: EnhRecommended,
			},
			UIWarning: "IP完全暴露，仅用于VPS来源清洁、追求极致速度的高级直连线路",
			BaseSpec: NodeSpec{
				Protocol:    ProtocolVLESS,
				Port:        443,
				AllowUDP:    true,
				TrafficRate: 1.0,
				IsVisible:   true,
				Transport: TransportConfig{
					Type: TransportXHTTP,
					XHTTP: &XHTTPConfig{
						Path:         "/su",
						Mode:         "stream-up",
						NoGRPCHeader: true,
						Headers:      map[string]string{"Referer": "https://www.google.com/"},
					},
					Mux: &MuxConfig{
						Enabled:         true,
						Protocol:        MuxProtocolXmux,
						MaxConnections:  32,
						MaxConcurrency:  "16-32",
						CMaxReuseTimes:  "64-128",
						HMaxRequestTimes: "128-256",
						HMaxReusableSecs: "10-30",
						Padding:         true,
						KeepAlivePeriod: 30,
					},
					Sockopt: &SockoptConfig{
						TCPFastOpen:  true,
						TCPMultipath: true,
						Congestion:   "bbr",
						TCPKeepAlive: 60,
					},
				},
				Security: SecurityReality,
				Reality: &RealityConfig{
					SNI:         defaultRealitySNI,
					Fingerprint: "chrome",
				},
				Credentials: VLESSCredentials{},
			},
			Recommendations: []string{
				"STREAM-UP 为 Xray 26.3+ 独有能力，Sing-box 暂不支持",
				"XMUX max_concurrency/c_max_reuse_times 必须用范围如 16-32，禁止填固定值",
				"sockopt.tcp_fast_open + tcp_multipath + bbr 拉满直连性能",
				"Referer 伪装 Header 用于流量伪装，可被中间设备识别为正常 HTTPS",
			},
			Warnings: []string{
				"IP完全暴露，仅推荐追求极致速度的高级直连线路",
				"STREAM-UP 为实验性模式，目前仅 v2rayNG 验证通过",
				"Sing-box 不支持 stream-up 模式",
			},
			UpdatedFromUpstream: now,
		},
		// ===== P10: VLESS+HTTPUpgrade+TLS（cf_saas）=====
		{
			ID:                "P10-vless-httpupgrade-tls",
			Name:              "VLESS + HTTPUpgrade + TLS",
			Badge:             PresetBadgeNew,
			Description:       "VLESS+HTTPUpgrade+TLS CDN组合，sing-box原生支持的新传输，比WS更高效(CDN下性能更好)",
			Protocol:          ProtocolVLESS,
			Transport:         TransportHTTPUpgrade,
			Security:          SecurityTLS,
			MinXrayVersion:    "1.8.24",
			MinSingboxVersion: "1.10.0",
			ClientSupport:     []string{"sing-box", "Hiddify", "v2rayNG", "Xray"},
			KernelCompat:      CompatBoth,
			DeploymentProfile: ProfileCFSaaS,
			BaseSpec: NodeSpec{
				Protocol:    ProtocolVLESS,
				Port:        443,
				AllowUDP:    true,
				TrafficRate: 1.0,
				IsVisible:   true,
				Transport: TransportConfig{
					Type: TransportHTTPUpgrade,
					HTTPUpgrade: &HTTPUpgradeConfig{
						Path: "/hu-vless",
					},
				},
				Security: SecurityTLS,
				TLS: &TLSConfig{
					Fingerprint: "chrome",
					CertMode:    TLSCertModeACME,
				},
				Credentials: VLESSCredentials{},
			},
			Recommendations: []string{
				"HTTPUpgrade 比 WS 更高效，CDN 下性能更好",
				"sing-box 原生支持，无需额外插件",
				"CDN域名需开启小黄云(Proxied)",
			},
			Warnings: []string{
				"部分老客户端不支持 HTTPUpgrade",
				"需 sing-box 1.10+ 或 Xray 1.8.24+",
			},
			UpdatedFromUpstream: now,
		},
		// ===== P11: Hysteria2（direct，UDP/QUIC）=====
		{
			ID:                "P11-hysteria2",
			Name:              "Hysteria2",
			Badge:             PresetBadgeRecommended,
			Description:       "基于QUIC的高速协议，丢包/高延迟环境下速度远超TCP类协议，支持端口跳跃",
			Protocol:          ProtocolHysteria2,
			Transport:         TransportQUIC,
			Security:          SecurityTLS,
			MinXrayVersion:    "1.8.6",
			MinSingboxVersion: "1.9.0",
			ClientSupport:     []string{"Clash Meta", "sing-box", "Nekoray", "Hiddify"},
			KernelCompat:      CompatBoth,
			DeploymentProfile: ProfileDirect,
			BaseSpec: NodeSpec{
				Protocol: ProtocolHysteria2,
				Port:     443,
				Transport: TransportConfig{
					Type: TransportQUIC,
					PortHopping: &PortHoppingConfig{
						Enabled:   true,
						PortRange: "40000-41000",
						Interval:  30,
					},
				},
				Security:    SecurityTLS,
				AllowUDP:    true,
				TrafficRate: 1.0,
				IsVisible:   true,
				TLS: &TLSConfig{
					ALPN:        []string{"h3"},
					Fingerprint: "chrome",
					CertMode:    TLSCertModeACME,
				},
				Credentials: Hysteria2Credentials{
					UpMbps:   500,
					DownMbps: 500,
				},
			},
			Recommendations: []string{
				"必须配置TLS证书（ACME自动申请或自签）",
				"端口跳跃配置如40000-41000可抗端口封锁",
				"up_mbps/down_mbps 必须按 VPS 真实带宽的 70-80% 填写",
			},
			Warnings: []string{
				"必须配置 masquerade，否则 GFW 主动探测会看到空响应",
				"bandwidth 必须填真实可用带宽：超填导致丢包重传雪崩，低填浪费带宽",
				"国内运营商对 UDP 限速严重，端口跳跃是必须的，不是可选",
			},
			UpdatedFromUpstream: now,
		},
		// ===== P12: TUIC v5（direct，UDP/QUIC）=====
		{
			ID:                "P12-tuic-v5",
			Name:              "TUIC v5",
			Badge:             PresetBadgeRecommended,
			Description:       "TUIC v5 基于 QUIC 的高速协议，支持 0-RTT，强制 BBR 拥塞控制，生产环境禁用 0-RTT",
			Protocol:          ProtocolTUIC,
			Transport:         TransportQUIC,
			Security:          SecurityTLS,
			MinSingboxVersion: "1.8.0",
			ClientSupport:     []string{"sing-box", "Hiddify", "Nekoray", "Clash Meta"},
			KernelCompat:      CompatBoth,
			DeploymentProfile: ProfileDirect,
			BaseSpec: NodeSpec{
				Protocol:    ProtocolTUIC,
				Port:        443,
				Security:    SecurityTLS,
				AllowUDP:    true,
				TrafficRate: 1.0,
				IsVisible:   true,
				TLS: &TLSConfig{
					ALPN:        []string{"h3"},
					Fingerprint: "chrome",
					CertMode:    TLSCertModeACME,
				},
				Credentials: TUICCredentials{},
			},
			Recommendations: []string{
				"congestion_control 强制 bbr",
				"zero_rtt_handshake 强制 false（生产环境禁用 0-RTT）",
				"需配置真实证书（ACME）",
			},
			Warnings: []string{
				"基于 UDP，部分网络可能限制 UDP 流量",
				"需开放 UDP 443 端口",
				"0-RTT 在生产环境禁用以避免重放攻击",
				"TUIC 不支持端口跳跃，单端口监听",
			},
			UpdatedFromUpstream: now,
		},
		// ===== P13: WARP MASQUE 出口叠加层（overlay，sing-box only）=====
		{
			ID:                "P13-warp-masque-overlay",
			Name:              "任意协议 + WARP MASQUE 出口 (叠加层)",
			Badge:             PresetBadgeBalanced,
			Description:       "WARP MASQUE 出口叠加层，挂在任意协议 outbound 之上，实现出口 IP 隐蔽，解锁 Netflix/ChatGPT 等",
			Protocol:          ProtocolVLESS,
			Transport:         TransportTCP,
			Security:          SecurityNone,
			MinSingboxVersion: "1.8.0",
			ClientSupport:     []string{"sing-box", "Hiddify"},
			KernelCompat:      CompatSingboxOnly,
			DeploymentProfile: ProfileOverlay,
			Enhancement: &EnhancementSpec{
				UTLS:      EnhOptional,
				ECH:       EnhOptional,
				Multiplex: EnhOptional,
			},
			UIWarning: "这是叠加层协议，不是独立协议，需挂在其他协议 outbound 之上",
			BaseSpec: NodeSpec{
				Protocol:    ProtocolVLESS,
				Port:        443,
				AllowUDP:    true,
				TrafficRate: 1.0,
				IsVisible:   true,
				Transport: TransportConfig{
					Type: TransportTCP,
				},
				Security:    SecurityNone,
				Credentials: VLESSCredentials{},
			},
			Recommendations: []string{
				"叠加层不影响 base_protocol 的 CDN 兼容性判断，独立校验",
				"routing_domains 默认 [netflix.com, chatgpt.com, disneyplus.com]",
				"warp_private_key 用 wgcf generate 生成",
			},
			Warnings: []string{
				"这不是独立协议，是叠加在 outbound 之上的出口层",
				"仅 Sing-box 内建 wireguard 支持 WARP MASQUE",
				"需配置 WARP 账户和 private_key",
			},
			UpdatedFromUpstream: now,
		},
		// ===== P14: VLESS+WS+TLS+SS2022（cf_saas，Xray only）=====
		{
			ID:                "P14-vless-ws-tls-ss2022",
			Name:              "VLESS + WS + TLS + SS2022 混合鉴权",
			Badge:             PresetBadgeExperimental,
			Description:       "VLESS+WS+TLS+SS2022 混合鉴权，利用 SS2022 AEAD 加密替代 VLESS none 加密，双重鉴权",
			Protocol:          ProtocolVLESS,
			Transport:         TransportWS,
			Security:          SecurityTLS,
			MinXrayVersion:    "1.8.0",
			ClientSupport:     []string{"v2rayNG", "Xray"},
			KernelCompat:      CompatXrayOnly,
			DeploymentProfile: ProfileCFSaaS,
			BaseSpec: NodeSpec{
				Protocol:   ProtocolVLESS,
				Port:       443,
				ClientPort: 443,
				ServerPort: 8448,
				Transport: TransportConfig{
					Type: TransportWS,
					WS: &WSConfig{
						Path: "/ws-vless-ss2022",
					},
				},
				Security:    SecurityTLS,
				TrafficRate: 1.0,
				IsVisible:   true,
				TLS: &TLSConfig{
					ALPN:        []string{"h2", "http/1.1"},
					Fingerprint: "chrome",
					CertMode:    TLSCertModeACME,
				},
				Credentials: VLESSCredentials{},
			},
			Recommendations: []string{
				"decryption 字段填 SS2022 密钥后，客户端 encryption 必须严格一致",
				"ss2022 key 用 openssl rand -base64 32 生成",
			},
			Warnings: []string{
				"实验性特性，依赖 Xray-core 特定版本",
				"不建议作为唯一鉴权手段大规模上线",
			},
			UpdatedFromUpstream: now,
		},
		// ===== P15: REALITY+Shadowsocks-2022（direct，实验性）=====
		{
			ID:                "P15-reality-ss2022",
			Name:              "REALITY + Shadowsocks-2022",
			Badge:             PresetBadgeExperimental,
			Description:       "Shadowsocks-2022 + REALITY 安全层组合，实验性，需 Xray 最新版本",
			Protocol:          ProtocolShadowsocks,
			Transport:         TransportTCP,
			Security:          SecurityReality,
			MinXrayVersion:    "26.3.0",
			ClientSupport:     []string{"v2rayNG", "Xray"},
			KernelCompat:      CompatExperimental,
			DeploymentProfile: ProfileDirect,
			ForbiddenCombos: map[DeploymentProfile]string{
				ProfileCFSaaS: "REALITY需完整TLS握手，CF会终结TLS破坏协议",
				ProfileCFArgo: "同上",
			},
			Enhancement: &EnhancementSpec{
				UTLS:      EnhMandatory,
				ECH:       EnhNotApplicable,
				Multiplex: EnhOptional,
			},
			BaseSpec: NodeSpec{
				Protocol:    ProtocolShadowsocks,
				Port:        443,
				AllowUDP:    true,
				TrafficRate: 1.0,
				IsVisible:   true,
				Transport: TransportConfig{
					Type: TransportTCP,
				},
				Security: SecurityReality,
				Reality: &RealityConfig{
					SNI:         defaultRealitySNI,
					Fingerprint: "chrome",
				},
				Credentials: ShadowsocksCredentials{
					Method: "2022-blake3-aes-256-gcm",
				},
			},
			Recommendations: []string{
				"SS2022 method: 2022-blake3-aes-256-gcm",
				"REALITY 保护 SS 流量，增强抗审查",
				"ss2022_key 用 openssl rand -base64 32 生成",
			},
			Warnings: []string{
				"实验性组合，Xray-core 需最新版本",
				"Sing-box 暂不支持 SS+REALITY 组合",
				"生产环境谨慎使用",
			},
			UpdatedFromUpstream: now,
		},
		// ===== P16: Trojan+gRPC+TLS（hybrid）=====
		{
			ID:                "P16-trojan-grpc-tls",
			Name:              "Trojan + gRPC + TLS",
			Badge:             PresetBadgeBalanced,
			Description:       "Trojan+gRPC+TLS 混合部署，gRPC 多路复用适合高并发，可走 CDN 也可直连",
			Protocol:          ProtocolTrojan,
			Transport:         TransportGRPC,
			Security:          SecurityTLS,
			MinXrayVersion:    "1.8.0",
			MinSingboxVersion: "1.8.0",
			ClientSupport:     []string{"v2rayNG", "Shadowrocket", "Clash Meta", "sing-box", "Nekoray"},
			KernelCompat:      CompatBoth,
			DeploymentProfile: ProfileHybrid,
			BaseSpec: NodeSpec{
				Protocol:    ProtocolTrojan,
				Port:        443,
				AllowUDP:    false,
				TrafficRate: 1.0,
				IsVisible:   true,
				Transport: TransportConfig{
					Type: TransportGRPC,
					GRPC: &GRPCConfig{
						ServiceName: "grpc-trojan",
					},
				},
				Security: SecurityTLS,
				TLS: &TLSConfig{
					Fingerprint: "chrome",
					CertMode:    TLSCertModeACME,
				},
				Credentials: TrojanCredentials{},
			},
			Recommendations: []string{
				"gRPC 多路复用，适合高并发场景",
				"grpc_service_name 建议用随机字符串",
				"可走 CDN 也可直连，hybrid 模式灵活",
			},
			Warnings: []string{
				"CF 与源站间 HTTP/2 支持不完整，历史上出现过间歇性连接失败",
				"CF 走 gRPC 不推荐生产使用",
			},
			UpdatedFromUpstream: now,
		},
		// ===== P17: XHTTP stream-up+REALITY+XMUX+v4/v6（direct，旗舰）=====
		{
			ID:                "P17-xhttp-stream-up-reality-xmux-v4v6",
			Name:              "XHTTP stream-up + REALITY + XMUX + v4/v6双栈 (终极天花板)",
			Badge:             PresetBadgeRecommended,
			Description:       "XHTTP STREAM-UP+REALITY+v4/v6双栈分离+XMUX多路复用，追求极致速度的终极天花板配置，IPv4/IPv6智能分流",
			Protocol:          ProtocolVLESS,
			Transport:         TransportXHTTP,
			Security:          SecurityReality,
			MinXrayVersion:    "26.3.27",
			ClientSupport:     []string{"v2rayNG", "Xray", "Clash.Meta"},
			KernelCompat:      CompatXrayOnly,
			DeploymentProfile: ProfileDirect,
			ForbiddenCombos: map[DeploymentProfile]string{
				ProfileCFSaaS: "stream-up+REALITY架构下行必须直连，无法整体套CDN",
				ProfileCFArgo: "同上",
			},
			Enhancement: &EnhancementSpec{
				UTLS:      EnhMandatory,
				ECH:       EnhNotApplicable,
				Multiplex: EnhRecommended,
			},
			UIWarning: "IP完全暴露，仅用于VPS来源清洁、追求极致速度的高级直连线路；需服务器同时具备IPv4和IPv6地址",
			BaseSpec: NodeSpec{
				Protocol:    ProtocolVLESS,
				Port:        443,
				AllowUDP:    true,
				TrafficRate: 1.0,
				IsVisible:   true,
				Transport: TransportConfig{
					Type: TransportXHTTP,
					XHTTP: &XHTTPConfig{
						Path:         "/su",
						Mode:         "stream-up",
						NoGRPCHeader: true,
					},
					Mux: &MuxConfig{
						Enabled:         true,
						Protocol:        MuxProtocolXmux,
						MaxConnections:  32,
						MaxConcurrency:  "16-32",
						Padding:         true,
						KeepAlivePeriod: 30,
					},
				},
				Security: SecurityReality,
				Reality: &RealityConfig{
					SNI:         defaultRealitySNI,
					Fingerprint: "chrome",
				},
				Credentials: VLESSCredentials{},
			},
			Recommendations: []string{
				"STREAM-UP 为 Xray 26.3+ 独有能力，Sing-box 暂不支持",
				"XMUX max_concurrency 推荐使用范围如 16-32，获得最佳多路复用效果",
				"address_ipv6 填写 VPS 真实 IPv6 地址，实现 v4/v6 智能分流",
				"dual-stack 环境下客户端自动选择最优协议栈，连接成功率提升",
			},
			Warnings: []string{
				"IP完全暴露，仅推荐追求极致速度的高级直连线路",
				"STREAM-UP 为实验性模式，目前仅 v2rayNG 和最新 Xray 核心验证通过",
				"Sing-box 不支持 stream-up 模式和 xmux 协议",
				"需服务器同时具备 IPv4 和 IPv6 公网地址，否则 v4/v6 分离无意义",
			},
			UpdatedFromUpstream: now,
		},
		// ===== 通用预设（非 P 系列）=====
		{
			ID:                "shadowsocks",
			Name:              "Shadowsocks AEAD",
			Badge:             PresetBadgeBalanced,
			Description:       "经典轻量协议，适合作为中转节点或游戏低延迟节点",
			Protocol:          ProtocolShadowsocks,
			Transport:         TransportTCP,
			Security:          SecurityNone,
			MinXrayVersion:    "1.0.0",
			MinSingboxVersion: "1.0.0",
			ClientSupport:     []string{"所有客户端"},
			KernelCompat:      CompatBoth,
			DeploymentProfile: ProfileDirect,
			BaseSpec: NodeSpec{
				Protocol:    ProtocolShadowsocks,
				Port:        8388,
				Security:    SecurityNone,
				TrafficRate: 1.0,
				IsVisible:   true,
				Credentials: ShadowsocksCredentials{
					Method: "aes-256-gcm",
				},
			},
			Recommendations: []string{
				"建议配合插件(如v2ray-plugin/simple-obfs)或通过TLS隧道使用",
				"2022-blake3加密方法性能更佳",
			},
			UpdatedFromUpstream: now,
		},
		{
			ID:                "mieru",
			Name:              "Mieru (米兔)",
			Badge:             PresetBadgeExperimental,
			Description:       "基于TCP/UDP的零特征代理协议，无TLS特征，抗主动探测能力强",
			Protocol:          ProtocolMieru,
			Transport:         TransportTCP,
			Security:          SecurityNone,
			MinSingboxVersion: "1.10.0",
			ClientSupport:     []string{"sing-box", "Nekoray"},
			KernelCompat:      CompatExperimental,
			DeploymentProfile: ProfileDirect,
			BaseSpec: NodeSpec{
				Protocol:    ProtocolMieru,
				Port:        4000,
				Security:    SecurityNone,
				TrafficRate: 1.0,
				IsVisible:   true,
			},
			Warnings: []string{
				"Mieru为实验性协议，客户端支持有限",
				"不使用TLS，依赖内置混淆保证安全",
			},
			UpdatedFromUpstream: now,
		},
	}
}

func LoadPresetRegistry(presetDirs ...string) (*PresetRegistry, error) {
	reg := NewPresetRegistry()
	defaults := BuildDefaultPresets()
	for _, p := range defaults {
		if err := reg.Register(p); err != nil {
			return nil, fmt.Errorf("register default preset %s: %w", p.ID, err)
		}
	}
	for _, dir := range presetDirs {
		if err := reg.LoadFromDirectory(dir); err != nil {
			return nil, err
		}
	}
	return reg, nil
}
