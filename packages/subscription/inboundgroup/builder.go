package inboundgroup

import (
	"fmt"
	"strings"

	"github.com/airport-panel/subscription/nodespec"
)

// GroupBuilder 将多个 NodeSpec 编译为 InboundGroup。
//
// 分组规则：
//   - TCP+REALITY+Vision → Primary inbound（0.0.0.0:443，带 fallbacks）
//   - XHTTP+REALITY+无downloadSettings → @xhttp-internal unix socket internal inbound
//   - XHTTP+TLS（CDN模式）→ fallback path 指向 127.0.0.1:8080 + internal inbound
//   - 独立端口节点（非443）→ 独立 Group，不合并，行为与现状一致
type GroupBuilder struct {
	// PrimaryPort 需要合并的主端口，默认 443
	PrimaryPort int
}

// NewGroupBuilder 创建一个 GroupBuilder。
func NewGroupBuilder() *GroupBuilder {
	return &GroupBuilder{PrimaryPort: 443}
}

// BuildPrimaryClients 合并所有 TCP+Vision 节点的用户到 primary clients。
//
// 三层防护的第一层：Builder 层去重 + 硬断言。
//   - UUID 重复 → 直接返回 error，拒绝构建
//   - 用户数不匹配 → 硬断言失败
//
// 返回的 clients 列表可直接用于 xray inbound settings.clients。
func (gb *GroupBuilder) BuildPrimaryClients(specs []*nodespec.NodeSpec) ([]map[string]interface{}, error) {
	seen := make(map[string]bool)
	var clients []map[string]interface{}
	expectedCount := 0

	for _, spec := range specs {
		if spec == nil || spec.Transport.Type != nodespec.TransportTCP {
			continue
		}
		users := ExtractUsersFromSpec(spec)
		expectedCount += len(users)
		for _, u := range users {
			if seen[u.UUID] {
				return nil, fmt.Errorf("duplicate UUID %s across specs, refusing to build primary clients", u.UUID)
			}
			seen[u.UUID] = true
			// flow 字段通过 RenderClientFlow 统一获取（B3-2 类型约束）
			flow := nodespec.RenderClientFlow(spec.Transport.Type)
			clients = append(clients, map[string]interface{}{
				"id":         u.UUID,
				"flow":       string(flow),
				"email":      u.Email,
				"encryption": "none",
			})
		}
	}

	// 硬断言：渲染出的 clients 数必须等于输入用户总数
	if len(clients) != expectedCount {
		return nil, fmt.Errorf("primary client count mismatch: rendered=%d, expected=%d, refusing to deploy",
			len(clients), expectedCount)
	}

	return clients, nil
}

// DetectPathConflicts 检测 fallback path 冲突。
// Xray fallback-by-path 是精确字符串匹配，path 冲突会导致流量路由错误。
//
// 检测范围：XHTTP 和 WS（两种走 fallback 的传输类型），不只在同协议内查重。
// 边界情况：
//   - 末尾斜杠差异（/path vs /path/）→ normalizePath 后视为冲突
//   - query string 或 fragment → 直接拒绝
//   - WS 与 XHTTP 使用同一 path → 视为冲突
func (gb *GroupBuilder) DetectPathConflicts(specs []*nodespec.NodeSpec) error {
	pathOwner := make(map[string]string)
	for _, spec := range specs {
		if spec == nil {
			continue
		}
		path := extractFallbackPath(spec)
		if path == "" {
			continue
		}

		// 边界检查：path 不应包含 ? 或 #
		if err := validatePathEdgeCases(path); err != nil {
			return fmt.Errorf("spec %s: %w", spec.Code, err)
		}

		normalized := NormalizePath(path)
		if owner, exists := pathOwner[normalized]; exists {
			return fmt.Errorf("path冲突: %q (normalized=%q) 同时被 %s 和 %s 使用",
				path, normalized, owner, spec.Code)
		}
		pathOwner[normalized] = spec.Code
	}
	return nil
}

// BuildGroupsFromNodeSpecs 主入口：将多个 NodeSpec 编译为 InboundGroup 列表。
//
// 步骤：
//  1. DetectPathConflicts 预检（双重校验第一层）
//  2. 分离 TCP+443 节点（primary candidates）和 fallback 节点（internal candidates）
//  3. BuildPrimaryClients 合并 primary 用户（三层防护第一层）
//  4. 为 fallback 节点构建 internal inbound + fallback rules
//  5. 独立端口节点单独成 Group
func (gb *GroupBuilder) BuildGroupsFromNodeSpecs(specs []*nodespec.NodeSpec) ([]*InboundGroup, error) {
	if gb.PrimaryPort == 0 {
		gb.PrimaryPort = 443
	}

	// 第一层：path 冲突预检
	if err := gb.DetectPathConflicts(specs); err != nil {
		return nil, fmt.Errorf("path conflict check failed: %w", err)
	}

	// 分离 primary candidates（TCP+443）和 fallback candidates（XHTTP/WS+443）
	// 和独立端口节点（非 443）
	var primarySpecs []*nodespec.NodeSpec
	var fallbackSpecs []*nodespec.NodeSpec
	var standaloneSpecs []*nodespec.NodeSpec

	for _, spec := range specs {
		if spec == nil {
			continue
		}
		if spec.Port != gb.PrimaryPort {
			standaloneSpecs = append(standaloneSpecs, spec)
			continue
		}
		if spec.Transport.Type == nodespec.TransportTCP {
			primarySpecs = append(primarySpecs, spec)
		} else {
			fallbackSpecs = append(fallbackSpecs, spec)
		}
	}

	var groups []*InboundGroup

	// 构建 primary group（如果有 primary 或 fallback 节点共享 443）
	if len(primarySpecs) > 0 || len(fallbackSpecs) > 0 {
		group, err := gb.buildPrimaryGroup(primarySpecs, fallbackSpecs)
		if err != nil {
			return nil, fmt.Errorf("build primary group: %w", err)
		}
		groups = append(groups, group)
	}

	// 独立端口节点单独成 Group（不合并，行为与现状一致）
	for _, spec := range standaloneSpecs {
		group := gb.buildStandaloneGroup(spec)
		groups = append(groups, group)
	}

	return groups, nil
}

// buildPrimaryGroup 构建共享 443 端口的 primary + internal 架构。
func (gb *GroupBuilder) buildPrimaryGroup(primarySpecs, fallbackSpecs []*nodespec.NodeSpec) (*InboundGroup, error) {
	// 合并 primary clients（三层防护第一层）
	clients, err := gb.BuildPrimaryClients(primarySpecs)
	if err != nil {
		return nil, err
	}

	// Primary inbound：TCP+REALITY+Vision
	primary := &InboundUnit{
		Port:     gb.PrimaryPort,
		Listen:   "0.0.0.0",
		Protocol: nodespec.ProtocolVLESS,
		Tag:      "primary-inbound",
		Sniffing: true,
		Settings: map[string]interface{}{
			"clients":    clients,
			"decryption": "none",
		},
	}

	// 从第一个 primary spec 继承 REALITY stream 配置
	if len(primarySpecs) > 0 {
		primary.Stream = gb.buildRealityStream(primarySpecs[0])
	}

	// 构建 internal inbounds + fallback rules
	var internal []*InboundUnit
	var fallbacks []FallbackRule

	for _, spec := range fallbackSpecs {
		internalTag := fmt.Sprintf("%s-internal", spec.Code)
		internalListen := fmt.Sprintf("@%s", internalTag)

		// Internal inbound
		internalUnit := &InboundUnit{
			Listen:   internalListen,
			Protocol: spec.Protocol,
			Tag:      internalTag,
			Settings: gb.buildInternalSettings(spec),
		}
		internal = append(internal, internalUnit)

		// Fallback rule（dest 统一显式写 @internalTag，不依赖纯端口号隐式补全）
		fb := FallbackRule{
			Dest: internalListen,
			Xver: 1,
		}
		if path := extractFallbackPath(spec); path != "" {
			fb.Path = path
		}
		fallbacks = append(fallbacks, fb)
	}

	primary.Fallbacks = fallbacks

	return &InboundGroup{
		ID:       fmt.Sprintf("primary-%d", gb.PrimaryPort),
		Port:     gb.PrimaryPort,
		Listen:   "0.0.0.0",
		Primary:  primary,
		Internal: internal,
	}, nil
}

// buildStandaloneGroup 构建独立端口节点的 Group（不合并）。
func (gb *GroupBuilder) buildStandaloneGroup(spec *nodespec.NodeSpec) *InboundGroup {
	tag := fmt.Sprintf("%s-inbound", spec.Code)
	unit := &InboundUnit{
		Port:     spec.Port,
		Listen:   "0.0.0.0",
		Protocol: spec.Protocol,
		Tag:      tag,
		Sniffing: true,
		Settings: gb.buildInternalSettings(spec),
	}
	// 独立 TCP 节点需要 clients
	if spec.Transport.Type == nodespec.TransportTCP {
		clients, _ := gb.BuildPrimaryClients([]*nodespec.NodeSpec{spec})
		if unit.Settings == nil {
			unit.Settings = map[string]interface{}{}
		}
		unit.Settings["clients"] = clients
		unit.Settings["decryption"] = "none"
		unit.Stream = gb.buildRealityStream(spec)
	}
	return &InboundGroup{
		ID:      fmt.Sprintf("%s-%d", spec.Code, spec.Port),
		Port:    spec.Port,
		Listen:  "0.0.0.0",
		Primary: unit,
	}
}

// buildRealityStream 构建 REALITY stream 配置（简化版，实际生产应由 kernelrender 处理）。
func (gb *GroupBuilder) buildRealityStream(spec *nodespec.NodeSpec) map[string]interface{} {
	if spec.Reality == nil {
		return nil
	}
	stream := map[string]interface{}{
		"network":  "tcp",
		"security": "reality",
	}
	reality := map[string]interface{}{
		"show": false,
	}
	if spec.Reality.SNI != "" {
		reality["serverNames"] = []string{spec.Reality.SNI}
	}
	if spec.Reality.PublicKey != "" {
		reality["publicKey"] = spec.Reality.PublicKey
	}
	if spec.Reality.ShortID != "" {
		reality["shortIds"] = []string{spec.Reality.ShortID}
	}
	if spec.Reality.Fingerprint != "" {
		reality["fingerprint"] = spec.Reality.Fingerprint
	}
	stream["realitySettings"] = reality
	return stream
}

// buildInternalSettings 构建内部 inbound 的 settings。
func (gb *GroupBuilder) buildInternalSettings(spec *nodespec.NodeSpec) map[string]interface{} {
	settings := map[string]interface{}{
		"decryption": "none",
	}
	// 从 spec 提取 clients（单节点 internal，不需要合并）
	users := ExtractUsersFromSpec(spec)
	if len(users) > 0 {
		var clients []map[string]interface{}
		for _, u := range users {
			clients = append(clients, map[string]interface{}{
				"id":         u.UUID,
				"email":      u.Email,
				"encryption": "none",
			})
		}
		settings["clients"] = clients
	}
	// Trojan 协议需要 password
	if spec.Protocol == nodespec.ProtocolTrojan {
		if tc, ok := spec.Credentials.(nodespec.TrojanCredentials); ok {
			settings["passwords"] = []string{tc.Password}
		} else if tc, ok := spec.Credentials.(*nodespec.TrojanCredentials); ok && tc != nil {
			settings["passwords"] = []string{tc.Password}
		}
	}
	return settings
}

// PreflightUUIDCheck 三层防护的第二层：部署前自检，对比 DB 总数与渲染结果。
//
// 调用点：DeploymentService 在真正下发配置前调用此函数，
// 如果 DB 中 TCP 节点的用户总数 ≠ 渲染出的 primary clients 数，拒绝部署。
func PreflightUUIDCheck(specs []*nodespec.NodeSpec, renderedInbounds []interface{}) error {
	gb := NewGroupBuilder()
	expectedClients, err := gb.BuildPrimaryClients(specs)
	if err != nil {
		return fmt.Errorf("preflight build primary clients: %w", err)
	}
	expectedCount := len(expectedClients)

	// 从渲染结果中提取 primary inbound 的 clients 数
	renderedCount := extractPrimaryClientCount(renderedInbounds)

	if expectedCount != renderedCount {
		return fmt.Errorf("UUID count mismatch: DB=%d, rendered=%d, refusing to deploy",
			expectedCount, renderedCount)
	}
	return nil
}

// extractPrimaryClientCount 从渲染出的 inbounds 中提取 primary inbound 的 client 数。
// primary inbound 是监听 0.0.0.0:443 的那个。
func extractPrimaryClientCount(inbounds []interface{}) int {
	for _, ib := range inbounds {
		m, ok := ib.(map[string]interface{})
		if !ok {
			continue
		}
		listen, _ := m["listen"].(string)
		port, _ := m["port"].(float64)
		if listen == "0.0.0.0" && int(port) == 443 {
			settings, _ := m["settings"].(map[string]interface{})
			if settings == nil {
				continue
			}
			clients, _ := settings["clients"].([]interface{})
			return len(clients)
		}
	}
	return 0
}

// IsGroupedConfig 通过结构特征识别是否为 grouped 配置（不依赖版本号）。
// 用于 node-agent 双格式兼容：解析 xray 配置时自动识别 legacy 或 grouped 格式。
//
// 识别规则：
//   - 任一 inbound 的 listen 以 "@" 开头（abstract unix socket）→ grouped
//   - 任一 inbound 有 fallbacks 数组且非空 → grouped
func IsGroupedConfig(inbounds []interface{}) bool {
	for _, ib := range inbounds {
		m, ok := ib.(map[string]interface{})
		if !ok {
			continue
		}
		if listen, _ := m["listen"].(string); strings.HasPrefix(listen, "@") {
			return true
		}
		if fb, ok := m["fallbacks"].([]interface{}); ok && len(fb) > 0 {
			return true
		}
	}
	return false
}
