package validator

import (
	"encoding/base64"
	"fmt"
	"net"
	"strings"

	"github.com/airport-panel/subscription/nodespec"
)

// EnhancementValidatorFunc 统一签名，便于注册表批量执行
type EnhancementValidatorFunc func(spec *nodespec.NodeSpec, tmpl *nodespec.PresetTemplate) []ValidationError

// EnhancementValidators Enhancement 专项校验函数注册表
var EnhancementValidators = []EnhancementValidatorFunc{
	validateECHRequiresTLS13,
	validateECHKernelSupport,
	validateMuxFieldExclusivity,
	validateUTLSFingerprintEnum,
	validateRealityUTLSMandatory,
	validateXHTTPMode,
	validateXHTTPSplitMode,
	validateXHTTPDownloadSettingsRuntime,
	validateIPv4v6Config,
	validateSS2022Key,
}

// RunEnhancementValidators 统一执行入口，供 DualKernelValidator.ValidateBoth 调用
func RunEnhancementValidators(spec *nodespec.NodeSpec, tmpl *nodespec.PresetTemplate) []ValidationError {
	var all []ValidationError
	for _, fn := range EnhancementValidators {
		all = append(all, fn(spec, tmpl)...)
	}
	return all
}

// validUTLSFingerprints Xray/Sing-box 支持的 uTLS 指纹枚举值
var validUTLSFingerprints = map[string]bool{
	"chrome":     true,
	"firefox":    true,
	"safari":     true,
	"ios":        true,
	"android":    true,
	"edge":       true,
	"360":        true,
	"qq":         true,
	"random":     true,
	"randomized": true,
	"utls":       true, // sing-box 别名
}

// ===== 1. validateECHRequiresTLS13 =====
// ECH 要求 TLS 1.3，否则会被静默丢弃，导致功能失效但无错误提示。
// 此校验确保 ECH 启用时，渲染器强制写入 min_version: "1.3"。
// 由于渲染器已强制写入，这里只做声明性校验（记录 info）。
func validateECHRequiresTLS13(spec *nodespec.NodeSpec, tmpl *nodespec.PresetTemplate) []ValidationError {
	if spec == nil || spec.TLS == nil || spec.TLS.ECH == nil || !spec.TLS.ECH.Enabled {
		return nil
	}
	// ECH 启用时检查安全类型必须为 TLS（REALITY 不支持 ECH）
	if spec.Security != nodespec.SecurityTLS {
		return []ValidationError{{
			Level:   LevelError,
			Field:   "tls.ech",
			Message: "ECH只能在security=tls场景下启用，当前security=" + string(spec.Security) + "（REALITY场景不支持ECH）",
		}}
	}
	// 检查预设声明是否标记 ECH 为 not_applicable
	if tmpl != nil && tmpl.Enhancement != nil && tmpl.Enhancement.ECH == nodespec.EnhNotApplicable {
		return []ValidationError{{
			Level:   LevelError,
			Field:   "tls.ech",
			Message: "当前协议预设声明ECH为not_applicable，不应启用ECH（协议机制冲突）",
		}}
	}
	// 记录 info：渲染器已强制写入 min_version: "1.3"
	return []ValidationError{{
		Level:   LevelInfo,
		Field:   "tls.ech.min_version",
		Message: "ECH已启用，渲染器将强制写入tls.min_version=1.3（防止静默失效）",
	}}
}

// ===== 2. validateECHKernelSupport =====
// ECH 仅 Sing-box 内核支持，Xray 不支持。
// 如果预设声明的 KernelCompat 包含 Xray，需要给出 warning。
func validateECHKernelSupport(spec *nodespec.NodeSpec, tmpl *nodespec.PresetTemplate) []ValidationError {
	if spec == nil || spec.TLS == nil || spec.TLS.ECH == nil || !spec.TLS.ECH.Enabled {
		return nil
	}
	var errs []ValidationError
	// 检查预设声明
	if tmpl != nil {
		if tmpl.Enhancement != nil && tmpl.Enhancement.ECH == nodespec.EnhRequiresSingBox {
			// 预设已声明仅 Sing-box 支持，Xray 会忽略
			errs = append(errs, ValidationError{
				Level:   LevelInfo,
				Kernel:  "xray",
				Field:   "tls.ech",
				Message: "ECH仅Sing-box内核支持，Xray渲染器将忽略此字段（预期行为）",
			})
		}
		// 如果 KernelCompat 是 both 或 xray_only，ECH 在 Xray 端会失效
		if tmpl.KernelCompat == nodespec.CompatBoth || tmpl.KernelCompat == nodespec.CompatXrayOnly {
			errs = append(errs, ValidationError{
				Level:   LevelWarning,
				Kernel:  "xray",
				Field:   "tls.ech",
				Message: "ECH在Xray内核不支持，Xray客户端将无法使用ECH特性（Sing-box客户端不受影响）",
			})
		}
	} else {
		// 无预设时，默认给 warning
		errs = append(errs, ValidationError{
			Level:   LevelWarning,
			Kernel:  "xray",
			Field:   "tls.ech",
			Message: "ECH在Xray内核不支持，Xray客户端将无法使用ECH特性",
		})
	}
	return errs
}

// ===== 3. validateMuxFieldExclusivity =====
// Mux 字段互斥性：max_connections 与 min/max_streams 二选一，不能同时设置。
// 同时设置会导致内核行为不确定。
func validateMuxFieldExclusivity(spec *nodespec.NodeSpec, tmpl *nodespec.PresetTemplate) []ValidationError {
	if spec == nil || spec.Transport.Mux == nil || !spec.Transport.Mux.Enabled {
		return nil
	}
	m := spec.Transport.Mux
	var errs []ValidationError

	// 互斥检查：max_connections 和 max_streams 不能同时设置
	// sing-box 的 multiplex 配置中，max_connections 用连接池模式，max_streams 用流模式，二选一
	if m.MaxConnections > 0 && m.MaxStreams > 0 {
		errs = append(errs, ValidationError{
			Level:   LevelError,
			Field:   "transport.mux",
			Message: "Mux字段互斥冲突：max_connections与max_streams不能同时设置，请二选一",
		})
	}

	// 检查预设声明是否标记 Mux 为 not_applicable
	if tmpl != nil && tmpl.Enhancement != nil && tmpl.Enhancement.Multiplex == nodespec.EnhNotApplicable {
		// QUIC 协议（Hysteria2/TUIC）自带多路复用，不应启用 Mux
		if spec.Protocol == nodespec.ProtocolHysteria2 || spec.Protocol == nodespec.ProtocolTUIC {
			errs = append(errs, ValidationError{
				Level:   LevelError,
				Field:   "transport.mux",
				Message: "QUIC协议（Hysteria2/TUIC）自带多路复用，启用Mux会导致冲突",
			})
		} else {
			errs = append(errs, ValidationError{
				Level:   LevelWarning,
				Field:   "transport.mux",
				Message: "当前协议预设声明Mux为not_applicable，启用Mux可能导致异常",
			})
		}
	}

	// XMUX 冲突检查：XMUX 协议是 XHTTP 传输特有的多路复用协议
	// 其他 mux 协议（h2mux/yamux/smux）可用于 TCP/WS/gRPC 等传输
	if m.Protocol == nodespec.MuxProtocolXmux && spec.Transport.Type != nodespec.TransportXHTTP {
		errs = append(errs, ValidationError{
			Level:   LevelError,
			Field:   "transport.mux.protocol",
			Message: "XMUX协议仅适用于XHTTP传输，当前传输类型=" + string(spec.Transport.Type) + "，请改用h2mux/yamux",
		})
	}

	return errs
}

// ===== 4. validateUTLSFingerprintEnum =====
// uTLS 指纹必须是内核支持的枚举值，否则会导致 TLS 握手失败。
func validateUTLSFingerprintEnum(spec *nodespec.NodeSpec, tmpl *nodespec.PresetTemplate) []ValidationError {
	if spec == nil {
		return nil
	}
	var errs []ValidationError

	// 检查 TLS 场景的指纹
	if spec.TLS != nil && spec.TLS.Fingerprint != "" {
		fp := strings.ToLower(spec.TLS.Fingerprint)
		if !validUTLSFingerprints[fp] {
			errs = append(errs, ValidationError{
				Level:   LevelError,
				Field:   "tls.fingerprint",
				Message: fmt.Sprintf("无效的uTLS指纹: %s（支持: chrome/firefox/safari/ios/android/edge/360/qq/random/randomized）", spec.TLS.Fingerprint),
			})
		}
	}

	// 检查 REALITY 场景的指纹
	if spec.Reality != nil && spec.Reality.Fingerprint != "" {
		fp := strings.ToLower(spec.Reality.Fingerprint)
		if !validUTLSFingerprints[fp] {
			errs = append(errs, ValidationError{
				Level:   LevelError,
				Field:   "reality.fingerprint",
				Message: fmt.Sprintf("无效的uTLS指纹: %s（支持: chrome/firefox/safari/ios/android/edge/360/qq/random/randomized）", spec.Reality.Fingerprint),
			})
		}
	}

	// 检查预设声明是否标记 uTLS 为 mandatory 但未设置指纹
	if tmpl != nil && tmpl.Enhancement != nil && tmpl.Enhancement.UTLS == nodespec.EnhMandatory {
		hasFingerprint := (spec.TLS != nil && spec.TLS.Fingerprint != "") ||
			(spec.Reality != nil && spec.Reality.Fingerprint != "")
		if !hasFingerprint {
			errs = append(errs, ValidationError{
				Level:   LevelError,
				Field:   "enhancement.utls",
				Message: "当前协议预设声明uTLS为mandatory（强制必须启用），但未配置fingerprint字段",
			})
		}
	}

	return errs
}

// ===== 5. validateRealityUTLSMandatory =====
// REALITY 机制依赖 uTLS 指纹伪装，必须启用 uTLS。
// 如果 REALITY 场景未设置指纹，渲染器会默认 chrome，但校验层应给出 warning。
func validateRealityUTLSMandatory(spec *nodespec.NodeSpec, tmpl *nodespec.PresetTemplate) []ValidationError {
	if spec == nil || spec.Security != nodespec.SecurityReality {
		return nil
	}
	var errs []ValidationError

	// REALITY 必须有 reality 配置
	if spec.Reality == nil {
		errs = append(errs, ValidationError{
			Level:   LevelError,
			Field:   "reality",
			Message: "REALITY场景缺少reality配置",
		})
		return errs
	}

	// REALITY 必须有 PrivateKey（服务端配置）
	if spec.Reality.PrivateKey == "" {
		errs = append(errs, ValidationError{
			Level:   LevelError,
			Field:   "reality.private_key",
			Message: "REALITY服务端缺少private_key，请用 `xray x25519` 生成密钥对",
		})
	}

	// REALITY 必须有 SNI（dest 域名）
	if spec.Reality.SNI == "" {
		errs = append(errs, ValidationError{
			Level:   LevelError,
			Field:   "reality.sni",
			Message: "REALITY缺少SNI（dest域名），建议使用 mesu.apple.com:443（用户规范统一伪装站）",
		})
	}

	// REALITY 必须有 short_id
	hasShortID := spec.Reality.ShortID != "" || len(spec.Reality.ShortIDs) > 0
	if !hasShortID {
		errs = append(errs, ValidationError{
			Level:   LevelWarning,
			Field:   "reality.short_id",
			Message: "REALITY未配置short_id，建议用 `openssl rand -hex 4` 生成",
		})
	}

	// uTLS 指纹检查：REALITY 依赖 uTLS，如果未设置则给 warning（渲染器会默认 chrome）
	if spec.Reality.Fingerprint == "" {
		errs = append(errs, ValidationError{
			Level:   LevelWarning,
			Field:   "reality.fingerprint",
			Message: "REALITY依赖uTLS指纹伪装但未配置，渲染器将默认使用chrome指纹",
		})
	}

	// 检查 SNI 是否为已知的不推荐值
	// 用户规范：统一使用 mesu.apple.com，不再检测 apple.com
	lowerSNI := strings.ToLower(spec.Reality.SNI)
	if strings.Contains(lowerSNI, "bing.com") || strings.Contains(lowerSNI, "microsoft.com") {
		errs = append(errs, ValidationError{
			Level:   LevelWarning,
			Field:   "reality.sni",
			Message: fmt.Sprintf("REALITY dest域名 %s 被 GFW 主动探测频繁，建议改为 mesu.apple.com", spec.Reality.SNI),
		})
	}

	// 检查预设声明的 deployment_profile 是否为禁止组合
	if tmpl != nil {
		if reason, forbidden := tmpl.ForbiddenCombos[nodespec.ProfileCFSaaS]; forbidden {
			errs = append(errs, ValidationError{
				Level:   LevelError,
				Field:   "deployment_profile",
				Message: "REALITY协议禁止使用cf_saas部署画像：" + reason,
			})
		}
		if reason, forbidden := tmpl.ForbiddenCombos[nodespec.ProfileCFArgo]; forbidden {
			errs = append(errs, ValidationError{
				Level:   LevelError,
				Field:   "deployment_profile",
				Message: "REALITY协议禁止使用cf_argo部署画像：" + reason,
			})
		}
	}

	return errs
}

// ===== 6. validateXHTTPMode =====
// XHTTP mode 字段禁止为空；mode="auto"（预设使用）会按场景归一化为
// packet-up（CDN 场景）或 stream-up（直连/REALITY 场景）并给出 warning，
// 不再直接返回 error，避免与预设 auto 冲突。
// mode="auto" 在不同版本 Xray 行为不一致，归一化后提示显式指定更稳妥。
func validateXHTTPMode(spec *nodespec.NodeSpec, tmpl *nodespec.PresetTemplate) []ValidationError {
	if spec == nil || spec.Transport.Type != nodespec.TransportXHTTP {
		return nil
	}
	var errs []ValidationError

	if spec.Transport.XHTTP == nil {
		errs = append(errs, ValidationError{
			Level:   LevelError,
			Field:   "transport.xhttp",
			Message: "XHTTP传输类型需要配置xhttp参数",
		})
		return errs
	}

	mode := spec.Transport.XHTTP.Mode
	// B25: mode="auto" 由预设使用（见 nodespec.BuildDefaultPresets 的 vless-reality-xhttp），
	// 不再直接拒绝。改为按场景归一化为合理默认值：CDN 场景→packet-up，直连/REALITY→stream-up，
	// 并以 warning 形式提示（而非 error），避免预设校验失败。
	if mode == "auto" {
		normalized := "packet-up" // CDN 场景默认
		// REALITY 必须直连（不能走 CDN），直连场景使用 stream-up
		if spec.Security == nodespec.SecurityReality {
			normalized = "stream-up"
		}
		spec.Transport.XHTTP.Mode = normalized
		errs = append(errs, ValidationError{
			Level:   LevelWarning,
			Field:   "transport.xhttp.mode",
			Message: fmt.Sprintf("xhttp_mode=auto 已自动归一化为 %s（CDN→packet-up，直连/REALITY→stream-up），建议显式指定以避免不同版本 Xray 行为差异", normalized),
		})
		mode = normalized
	}

	if mode == "" {
		errs = append(errs, ValidationError{
			Level:   LevelError,
			Field:   "transport.xhttp.mode",
			Message: "xhttp_mode禁止为空，必须显式指定packet-up/stream-up/stream-down（或使用auto自动选择）",
		})
	}

	validModes := map[string]bool{
		"packet-up":   true,
		"stream-up":   true,
		"stream-down": true,
	}
	if mode != "" && !validModes[mode] {
		errs = append(errs, ValidationError{
			Level:   LevelWarning,
			Field:   "transport.xhttp.mode",
			Message: fmt.Sprintf("xhttp_mode=%s 为非标准值，推荐使用 packet-up/stream-up/stream-down", mode),
		})
	}

	// XHTTP 仅 Xray 内核支持（kernelrender 会拦截 Sing-box）
	if tmpl != nil {
		if tmpl.KernelCompat == nodespec.CompatSingboxOnly {
			errs = append(errs, ValidationError{
				Level:   LevelError,
				Kernel:  "singbox",
				Field:   "transport.xhttp",
				Message: "XHTTP传输仅Xray内核支持，Sing-box不支持此传输类型",
			})
		}
	}

	return errs
}

// ===== 7. validateXHTTPSplitMode =====
// XHTTP split mode（上下行分离）校验：download_settings 配置完整性检查。
func validateXHTTPSplitMode(spec *nodespec.NodeSpec, tmpl *nodespec.PresetTemplate) []ValidationError {
	if spec == nil || spec.Transport.Type != nodespec.TransportXHTTP || spec.Transport.XHTTP == nil {
		return nil
	}
	var errs []ValidationError

	ds := spec.Transport.XHTTP.DownloadSettings
	if ds == nil || ds.Address == "" {
		return nil
	}

	// 下行配置必须有 port
	if ds.Port <= 0 {
		errs = append(errs, ValidationError{
			Level:   LevelWarning,
			Field:   "transport.xhttp.download_settings.port",
			Message: "download_settings.port未设置，将默认使用443",
		})
	}

	// 下行安全类型检查
	switch ds.Security {
	case nodespec.SecurityReality:
		if ds.Reality == nil {
			errs = append(errs, ValidationError{
				Level:   LevelError,
				Field:   "transport.xhttp.download_settings.reality",
				Message: "download_settings.security=reality 但未配置reality参数",
			})
		} else {
			if ds.Reality.PublicKey == "" {
				errs = append(errs, ValidationError{
					Level:   LevelError,
					Field:   "transport.xhttp.download_settings.reality.public_key",
					Message: "download_settings REALITY场景需要public_key",
				})
			}
			if ds.Reality.SNI == "" {
				errs = append(errs, ValidationError{
					Level:   LevelWarning,
					Field:   "transport.xhttp.download_settings.reality.sni",
					Message: "download_settings REALITY场景建议配置sni（dest域名）",
				})
			}
		}
	case nodespec.SecurityTLS:
		if ds.TLS == nil {
			errs = append(errs, ValidationError{
				Level:   LevelWarning,
				Field:   "transport.xhttp.download_settings.tls",
				Message: "download_settings.security=tls 建议配置tls参数（sni/alpn等）",
			})
		}
	}

	// 下行 mode 默认 stream-down
	if ds.Mode == "" {
		errs = append(errs, ValidationError{
			Level:   LevelInfo,
			Field:   "transport.xhttp.download_settings.mode",
			Message: "download_settings.mode未设置，建议显式设置为stream-down",
		})
	}

	// downloadSettings.network 必须为 xhttp（downloadSettings 本身是一条 XHTTP 连接，
	// network 字段描述的是这条连接的传输类型，不是"目标协议"）
	if ds.Network != "" && ds.Network != nodespec.TransportXHTTP {
		errs = append(errs, ValidationError{
			Level:   LevelError,
			Field:   "transport.xhttp.download_settings.network",
			Message: fmt.Sprintf("xhttp downloadSettings.network必须为\"xhttp\"，got=%q", string(ds.Network)),
		})
	}

	return errs
}

// ===== 7b. validateXHTTPDownloadSettingsRuntime =====
// downloadSettings+stream-up 组合在部分 xray 版本存在静默失败风险
// （TCP/TLS 握手正常但无数据传输）。此处不硬编码版本号黑名单，
// 改为发出 Warning，要求部署时必须跑通 check_xhttp_download.sh 实测脚本。
// 详见技术方案文档 Batch 1 §3.3。
func validateXHTTPDownloadSettingsRuntime(spec *nodespec.NodeSpec, tmpl *nodespec.PresetTemplate) []ValidationError {
	if spec == nil || spec.Transport.Type != nodespec.TransportXHTTP || spec.Transport.XHTTP == nil {
		return nil
	}
	ds := spec.Transport.XHTTP.DownloadSettings
	if ds == nil || ds.Address == "" {
		return nil
	}
	if spec.Transport.XHTTP.Mode != "stream-up" {
		return nil
	}
	// 不判断版本号，要求部署时必须跑通自检脚本
	return []ValidationError{{
		Level:   LevelWarning,
		Field:   "transport.xhttp.download_settings",
		Message: "downloadSettings+stream-up组合在部分xray版本存在静默失败风险（TCP/TLS握手正常但无数据），部署前必须执行 scripts/check_xhttp_download.sh 验证实际数据传输，仅凭连通性测试不足以确认可用",
	}}
}

// ===== 8. validateIPv4v6Config =====
// IPv4/IPv6 双栈分离配置校验。
func validateIPv4v6Config(spec *nodespec.NodeSpec, tmpl *nodespec.PresetTemplate) []ValidationError {
	if spec == nil {
		return nil
	}
	var errs []ValidationError

	// 检查主地址和IPv6地址格式
	if spec.AddressIPv6 != "" {
		if spec.Address == "" {
			errs = append(errs, ValidationError{
				Level:   LevelWarning,
				Field:   "address_ipv6",
				Message: "配置了address_ipv6但address(IPv4)为空，双栈分离需要同时配置IPv4和IPv6",
			})
		}
		// 严格 IP 格式校验（net.ParseIP 同时校验 IPv4/IPv6）
		// 注意：net.ParseIP 不接受方括号和端口，需要纯净 IP 字符串
		if net.ParseIP(spec.AddressIPv6) == nil {
			errs = append(errs, ValidationError{
				Level:   LevelError,
				Field:   "address_ipv6",
				Message: fmt.Sprintf("address_ipv6=%s 不是有效的IP地址格式（net.ParseIP校验失败）", spec.AddressIPv6),
			})
		}
	}

	// XHTTP download_settings 中的 IPv6 地址检查
	if spec.Transport.XHTTP != nil && spec.Transport.XHTTP.DownloadSettings != nil {
		ds := spec.Transport.XHTTP.DownloadSettings
		if ds.AddressIPv6 != "" {
			if ds.Address == "" {
				errs = append(errs, ValidationError{
					Level:   LevelWarning,
					Field:   "transport.xhttp.download_settings.address_ipv6",
					Message: "download_settings配置了address_ipv6但address(IPv4)为空",
				})
			}
			// 严格校验 download_settings.address_ipv6 格式
			if net.ParseIP(ds.AddressIPv6) == nil {
				errs = append(errs, ValidationError{
					Level:   LevelError,
					Field:   "transport.xhttp.download_settings.address_ipv6",
					Message: fmt.Sprintf("download_settings.address_ipv6=%s 不是有效的IP地址（net.ParseIP校验失败）", ds.AddressIPv6),
				})
			}
		}
	}

	return errs
}

// ===== 10. validateSS2022Key =====
// SS2022 的 key 是密码学 PSK（Pre-Shared Key），必须是指定字节长度的随机数据再做 base64 编码。
// 不能是普通密码字符串。key 长度与 cipher 不匹配会导致 xray "bad key" 报错。
//
// Cipher 要求原始字节长度 | base64 编码后长度（含 padding）
// 2022-blake3-aes-128-gcm       | 16 bytes | 24 字符（含 ==）
// 2022-blake3-aes-256-gcm       | 32 bytes | 44 字符（含 =）
// 2022-blake3-chacha20-poly1305 | 32 bytes | 44 字符（含 =）
var ss2022KeyLengths = map[string]int{
	"2022-blake3-aes-128-gcm":       16,
	"2022-blake3-aes-256-gcm":       32,
	"2022-blake3-chacha20-poly1305": 32,
}

func validateSS2022Key(spec *nodespec.NodeSpec, tmpl *nodespec.PresetTemplate) []ValidationError {
	if spec == nil || spec.Protocol != nodespec.ProtocolShadowsocks {
		return nil
	}

	// 从 credentials 提取 method 和 password
	var method, password string
	switch c := spec.Credentials.(type) {
	case nodespec.ShadowsocksCredentials:
		method, password = c.Method, c.Password
	case *nodespec.ShadowsocksCredentials:
		if c == nil {
			return nil
		}
		method, password = c.Method, c.Password
	case map[string]interface{}:
		if m, ok := c["method"].(string); ok {
			method = m
		}
		if p, ok := c["password"].(string); ok {
			password = p
		}
	default:
		return nil
	}

	// 只校验 SS2022（2022- 开头的 method）
	if !strings.HasPrefix(method, "2022-") {
		return nil
	}

	expectedLen, ok := ss2022KeyLengths[method]
	if !ok {
		return []ValidationError{{
			Level:   LevelError,
			Field:   "credentials.method",
			Message: fmt.Sprintf("未知 SS2022 cipher: %s", method),
		}}
	}

	// key 必须是有效 base64 编码
	raw, err := base64.StdEncoding.DecodeString(password)
	if err != nil {
		return []ValidationError{{
			Level:   LevelError,
			Field:   "credentials.password",
			Message: fmt.Sprintf("SS2022 key 必须是有效 base64 编码（cipher=%s），got err: %v；用 `openssl rand -base64 %d` 重新生成", method, err, expectedLen),
		}}
	}

	// 解码后字节长度必须精确匹配
	if len(raw) != expectedLen {
		return []ValidationError{{
			Level:   LevelError,
			Field:   "credentials.password",
			Message: fmt.Sprintf("SS2022 cipher=%s 要求 key 解码后为 %d 字节，实际为 %d 字节；用 `openssl rand -base64 %d` 重新生成", method, expectedLen, len(raw), expectedLen),
		}}
	}

	return nil
}
