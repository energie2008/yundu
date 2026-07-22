package renderer

import (
	"regexp"
	"strings"
)

// ClientType 客户端类型（按内核/订阅格式分组）。
//
// 本文件实现独立于 subscription-service/internal/client 的轻量 UA 识别，
// 供 renderer 包在缺少 service 层依赖时直接根据 User-Agent 选择渲染器。
// 正则规则对齐 Xboard Utils\Client.php 的 get-client-info 逻辑。
type ClientType string

const (
	ClientClash        ClientType = "clash"
	ClientClashMeta    ClientType = "clashmeta"
	ClientClashVerge   ClientType = "clash-verge"
	ClientSingBox      ClientType = "sing-box"
	ClientShadowrocket ClientType = "shadowrocket"
	ClientQuantumultX  ClientType = "quantumult-x"
	ClientQuantumult   ClientType = "quantumult"
	ClientSurge        ClientType = "surge"
	ClientSurge2       ClientType = "surge2"
	ClientSurge3       ClientType = "surge3"
	ClientSurge4       ClientType = "surge4"
	ClientStash        ClientType = "stash"
	ClientLoon         ClientType = "loon"
	ClientLoonLite     ClientType = "loon-lite"
	ClientV2rayN       ClientType = "v2rayn"
	ClientV2rayNG      ClientType = "v2rayng"
	ClientNekoBox      ClientType = "nekoray"
	ClientHiddify      ClientType = "hiddify"
	ClientPhobos       ClientType = "phobos"
	ClientKaring       ClientType = "karing"
	ClientSurfboard    ClientType = "surfboard"
	ClientMihomo       ClientType = "mihomo"
	ClientURI          ClientType = "uri"
	ClientUnknown      ClientType = "unknown"
)

// 预编译正则：对齐 Xboard Client.php 的匹配规则。
// 顺序敏感 —— 更具体的规则优先匹配，避免被宽泛规则（如 clash）误吞。
var (
	// Clash 系：先排除 meta/verge/x/stash，再落到基础 clash
	// 注意：分隔符支持 - _ . 三种（clash-meta / clash_meta / clash.meta 均应匹配）
	reClashMeta    = regexp.MustCompile(`(?i)clash[-_.]?(meta|verge)|mihomo|stash`)
	reClash        = regexp.MustCompile(`(?i)clash`)
	// sing-box 检测：匹配 "sing-box" / "singbox" 以及官方客户端短码
	// SFI=iOS  SFA=Android  SFM=macOS  SGM=SingBoxG  SGT=GTK
	// 短码使用 \b 词边界，避免误匹配其他单词中包含 sfi/sfa 的 UA
	reSingBox      = regexp.MustCompile(`(?i)sing-?box`)
	reSingBoxShort = regexp.MustCompile(`(?i)\b(?:sfi|sfa|sfm|sgm|sgt)\b`)
	// sing-box 版本号提取（兼容 "sing-box/1.12.3" 与 "SFI/1.11.0" 与 "sing-box v1.10.0"）
	reSingBoxVer   = regexp.MustCompile(`(?i)(?:sing-?box|\b(?:sfi|sfa|sfm|sgm|sgt)\b)[\s/]+v?(\d+(?:\.\d+){0,2})`)
	reShadowrocket = regexp.MustCompile(`(?i)shadowrocket|shadow\s?rocket`)
	reQuantumultX  = regexp.MustCompile(`(?i)quantumult%20x|quantumult[\s_-]?x|quanx`)
	reQuantumult   = regexp.MustCompile(`(?i)quantumult`)
	reSurge        = regexp.MustCompile(`(?i)surge`)
	reSurgeVer     = regexp.MustCompile(`(?i)surge/(\d)`)
	reLoon         = regexp.MustCompile(`(?i)loon`)
	reV2rayNG      = regexp.MustCompile(`(?i)v2rayng|v2ray\s?ng`)
	reV2rayN       = regexp.MustCompile(`(?i)v2rayn|v2ray\s?n`)
	reNekoBox      = regexp.MustCompile(`(?i)nekoray|nekobox|neko\s?(ray|box)`)
	reHiddify      = regexp.MustCompile(`(?i)hiddify`)
	reKaring       = regexp.MustCompile(`(?i)karing`)
	reSurfboard    = regexp.MustCompile(`(?i)surfboard`)
	rePhobos       = regexp.MustCompile(`(?i)phobos`)
)

// DetectClient 从 User-Agent 识别客户端类型。
// 匹配顺序遵循"具体优先于宽泛"原则，对齐 Xboard Client.php：
//
//   - ClashMeta（含 verge/mihomo/stash）优先于基础 Clash
//   - V2rayNG 优先于 V2rayN（前者是后者的超集）
//   - QuantumultX 优先于 Quantumult
//   - Surge 区分主版本号（2/3/4）
func DetectClient(ua string) ClientType {
	if ua == "" {
		return ClientUnknown
	}
	lower := strings.ToLower(ua)

	// 1. sing-box（含 SFA/SFI/SFM/SGM/SGT 等官方客户端短码）
	if reSingBox.MatchString(lower) || reSingBoxShort.MatchString(lower) {
		return ClientSingBox
	}
	// 2. Shadowrocket
	if reShadowrocket.MatchString(lower) {
		return ClientShadowrocket
	}
	// 3. Quantumult X（优先于 Quantumult）
	if reQuantumultX.MatchString(lower) {
		return ClientQuantumultX
	}
	if reQuantumult.MatchString(lower) {
		return ClientQuantumult
	}
	// 4. Surge（区分版本号）
	if reSurge.MatchString(lower) {
		if m := reSurgeVer.FindStringSubmatch(lower); len(m) >= 2 {
			switch m[1] {
			case "2":
				return ClientSurge2
			case "3":
				return ClientSurge3
			case "4", "5", "6":
				return ClientSurge4
			}
		}
		return ClientSurge
	}
	// 5. Loon
	if reLoon.MatchString(lower) {
		if strings.Contains(lower, "lite") {
			return ClientLoonLite
		}
		return ClientLoon
	}
	// 6. Stash（归入 ClashMeta 类）
	if strings.Contains(lower, "stash") {
		return ClientStash
	}
	// 7. Surfboard
	if reSurfboard.MatchString(lower) {
		return ClientSurfboard
	}
	// 8. V2rayNG（优先于 V2rayN）
	if reV2rayNG.MatchString(lower) {
		return ClientV2rayNG
	}
	if reV2rayN.MatchString(lower) {
		return ClientV2rayN
	}
	// 9. NekoBox / NekoRay
	if reNekoBox.MatchString(lower) {
		return ClientNekoBox
	}
	// 10. Hiddify
	if reHiddify.MatchString(lower) {
		return ClientHiddify
	}
	// 11. Karing
	if reKaring.MatchString(lower) {
		return ClientKaring
	}
	// 12. Phobos
	if rePhobos.MatchString(lower) {
		return ClientPhobos
	}
	// 13. ClashMeta / Mihomo（优先于基础 Clash）
	if reClashMeta.MatchString(lower) {
		return ClientClashMeta
	}
	// 14. 基础 Clash（排除已被上面拦截的 meta/verge/x/stash 后）
	if reClash.MatchString(lower) {
		return ClientClash
	}

	return ClientUnknown
}

// DetectSingBoxVersion 从 UA 提取 sing-box 版本号。
// 返回形如 "1.12.3" 的字符串；无法识别时返回空串。
// 兼容 "sing-box/1.12.3"、"SFI/1.11.0"、"sing-box v1.10.0" 等 UA 格式。
// 对齐 Xboard SingBox::getSingBoxCoreVersion 的 UA 提取逻辑。
func DetectSingBoxVersion(ua string) string {
	if ua == "" {
		return ""
	}
	m := reSingBoxVer.FindStringSubmatch(ua)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

// DetectClientEx 返回客户端类型与识别到的版本号（若有）。
// 版本号目前仅对 sing-box 提取，其他客户端返回空串。
func DetectClientEx(ua string) (ClientType, string) {
	ct := DetectClient(ua)
	if ct == ClientSingBox {
		return ct, DetectSingBoxVersion(ua)
	}
	if ct == ClientSurge2 || ct == ClientSurge3 || ct == ClientSurge4 {
		if m := reSurgeVer.FindStringSubmatch(strings.ToLower(ua)); len(m) >= 2 {
			return ct, m[1]
		}
	}
	return ct, ""
}

// ClientToRenderer 将客户端类型映射到渲染器名称。
// 用于在没有 service 层时直接根据 UA 选择 clash/clashmeta/singbox/surge/uri 渲染器。
func ClientToRenderer(ct ClientType) string {
	switch ct {
	case ClientClash:
		return "clash"
	case ClientClashMeta, ClientClashVerge, ClientStash, ClientMihomo, ClientKaring, ClientHiddify:
		return "clashmeta"
	case ClientSingBox:
		return "singbox"
	case ClientSurge, ClientSurge2, ClientSurge3, ClientSurge4:
		return "surge"
	case ClientShadowrocket, ClientV2rayN, ClientV2rayNG, ClientNekoBox,
		ClientQuantumult, ClientQuantumultX, ClientLoon, ClientLoonLite,
		ClientSurfboard, ClientPhobos:
		return "uri"
	default:
		return "uri"
	}
}
