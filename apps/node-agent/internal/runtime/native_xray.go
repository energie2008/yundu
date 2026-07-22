package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	proxymanCmd "github.com/xtls/xray-core/app/proxyman/command"
	statsCmd "github.com/xtls/xray-core/app/stats/command"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/core"
	_ "github.com/xtls/xray-core/main/distro/all" // Register all protocols (VLESS/VMess/Trojan/SS/etc.)
	"github.com/xtls/xray-core/proxy/shadowsocks"
	"github.com/xtls/xray-core/proxy/trojan"
	"github.com/xtls/xray-core/proxy/vless"
	"github.com/xtls/xray-core/proxy/vmess"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	// defaultXrayAPIEndpoint is the default gRPC API endpoint on the xray loopback listener.
	// The control plane (kernelrender) already configures this api inbound (tag=api, port=10085)
	// with HandlerService + StatsService enabled.
	defaultXrayAPIEndpoint = "127.0.0.1:10085"
	// grpcDialTimeout is the timeout for connecting to xray's built-in gRPC API.
	grpcDialTimeout = 3 * time.Second
)

// NativeXray 基于 xray-core Go 库的原生 Xray 运行时。
//
// 与 exec.Command 子进程模式相比：
//   - 配置从内存字节流加载（core.LoadConfig），不落盘
//   - 内核在 agent 进程内启动（core.New + instance.Start），无进程创建开销
//   - 通过 gRPC HandlerService (127.0.0.1:10085) 实现 AlterInbound 增量用户管理
//   - 通过 gRPC StatsService 获取 per-user 流量统计
//   - LKG 配置缓存在内存中，崩溃恢复无需读磁盘
type NativeXray struct {
	mu           sync.RWMutex
	instance     *core.Instance // xray-core 核心实例
	grpcConn     *grpc.ClientConn
	logger       *slog.Logger
	startedAt    time.Time
	restartCount atomic.Int64
	configHash   string
	configBytes  []byte // 当前运行的配置（内存缓存，用于全量重载回退）
	apiEndpoint  string // xray 内置 gRPC API 地址
	assignedPort int    // Machine模式分配的端口，0表示用默认

	// lastStatsValues 缓存上次 QueryStats(Reset=false) 返回的累计值，用于本地计算增量。
	// key 为 stat name（如 "user>>><email>>>traffic>>>uplink"）。
	// 使用 Reset=false 避免上报失败时流量丢失：xray 计数器不清零，下次查询仍可读到完整累计值。
	lastStatsValues map[string]int64
	lastStatsMu     sync.Mutex
}

// NewNativeXray 创建 NativeXray 实例。
// apiEndpoint 若为空则使用默认值 127.0.0.1:10085。
func NewNativeXray(logger *slog.Logger, apiEndpoint string) *NativeXray {
	if apiEndpoint == "" {
		apiEndpoint = defaultXrayAPIEndpoint
	}
	port := 0
	if _, portStr, err := net.SplitHostPort(apiEndpoint); err == nil {
		if p, err := strconv.Atoi(portStr); err == nil {
			port = p
		}
	}
	return &NativeXray{
		logger:       logger.With("runtime", "native-xray"),
		apiEndpoint:  apiEndpoint,
		assignedPort: port,
	}
}

// Start 从内存字节流启动 Xray，不需要落盘到 config.json。
// 如果已有实例在运行，先优雅停止旧实例。
func (x *NativeXray) Start(ctx context.Context, configBytes []byte) error {
	x.mu.Lock()
	defer x.mu.Unlock()

	// 如果已有实例在运行，先停止
	if x.instance != nil {
		x.stopLocked()
	}

	// 防御性去重：面板渲染链路可能重复注入 api inbound/outbound，
	// 导致 xray 启动时报 "existing tag found: api"。
	// 在启动前对 inbounds/outbounds 按 tag 去重，保留第一个出现的条目。
	if deduped, err := dedupeXrayConfigByTag(configBytes); err == nil {
		configBytes = deduped
	} else {
		x.logger.Warn("failed to dedupe xray config tags, proceeding with original", "error", err)
	}

	// ★Machine模式端口强制改写：无论面板下发的配置中api端口是什么，
	// 最终绑定端口由本地分配器决定（双重保险）
	if x.assignedPort > 0 {
		rewritten, err := rewriteAPIInboundPort(configBytes, x.assignedPort)
		if err != nil {
			x.logger.Warn("failed to rewrite xray api inbound port, using original config", "error", err)
		} else {
			configBytes = rewritten
		}
	}

	x.logger.Info("starting native xray from in-memory config", "config_size", len(configBytes))

	// 计算配置 hash
	hash := sha256.Sum256(configBytes)
	x.configHash = hex.EncodeToString(hash[:])

	// 从内存字节流加载 JSON 配置（不落盘）
	// xray-core 支持 "json" 和 "protobuf" 两种格式
	config, err := core.LoadConfig("json", bytes.NewReader(configBytes))
	if err != nil {
		return fmt.Errorf("native-xray: load config from memory: %w", err)
	}

	// 创建 Xray 核心实例
	inst, err := core.New(config)
	if err != nil {
		return fmt.Errorf("native-xray: create instance: %w", err)
	}

	// 启动实例（在当前进程内启动所有 inbound/outbound/dispatcher）
	if err := inst.Start(); err != nil {
		return fmt.Errorf("native-xray: start instance: %w", err)
	}

	x.instance = inst
	x.configBytes = make([]byte, len(configBytes))
	copy(x.configBytes, configBytes)
	x.startedAt = time.Now()

	// xray（重新）启动后计数器归零，清空本地缓存避免产生负 delta
	x.lastStatsMu.Lock()
	x.lastStatsValues = make(map[string]int64)
	x.lastStatsMu.Unlock()

	x.logger.Info("native xray started successfully", "config_hash", x.configHash[:16])

	// 连接到 Xray 内置 gRPC API server（用于 AlterInbound 增量用户管理 + StatsService 流量统计）
	// 给 xray 一点时间启动 API listener
	time.Sleep(300 * time.Millisecond)
	x.connectGRPC()

	return nil
}

// connectGRPC 尝试连接到 xray 内置 gRPC API。失败时仅记录警告，不阻断启动。
func (x *NativeXray) connectGRPC() {
	dialCtx, cancel := context.WithTimeout(context.Background(), grpcDialTimeout)
	defer cancel()

	conn, err := grpc.DialContext(dialCtx, x.apiEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		x.logger.Warn("failed to connect to xray gRPC API, AlterInbound/Stats will be unavailable",
			"endpoint", x.apiEndpoint, "error", err)
		return
	}
	x.grpcConn = conn
	x.logger.Info("connected to xray gRPC API for hot user management & stats",
		"endpoint", x.apiEndpoint)
}

// Stop 优雅关闭 Xray 实例。
func (x *NativeXray) Stop(ctx context.Context) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.stopLocked()
}

func (x *NativeXray) stopLocked() error {
	if x.grpcConn != nil {
		x.grpcConn.Close()
		x.grpcConn = nil
	}
	if x.instance != nil {
		// core.Instance 实现了 io.Closer
		if closer, ok := interface{}(x.instance).(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				x.logger.Warn("xray instance close returned error", "error", err)
			}
		}
		x.instance = nil
		x.logger.Info("native xray stopped")
	}
	return nil
}

// UpdateUsers 增量更新用户。
//
// 实现策略：
//  1. 如果 gRPC HandlerService 可用，尝试通过 AlterInbound 逐个添加/删除用户（零断流）
//  2. 对于 adds：需要构造 protocol.User（含协议专有 Account），这需要知道每个 inbound 的协议类型
//     由于 xray 配置中 users 是嵌套在各 inbound 的 settings.clients 里，且不同协议 Account 结构不同，
//     最可靠的零断流方式是：从 configBytes 中解析 inbounds，提取已有 users，合并 adds/dels，
//     然后通过 gRPC AddInboundUser/RemoveInboundUser 操作。
//  3. 如果 gRPC 不可用或用户构造失败，回退到全量内存重载（Start），这比 exec.Command 快得多。
func (x *NativeXray) UpdateUsers(ctx context.Context, adds []User, dels []string) error {
	if len(adds) == 0 && len(dels) == 0 {
		return nil
	}

	x.mu.RLock()
	conn := x.grpcConn
	cfgBytes := x.configBytes
	x.mu.RUnlock()

	// 如果 gRPC 不可用，回退到全量内存重载
	if conn == nil {
		x.logger.Warn("handler service unavailable, falling back to in-memory full reload",
			"adds", len(adds), "dels", len(dels))
		return x.fullReload(ctx, nil)
	}

	// 尝试通过 gRPC AddInboundUser/RemoveInboundUser 进行真增量更新
	// 先从当前配置中解析 inbound 信息
	if err := x.alterInboundViaGRPC(ctx, conn, cfgBytes, adds, dels); err != nil {
		x.logger.Warn("gRPC AlterInbound failed, falling back to in-memory full reload",
			"error", err, "adds", len(adds), "dels", len(dels))
		// 合并 adds/dels 到 configBytes 后全量重载
		mergedBytes, mergeErr := mergeUserChanges(cfgBytes, adds, dels)
		if mergeErr != nil {
			x.logger.Error("failed to merge user changes, using cached config for reload", "error", mergeErr)
			return x.fullReload(ctx, nil)
		}
		return x.fullReload(ctx, mergedBytes)
	}

	// E4 修复：gRPC AlterInbound 成功后，同步更新内存中的 configBytes。
	// 如果不更新，后续 xray 崩溃被 watchdog 重启时，会使用旧配置丢失增量用户变更；
	// 后续 fullReload 也会基于过期配置重建。
	if mergedBytes, mergeErr := mergeUserChanges(cfgBytes, adds, dels); mergeErr != nil {
		x.logger.Warn("gRPC AlterInbound succeeded but failed to update in-memory configBytes",
			"error", mergeErr)
	} else {
		x.mu.Lock()
		hash := sha256.Sum256(mergedBytes)
		x.configBytes = make([]byte, len(mergedBytes))
		copy(x.configBytes, mergedBytes)
		x.configHash = hex.EncodeToString(hash[:])
		x.mu.Unlock()
	}

	x.logger.Info("users updated via gRPC AlterInbound (zero-disruption)",
		"adds", len(adds), "dels", len(dels))
	return nil
}

// alterInboundViaGRPC 通过 xray gRPC HandlerService 执行 AddInboundUser/RemoveInboundUser。
// B15 修复：实现真正的 gRPC 增量用户管理，替代原先的假实现（mergeUserChanges + fullReload）。
func (x *NativeXray) alterInboundViaGRPC(ctx context.Context, conn *grpc.ClientConn,
	configBytes []byte, adds []User, dels []string) error {

	// 解析配置获取 inbound 信息（tag + protocol + existing users）
	inbounds, err := parseXrayInbounds(configBytes)
	if err != nil {
		return fmt.Errorf("parse inbounds: %w", err)
	}

	x.logger.Info("attempting gRPC AlterInbound", "inbounds", len(inbounds), "adds", len(adds), "dels", len(dels))

	// 创建 HandlerService 客户端
	handlerClient := proxymanCmd.NewHandlerServiceClient(conn)

	// 为每个 inbound 执行增量用户操作
	// 按 inbound tag 分组 adds（根据 Extra.inbound_tag 匹配）
	addsByTag := make(map[string][]User)
	for _, u := range adds {
		tag := ""
		if u.Extra != nil {
			if t, ok := u.Extra["inbound_tag"].(string); ok {
				tag = t
			}
		}
		// 如果没有指定 tag，会广播到所有非 api inbound
		addsByTag[tag] = append(addsByTag[tag], u)
	}

	var errs []string
	processedAny := false

	for _, ib := range inbounds {
		tag, _ := ib["tag"].(string)
		// 跳过 api inbound
		if tag == "api" {
			continue
		}
		protocolType, _ := ib["protocol"].(string)

		// 处理该 inbound 的添加用户
		taggedAdds := addsByTag[tag]
		broadcastAdds := addsByTag[""] // 无指定 tag 的用户广播到所有 inbound
		allAdds := append(taggedAdds, broadcastAdds...)

		for _, u := range allAdds {
			account, err := buildProtocolAccount(protocolType, u)
			if err != nil {
				errs = append(errs, fmt.Sprintf("build account for %s (%s): %v", u.Email, protocolType, err))
				continue
			}

			// 使用 AlterInbound + AddUserOperation（xray-core gRPC API 正确调用方式）
			addOp := &proxymanCmd.AddUserOperation{
				User: &protocol.User{
					Email:   u.Email,
					Level:   uint32(u.Level),
					Account: account,
				},
			}
			req := &proxymanCmd.AlterInboundRequest{
				Tag:       tag,
				Operation: serial.ToTypedMessage(addOp),
			}
			_, err = handlerClient.AlterInbound(ctx, req)
			if err != nil {
				x.logger.Warn("AlterInbound AddUser failed, trying next inbound",
					"tag", tag, "email", u.Email, "error", err)
				errs = append(errs, fmt.Sprintf("add user %s to %s: %v", u.Email, tag, err))
				continue
			}
			x.logger.Info("AddInboundUser succeeded", "tag", tag, "email", u.Email)
			processedAny = true
		}

		// 处理删除用户
		for _, email := range dels {
			removeOp := &proxymanCmd.RemoveUserOperation{Email: email}
			req := &proxymanCmd.AlterInboundRequest{
				Tag:       tag,
				Operation: serial.ToTypedMessage(removeOp),
			}
			_, err := handlerClient.AlterInbound(ctx, req)
			if err != nil {
				// 删除不存在的用户不算错误，记录 debug 即可
				x.logger.Debug("RemoveInboundUser returned error (may not exist)",
					"tag", tag, "email", email, "error", err)
				continue
			}
			x.logger.Info("RemoveInboundUser succeeded", "tag", tag, "email", email)
			processedAny = true
		}
	}

	if len(errs) > 0 && !processedAny {
		return fmt.Errorf("all gRPC AlterInbound operations failed: %s", strings.Join(errs, "; "))
	}

	if len(errs) > 0 {
		x.logger.Warn("some gRPC AlterInbound operations had errors",
			"errors", strings.Join(errs, "; "), "processed", processedAny)
	}

	return nil
}

// buildProtocolAccount 根据协议类型构建 xray protocol.Account。
func buildProtocolAccount(protocolType string, u User) (*serial.TypedMessage, error) {
	switch protocolType {
	case "vless":
		account := &vless.Account{
			Id: u.UUID,
		}
		// 从 Extra 提取 flow 字段
		if u.Extra != nil {
			if flow, ok := u.Extra["flow"].(string); ok && flow != "" {
				account.Flow = flow
			}
		}
		return serial.ToTypedMessage(account), nil

	case "vmess":
		account := &vmess.Account{
			Id: u.UUID,
		}
		// VMess in xray-core 26.x no longer uses AlterId (removed in favor of AEAD)
		return serial.ToTypedMessage(account), nil

	case "trojan":
		account := &trojan.Account{
			Password: u.Password,
		}
		if u.Password == "" {
			account.Password = u.UUID
		}
		return serial.ToTypedMessage(account), nil

	case "shadowsocks":
		account := &shadowsocks.Account{
			Password: u.Password,
		}
		if u.Password == "" {
			account.Password = u.UUID
		}
		// 从 Extra 提取 method/cipher
		if u.Extra != nil {
			if method, ok := u.Extra["method"].(string); ok && method != "" {
				account.CipherType = shadowsocksCipherFromString(method)
			}
		}
		return serial.ToTypedMessage(account), nil

	default:
		return nil, fmt.Errorf("unsupported protocol type: %s", protocolType)
	}
}

// shadowsocksCipherFromString 将密码算法名称映射到 xray-core CipherType 枚举。
func shadowsocksCipherFromString(method string) shadowsocks.CipherType {
	switch strings.ToLower(method) {
	case "aes-128-gcm":
		return shadowsocks.CipherType_AES_128_GCM
	case "aes-256-gcm":
		return shadowsocks.CipherType_AES_256_GCM
	case "chacha20-poly1305", "chacha20-ietf-poly1305":
		return shadowsocks.CipherType_CHACHA20_POLY1305
	case "xchacha20-poly1305", "xchacha20-ietf-poly1305":
		return shadowsocks.CipherType_XCHACHA20_POLY1305
	case "none":
		return shadowsocks.CipherType_NONE
	default:
		// 默认使用 AES-256-GCM（最安全的通用选项）
		return shadowsocks.CipherType_AES_256_GCM
	}
}

// parseXrayInbounds 从 xray JSON 配置中提取 inbound 列表。
func parseXrayInbounds(configBytes []byte) ([]map[string]interface{}, error) {
	var cfg map[string]interface{}
	if err := json.Unmarshal(configBytes, &cfg); err != nil {
		return nil, err
	}
	inboundsRaw, ok := cfg["inbounds"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("no inbounds in config")
	}
	var result []map[string]interface{}
	for _, ib := range inboundsRaw {
		if m, ok := ib.(map[string]interface{}); ok {
			result = append(result, m)
		}
	}
	return result, nil
}

// mergeUserChanges 在本地合并用户变更到 xray JSON 配置。
func mergeUserChanges(configBytes []byte, adds []User, dels []string) ([]byte, error) {
	var cfg map[string]interface{}
	if err := json.Unmarshal(configBytes, &cfg); err != nil {
		return nil, err
	}
	inbounds, ok := cfg["inbounds"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("no inbounds in config")
	}

	delSet := make(map[string]bool)
	for _, email := range dels {
		delSet[email] = true
	}

	for i, ibRaw := range inbounds {
		ib, ok := ibRaw.(map[string]interface{})
		if !ok {
			continue
		}
		tag, _ := ib["tag"].(string)
		// 跳过 api inbound
		if tag == "api" {
			continue
		}

		settings, _ := ib["settings"].(map[string]interface{})
		if settings == nil {
			continue
		}
		clientsRaw, _ := settings["clients"].([]interface{})

		// 删除用户
		var newClients []interface{}
		for _, c := range clientsRaw {
			client, ok := c.(map[string]interface{})
			if !ok {
				newClients = append(newClients, c)
				continue
			}
			email, _ := client["email"].(string) // xray clients use "email" field
			if delSet[email] {
				continue
			}
			id, _ := client["id"].(string)
			password, _ := client["password"].(string)
			// 也通过 id/password 匹配
			shouldDel := false
			for _, add := range adds {
				if add.UUID != "" && id == add.UUID {
					// 将要被替换，先删除旧的
					shouldDel = true
					break
				}
				if add.Password != "" && password == add.Password {
					shouldDel = true
					break
				}
			}
			if !shouldDel {
				newClients = append(newClients, c)
			}
		}

		// 添加用户
		protocol, _ := ib["protocol"].(string)
		for _, add := range adds {
			client := buildXrayClient(protocol, add)
			if client != nil {
				newClients = append(newClients, client)
			}
		}

		settings["clients"] = newClients
		ib["settings"] = settings
		inbounds[i] = ib
	}

	cfg["inbounds"] = inbounds
	return json.Marshal(cfg)
}

// buildXrayClient 根据协议类型构建 xray client 对象。
//
// TODO(U1): Per-user speed limit integration for xray (external process mode).
// xray does not have native per-user speed limiting in its JSON config.
// The intended approach for enforcing SpeedLimitMbps per user:
//   1. For native mode (NativeXray): After user connects, use StatsService to monitor
//      per-user traffic rates, and when a user exceeds their limit, throttle via
//      xray's gRPC HandlerService (RemoveInboundUser to disconnect, or use policy-level
//      rate limiting via xray's Policy system with per-level bandwidth settings).
//   2. For subprocess mode: Use OS-level traffic control (tc/iptables) with per-IP
//      rate limiting, where each user's IP is tracked via StatsService email→IP mapping.
//   3. Alternative: Leverage xray's built-in Policy level system — assign each user a
//      unique "level" with corresponding bandwidth limits in xray's policy.config.
//      This requires rendering per-user policy entries in the xray config's policy.levels.
// The SpeedLimiter in internal/limiter/speed_limiter.go is already wired via
// syncLimiters() in main.go, but only as metadata passthrough. Actual enforcement
// at the xray data path is not yet implemented.
func buildXrayClient(protocol string, u User) map[string]interface{} {
	client := map[string]interface{}{
		"email": u.Email,
		"level": u.Level,
	}
	switch protocol {
	case "vless":
		client["id"] = u.UUID
		if flow, ok := u.Extra["flow"]; ok {
			client["flow"] = flow
		}
		client["encryption"] = "none"
	case "vmess":
		client["id"] = u.UUID
		if alterId, ok := u.Extra["alterId"]; ok {
			client["alterId"] = alterId
		} else {
			client["alterId"] = 0
		}
		client["security"] = "auto"
	case "trojan":
		client["password"] = u.UUID
	case "shadowsocks":
		if u.Password != "" {
			client["password"] = u.Password
		} else {
			client["password"] = u.UUID
		}
		if method, ok := u.Extra["method"]; ok {
			client["method"] = method
		} else {
			client["method"] = "chacha20-ietf-poly1305"
		}
	default:
		// 默认使用 VLESS 格式
		client["id"] = u.UUID
	}
	return client
}

// fullReload 全量内存重载：停止旧实例，用新配置启动新实例。
// 如果 newConfigBytes 为 nil，使用缓存的 configBytes。
func (x *NativeXray) fullReload(ctx context.Context, newConfigBytes []byte) error {
	x.mu.Lock()
	defer x.mu.Unlock()

	cfgBytes := newConfigBytes
	if cfgBytes == nil {
		cfgBytes = x.configBytes
	}
	if len(cfgBytes) == 0 {
		return fmt.Errorf("native-xray: no config available for full reload")
	}

	x.stopLocked()

	config, err := core.LoadConfig("json", bytes.NewReader(cfgBytes))
	if err != nil {
		return fmt.Errorf("native-xray reload: load config: %w", err)
	}
	inst, err := core.New(config)
	if err != nil {
		return fmt.Errorf("native-xray reload: create instance: %w", err)
	}
	if err := inst.Start(); err != nil {
		return fmt.Errorf("native-xray reload: start instance: %w", err)
	}

	hash := sha256.Sum256(cfgBytes)
	x.instance = inst
	x.configHash = hex.EncodeToString(hash[:])
	x.configBytes = make([]byte, len(cfgBytes))
	copy(x.configBytes, cfgBytes)
	x.startedAt = time.Now()
	x.restartCount.Add(1)

	// xray 重启后计数器归零，清空本地缓存避免产生负 delta
	x.lastStatsMu.Lock()
	x.lastStatsValues = make(map[string]int64)
	x.lastStatsMu.Unlock()

	// 重连 gRPC API
	time.Sleep(300 * time.Millisecond)
	x.connectGRPC()

	x.logger.Info("native xray in-memory full reload completed",
		"restart_count", x.restartCount.Load())
	return nil
}

// GetTrafficStats 通过 StatsService.QueryStats(Reset=false) 获取 per-user 流量增量统计。
//
// 改造说明（P1）：原实现使用 Reset=true 读取后清零计数器，存在以下问题：
//   - 多消费者场景冲突（如健康检查 + 流量上报同时读取会互相清零）
//   - agent 崩溃恢复后无法获取崩溃期间的流量（计数器已被清零）
//
// 新实现使用 Reset=false + 本地增量计算：
//   - xray 计数器保持单调递增（不清零），多次读取不会互相影响
//   - 本地维护 lastStatsValues 缓存上次累计值，delta = current - last
//   - xray 重启导致计数器归零时（current < last），将 current 作为全量增量
//   - 返回值仍为增量（与原接口语义一致），调用方无需修改
func (x *NativeXray) GetTrafficStats(ctx context.Context) (map[string]TrafficStat, error) {
	x.mu.RLock()
	conn := x.grpcConn
	configBytes := x.configBytes
	x.mu.RUnlock()

	if conn == nil {
		return nil, fmt.Errorf("native-xray: stats service unavailable (gRPC not connected)")
	}

	statsClient := statsCmd.NewStatsServiceClient(conn)
	// 使用 Reset=false，不清零 xray 计数器，本地计算增量
	resp, err := statsClient.QueryStats(ctx, &statsCmd.QueryStatsRequest{Reset_: false})
	if err != nil {
		return nil, fmt.Errorf("query stats: %w", err)
	}

	// 从当前配置中提取 email → UUID 映射，用于流量上报凭证
	emailToUUID := extractEmailUUIDMap(configBytes)

	// 本地计算增量：delta = current - lastValue
	x.lastStatsMu.Lock()
	defer x.lastStatsMu.Unlock()

	if x.lastStatsValues == nil {
		x.lastStatsValues = make(map[string]int64)
	}

	stats := make(map[string]TrafficStat)
	for _, stat := range resp.GetStat() {
		name := stat.GetName()
		current := stat.GetValue()
		if current == 0 {
			continue
		}

		// 计算增量
		last := x.lastStatsValues[name]
		var delta int64
		if current >= last {
			delta = current - last
		} else {
			// xray 重启导致计数器归零，用当前值作为全量增量
			delta = current
		}
		// 更新缓存为当前累计值
		x.lastStatsValues[name] = current

		if delta == 0 {
			continue
		}

		// 解析 "user>>><email>>>traffic>>>uplink|downlink"
		parts := strings.Split(name, ">>>")
		if len(parts) != 4 || parts[0] != "user" || parts[2] != "traffic" {
			continue
		}
		email := parts[1]
		direction := parts[3]

		ts, ok := stats[email]
		if !ok {
			ts = TrafficStat{Email: email}
			if uid, found := emailToUUID[email]; found {
				ts.UUID = uid
			}
		}
		switch direction {
		case "uplink":
			ts.Upload += delta
		case "downlink":
			ts.Download += delta
		}
		stats[email] = ts
	}

	x.logger.Debug("traffic stats queried (Reset=false, local delta)", "user_count", len(stats))
	return stats, nil
}

// GetTrafficStatsNoReset 非破坏性读取 per-user 流量统计（不清零计数器）。
//
// 通过 QueryStats(Reset=false) 读取当前累计值（自 xray 启动以来的单调递增计数器）。
// 返回累计值，调用方需自行跟踪基线计算增量：
//   delta = current - lastReported
//   上报成功后更新 lastReported = current
//   上报失败时 lastReported 不变，下次自动包含未上报流量
//
// xray 重启时计数器归零，调用方应检测 current < lastReported 并将 current 作为全量增量。
func (x *NativeXray) GetTrafficStatsNoReset(ctx context.Context) (map[string]TrafficStat, error) {
	x.mu.RLock()
	conn := x.grpcConn
	configBytes := x.configBytes
	x.mu.RUnlock()

	if conn == nil {
		return nil, fmt.Errorf("native-xray: stats service unavailable (gRPC not connected)")
	}

	statsClient := statsCmd.NewStatsServiceClient(conn)
	// 非破坏性读取：Reset=false
	resp, err := statsClient.QueryStats(ctx, &statsCmd.QueryStatsRequest{Reset_: false})
	if err != nil {
		return nil, fmt.Errorf("query stats (no reset): %w", err)
	}

	// 调试日志：打印 QueryStats 原始返回值
	rawStats := resp.GetStat()
	if len(rawStats) == 0 {
		x.logger.Debug("QueryStats returned empty",
			"endpoint", x.apiEndpoint)
	}

	emailToUUID := extractEmailUUIDMap(configBytes)

	// 返回当前累计值（调用方负责计算增量）
	stats := make(map[string]TrafficStat)
	skippedZero := 0
	skippedFormat := 0
	for _, stat := range rawStats {
		name := stat.GetName()
		value := stat.GetValue()
		if value == 0 {
			skippedZero++
			continue
		}
		parts := strings.Split(name, ">>>")
		if len(parts) != 4 || parts[0] != "user" || parts[2] != "traffic" {
			skippedFormat++
			continue
		}
		email := parts[1]
		direction := parts[3]

		ts, ok := stats[email]
		if !ok {
			ts = TrafficStat{Email: email}
			if uid, found := emailToUUID[email]; found {
				ts.UUID = uid
			}
		}
		switch direction {
		case "uplink":
			ts.Upload += value
		case "downlink":
			ts.Download += value
		}
		stats[email] = ts
	}

	x.logger.Debug("QueryStats parsed result",
		"total_raw", len(rawStats),
		"skipped_zero", skippedZero,
		"skipped_format", skippedFormat,
		"valid_stats", len(stats))

	return stats, nil
}

// extractEmailUUIDMap 从 xray 配置 JSON 中提取 email → UUID/password 映射。
// xray inbound clients 结构: inbounds[].settings.clients[].{email, id, password}
// VLESS/VMess: 凭证在 id 字段
// Trojan/SS: 凭证在 password 字段
// 映射结果用于流量上报时将 xray StatsService 的 email key 关联到用户凭证。
func extractEmailUUIDMap(configBytes []byte) map[string]string {
	result := make(map[string]string)
	if len(configBytes) == 0 {
		return result
	}
	var config struct {
		Inbounds []struct {
			Settings struct {
				Clients []struct {
					Email    string `json:"email"`
					ID       string `json:"id"`
					Password string `json:"password"`
				} `json:"clients"`
			} `json:"settings"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(configBytes, &config); err != nil {
		return result
	}
	for _, ib := range config.Inbounds {
		for _, c := range ib.Settings.Clients {
			if c.Email == "" {
				continue
			}
			// 优先用 id 字段（VLESS/VMess），回退到 password 字段（Trojan/SS）
			cred := c.ID
			if cred == "" {
				cred = c.Password
			}
			if cred != "" {
				result[c.Email] = cred
			}
		}
	}
	return result
}

// Status 返回运行时状态。
func (x *NativeXray) Status(ctx context.Context) (*PluginStatus, error) {
	x.mu.RLock()
	defer x.mu.RUnlock()

	status := &PluginStatus{
		Version:      "xray 26.3.27 (native)",
		ActiveConns:  -1, // xray-core 不直接暴露活跃连接数
		RestartCount: x.restartCount.Load(),
		ConfigHash:   x.configHash,
	}

	if x.instance != nil {
		status.Running = true
		status.Uptime = int64(time.Since(x.startedAt).Seconds())
		status.StartedAt = x.startedAt
	} else {
		status.Running = false
	}

	return status, nil
}

// Validate 在内存中校验配置字节码（不启动内核）。
func (x *NativeXray) Validate(configBytes []byte) error {
	if len(bytes.TrimSpace(configBytes)) == 0 {
		return fmt.Errorf("native-xray: empty config")
	}
	// 尝试加载 JSON 配置但不启动实例
	_, err := core.LoadConfig("json", bytes.NewReader(configBytes))
	if err != nil {
		return fmt.Errorf("native-xray: config validation failed: %w", err)
	}
	return nil
}

// GetConfigBytes 返回当前运行的配置字节（用于 LKG 内存回滚）。
func (x *NativeXray) GetConfigBytes() []byte {
	x.mu.RLock()
	defer x.mu.RUnlock()
	result := make([]byte, len(x.configBytes))
	copy(result, x.configBytes)
	return result
}

// rewriteAPIInboundPort 强制覆写 xray 配置中 tag=="api" 的 inbound 端口为 targetPort。
// 若配置中不存在 api inbound，则追加一个标准的 api inbound 配置（防御性）。
func rewriteAPIInboundPort(configBytes []byte, targetPort int) ([]byte, error) {
	var cfg map[string]interface{}
	if err := json.Unmarshal(configBytes, &cfg); err != nil {
		return nil, fmt.Errorf("parse xray config: %w", err)
	}

	inboundsRaw, ok := cfg["inbounds"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("no inbounds array in config")
	}

	found := false
	newInbounds := make([]interface{}, 0, len(inboundsRaw))
	for _, ibRaw := range inboundsRaw {
		ib, ok := ibRaw.(map[string]interface{})
		if !ok {
			newInbounds = append(newInbounds, ibRaw)
			continue
		}
		tag, _ := ib["tag"].(string)
		if tag == "api" {
			ib["port"] = targetPort
			listen, _ := ib["listen"].(string)
			if listen == "" {
				ib["listen"] = "127.0.0.1"
			}
			found = true
		}
		newInbounds = append(newInbounds, ib)
	}

	// 防御性：若没有api inbound，追加一个
	if !found {
		apiInbound := map[string]interface{}{
			"tag":    "api",
			"port":   targetPort,
			"listen": "127.0.0.1",
			"protocol": "dokodemo-door",
			"settings": map[string]interface{}{
				"address": "127.0.0.1",
			},
		}
		newInbounds = append(newInbounds, apiInbound)
	}

	cfg["inbounds"] = newInbounds
	return json.Marshal(cfg)
}

// dedupeXrayConfigByTag 对 xray 配置中的 inbounds 和 outbounds 按 tag 去重。
// 面板渲染链路（kernelrender + kernel_render_adapter）可能因双重调用 ensureAPIInbound
// 导致重复注入 tag="api" 的 inbound/outbound，xray-core 启动时会报
// "existing tag found: api" 致命错误。
// 去重策略：保留第一个出现的 tag，丢弃后续重复条目。
func dedupeXrayConfigByTag(configBytes []byte) ([]byte, error) {
	var cfg map[string]interface{}
	if err := json.Unmarshal(configBytes, &cfg); err != nil {
		return nil, fmt.Errorf("parse xray config: %w", err)
	}

	dedupeList := func(key string) {
		raw, ok := cfg[key].([]interface{})
		if !ok {
			return
		}
		seen := make(map[string]bool)
		deduped := make([]interface{}, 0, len(raw))
		for _, item := range raw {
			m, ok := item.(map[string]interface{})
			if !ok {
				deduped = append(deduped, item)
				continue
			}
			tag, _ := m["tag"].(string)
			if tag == "" {
				deduped = append(deduped, item)
				continue
			}
			if seen[tag] {
				continue
			}
			seen[tag] = true
			deduped = append(deduped, item)
		}
		cfg[key] = deduped
	}

	dedupeList("inbounds")
	dedupeList("outbounds")

	return json.Marshal(cfg)
}
