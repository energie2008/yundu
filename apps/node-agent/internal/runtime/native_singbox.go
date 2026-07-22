package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
)

const (
	// sing-box 蓝绿排空超时时间
	defaultDrainTimeout = 30 * time.Second
	// sing-box 排空检查间隔
	defaultDrainCheckInterval = 2 * time.Second
)

// NativeSingbox 基于 sing-box Go 库的原生运行时，支持蓝绿热转（Linux SO_REUSEPORT）。
//
// 与 exec.Command 子进程模式相比：
//   - 配置从内存字节流加载（option.Options.UnmarshalJSONContext），不落盘
//   - 内核在 agent 进程内启动（box.New + box.Start），无进程创建开销
//   - 用户变更采用蓝绿热转策略（SO_REUSEPORT 双实例共存，老实例连接排空后关闭）
//   - 在不支持 SO_REUSEPORT 的平台上回退为快速优雅重启（仍远快于 exec.Command）
type NativeSingbox struct {
	mu          sync.Mutex
	activeBox   *box.Box       // 当前服务新连接的实例（"绿"）
	draining    []*drainingBox // 正在排空的老实例列表（"蓝"）
	logger      *slog.Logger
	startedAt   time.Time
	restartCount atomic.Int64
	configBytes []byte
	configHash  string
	clashAPIPort int // Machine模式分配的Clash API端口，0表示不启用

	// nameToUUID 缓存 sing-box inbound users 的 name→UUID 映射。
	// sing-box 的 ConnTracker 通过 metadata.User（即 name 字段）标识用户，
	// 但流量上报需要 UUID 作为 credential。此映射在 Start 时从配置提取，
	// 用于 GetTrafficStats 将 name 转换为真实 UUID。
	nameToUUID map[string]string

	drainTimeout       time.Duration
	drainCheckInterval time.Duration

	// tracker 用于 per-user 流量统计和 per-user 限速。
	// 通过实现 adapter.ConnectionTracker 接口，在连接路由时包装 conn，
	// 在 Read/Write 时用 atomic.Int64 累加字节数，并通过 RateLimiter 执行限速。
	// 蓝绿热转时 tracker 实例保持不变，累计数据和限速器自动迁移到新实例。
	tracker *ConnTracker
	// speedLimiter 存储 RateLimiter，在 tracker 创建时注入。
	// 通过 SetSpeedLimiter 可在运行时热更新限速配置。
	speedLimiter RateLimiter
	// deviceChecker 存储 DeviceChecker，在 tracker 创建时注入。
	// 通过 SetDeviceChecker 可在运行时热更新设备数限制配置。
	deviceChecker DeviceChecker
	// ipChecker 存储 IPChecker，在 tracker 创建时注入。
	// 通过 SetIPLimiter 可在运行时热更新 IP 限制配置。
	ipChecker IPChecker
}

// drainingBox 封装一个正在排空的老 sing-box 实例。
type drainingBox struct {
	box       *box.Box
	closed    chan struct{}
	startedAt time.Time
}

// NewNativeSingbox 创建 NativeSingbox 实例。
// clashEndpoint 为 Clash API 地址，为空则不启用 Clash API（Machine模式下由分配器指定）。
func NewNativeSingbox(logger *slog.Logger, clashEndpoint string) *NativeSingbox {
	port := 0
	if clashEndpoint != "" {
		if _, portStr, err := net.SplitHostPort(clashEndpoint); err == nil {
			if p, err := strconv.Atoi(portStr); err == nil {
				port = p
			}
		}
	}
	return &NativeSingbox{
		logger:             logger.With("runtime", "native-singbox"),
		drainTimeout:       defaultDrainTimeout,
		drainCheckInterval: defaultDrainCheckInterval,
		clashAPIPort:       port,
	}
}

// Start 从内存字节流启动 sing-box。
// 如果已有实例在运行，采用蓝绿策略：启动新实例，将老实例移入排空队列。
func (s *NativeSingbox) Start(ctx context.Context, configBytes []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// ★Machine模式端口强制改写：无论面板下发的配置中Clash API端口是什么，
	// 最终绑定端口由本地分配器决定（双重保险）
	if s.clashAPIPort > 0 {
		rewritten, err := rewriteClashAPIPort(configBytes, s.clashAPIPort)
		if err != nil {
			s.logger.Warn("failed to rewrite sing-box clash api port, using original config", "error", err)
		} else {
			configBytes = rewritten
		}
	}

	s.logger.Info("starting native sing-box from in-memory config", "config_size", len(configBytes))

	hash := sha256.Sum256(configBytes)
	configHash := hex.EncodeToString(hash[:])

	// 注入 sing-box 协议注册表到 context（sing-box 1.11+ 要求）
	// include.Context 注册所有内置协议（hysteria2/vless/vmess/trojan 等）的 fields registry
	sbCtx := include.Context(ctx)

	// 解析 JSON 配置为 option.Options
	var options option.Options
	if err := options.UnmarshalJSONContext(sbCtx, configBytes); err != nil {
		return fmt.Errorf("native-singbox: parse config: %w", err)
	}

	// 提取 inbound users 的 name→UUID 映射，用于 GetTrafficStats 将
	// ConnTracker 的 userID（name 字段）转换为真实 UUID 作为上报 credential
	s.nameToUUID = extractSingboxNameToUUID(configBytes)

	// 创建新实例
	newBox, err := box.New(box.Options{
		Context: sbCtx,
		Options: options,
	})
	if err != nil {
		return fmt.Errorf("native-singbox: create instance: %w", err)
	}

	// 注册流量统计 + 限速 ConnTracker（在 Start 之前注册）
	// tracker 实例在整个 NativeSingbox 生命周期内复用，蓝绿热转时数据和限速器自动迁移
	if s.tracker == nil {
		s.tracker = NewConnTracker()
		// 注入限速器（如有）
		if s.speedLimiter != nil {
			s.tracker.SetSpeedLimiter(s.speedLimiter)
		}
		// 注入设备检查器（如有）
		if s.deviceChecker != nil {
			s.tracker.SetDeviceChecker(s.deviceChecker)
		}
		// 注入 IP 限制检查器（如有）
		if s.ipChecker != nil {
			s.tracker.SetIPLimiter(s.ipChecker)
		}
	}
	newBox.Router().AppendTracker(s.tracker)

	// 蓝绿策略：如果已有活跃实例，先尝试启动新实例（SO_REUSEPORT）
	// 如果新实例启动失败（端口被占），则优雅关闭旧实例后重启
	if s.activeBox != nil {
		// 尝试启动新实例（SO_REUSEPORT 允许两个实例绑定同一端口）
		if err := newBox.Start(); err != nil {
			s.logger.Warn("blue-green start failed (SO_REUSEPORT unavailable), falling back to graceful restart",
				"error", err)
			// ★关键修复：失败的 newBox 内部可能已部分启动（netlink 路由订阅 goroutine、
			// 部分 inbound）。直接复用同一 newBox 再次 Start() 会触发 sing-box 内部
			// netlink routeSubscribeAt goroutine 的 "close of closed channel" panic。
			// 正确做法：先安全 Close 失败的 newBox（带 recover 保护防止清理时 panic
			// 扩散到主 goroutine），然后重新创建 box 实例再启动。
			safeCloseBox(newBox, s.logger, "failed newBox during blue-green fallback")
			// 清空 newBox 引用，防止后续误用
			newBox = nil

			// 优雅关闭旧实例（释放 UDP 40020 等被占端口）
			s.stopAllLocked()

			// 重新创建 box 实例（不复用部分初始化的 failed box）
			freshBox, ferr := box.New(box.Options{
				Context: sbCtx,
				Options: options,
			})
			if ferr != nil {
				return fmt.Errorf("native-singbox: recreate instance after failed start: %w", ferr)
			}
			freshBox.Router().AppendTracker(s.tracker)

			if err := freshBox.Start(); err != nil {
				safeCloseBox(freshBox, s.logger, "freshBox after graceful stop")
				return fmt.Errorf("native-singbox: start instance after graceful stop: %w", err)
			}
			newBox = freshBox
		} else {
			// 新实例启动成功，将旧实例移入排空队列
			s.startDrainingLocked(s.activeBox)
			s.logger.Info("blue-green rotation: new instance active, old instance draining")
		}
	} else {
		// 首次启动
		if err := newBox.Start(); err != nil {
			// 首次启动失败也需安全 Close，防止 netlink goroutine 泄漏
			safeCloseBox(newBox, s.logger, "newBox on first start failure")
			return fmt.Errorf("native-singbox: start instance: %w", err)
		}
	}

	s.activeBox = newBox
	s.configBytes = make([]byte, len(configBytes))
	copy(s.configBytes, configBytes)
	s.configHash = configHash
	s.startedAt = time.Now()
	s.restartCount.Add(1)

	s.logger.Info("native sing-box started successfully",
		"config_hash", configHash[:16],
		"draining_instances", len(s.draining))

	return nil
}

// startDrainingLocked 将实例移入排空队列（调用方需持有锁）。
func (s *NativeSingbox) startDrainingLocked(oldBox *box.Box) {
	if oldBox == nil {
		return
	}
	db := &drainingBox{
		box:       oldBox,
		closed:    make(chan struct{}),
		startedAt: time.Now(),
	}
	s.draining = append(s.draining, db)
	go s.drainAndClose(db)
}

// drainAndClose 监控老实例连接排空，完成后优雅关闭。
func (s *NativeSingbox) drainAndClose(db *drainingBox) {
	ticker := time.NewTicker(s.drainCheckInterval)
	defer ticker.Stop()
	timer := time.NewTimer(s.drainTimeout)
	defer timer.Stop()

	for {
		select {
		case <-ticker.C:
			// sing-box 未直接暴露活跃连接数 API，
			// 通过 Inbound() 管理器可间接获取，但跨版本不稳定。
			// 保守策略：等待 drainTimeout 后强制关闭（保证连接自然结束）。
			// 将来可通过 Clash API GET /connections 获取精确连接数。
			age := time.Since(db.startedAt)
			s.logger.Debug("draining sing-box instance", "age", age.String())
			// 排空策略：等待到超时时间后关闭，让 TCP 连接自然结束
		case <-timer.C:
			s.logger.Info("draining sing-box instance timeout, closing gracefully",
				"age", time.Since(db.startedAt).String())
			safeCloseBox(db.box, s.logger, "draining box on timeout")
			close(db.closed)
			s.removeFromDrainingList(db)
			return
		}
	}
}

// removeFromDrainingList 从排空列表中移除已关闭的实例。
func (s *NativeSingbox) removeFromDrainingList(target *drainingBox) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, db := range s.draining {
		if db == target {
			s.draining = append(s.draining[:i], s.draining[i+1:]...)
			return
		}
	}
}

// Stop 停止所有 sing-box 实例（active + draining）。
func (s *NativeSingbox) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopAllLocked()
}

// safeCloseBox 安全关闭 box 实例，捕获可能的 panic（如 netlink 订阅 goroutine
// 的 "close of closed channel"）。用于蓝绿热转失败时清理部分初始化的 box 实例。
//
// 注意：此 recover 只能保护主 goroutine。若 panic 来自 sing-box 内部启动的
// 独立 goroutine（如 netlink routeSubscribeAt），仍会导致进程崩溃。
// 因此调用方还须确保不重复 Start 同一 box 实例（见 Start 中的蓝绿回退逻辑）。
func safeCloseBox(b *box.Box, logger *slog.Logger, context string) {
	if b == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			logger.Warn("suppressed panic during box.Close()",
				"context", context,
				"panic", r)
		}
	}()
	b.Close()
}

func (s *NativeSingbox) stopAllLocked() error {
	if s.activeBox != nil {
		safeCloseBox(s.activeBox, s.logger, "activeBox during stopAllLocked")
		s.activeBox = nil
	}
	for _, db := range s.draining {
		safeCloseBox(db.box, s.logger, "draining box during stopAllLocked")
	}
	s.draining = nil
	s.logger.Info("all sing-box instances stopped")
	return nil
}

// UpdateUsers 增量用户更新。
//
// sing-box 没有类似 Xray AlterInbound 的 gRPC 增量 API，
// 因此采用蓝绿热转策略：
//  1. 在本地合并 adds/dels 到当前配置
//  2. 用新配置启动新实例（蓝绿模式，SO_REUSEPORT 共享端口）
//  3. 老实例等待连接排空后关闭
func (s *NativeSingbox) UpdateUsers(ctx context.Context, adds []User, dels []string) error {
	if len(adds) == 0 && len(dels) == 0 {
		return nil
	}

	s.logger.Info("sing-box user update via blue-green rotation",
		"adds", len(adds), "dels", len(dels))

	// 在本地配置中合并用户变更
	s.mu.Lock()
	cfgBytes := s.configBytes
	s.mu.Unlock()

	if len(cfgBytes) == 0 {
		return fmt.Errorf("native-singbox: no current config for user update")
	}

	mergedBytes, err := s.mergeSingboxUserChanges(cfgBytes, adds, dels)
	if err != nil {
		s.logger.Warn("local merge failed, caller should trigger full config fetch", "error", err)
		return fmt.Errorf("merge user changes: %w", err)
	}

	// 蓝绿热转：Start 方法已处理蓝绿逻辑
	return s.Start(ctx, mergedBytes)
}

// mergeSingboxUserChanges 在本地合并用户变更到 sing-box JSON 配置。
//
// sing-box 的用户配置在 inbounds[].users 数组中，每个用户有 name/uuid/password 字段。
func (s *NativeSingbox) mergeSingboxUserChanges(configBytes []byte, adds []User, dels []string) ([]byte, error) {
	var cfg map[string]interface{}
	if err := json.Unmarshal(configBytes, &cfg); err != nil {
		return nil, err
	}

	inbounds, ok := cfg["inbounds"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("no inbounds in sing-box config")
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
		if tag == "api" || tag == "tproxy" || tag == "redirect" {
			continue
		}

		usersRaw, _ := ib["users"].([]interface{})

		// 删除用户
		var newUsers []interface{}
		for _, u := range usersRaw {
			userMap, ok := u.(map[string]interface{})
			if !ok {
				newUsers = append(newUsers, u)
				continue
			}
			name, _ := userMap["name"].(string) // sing-box 用 name 字段标识用户
			if delSet[name] {
				continue
			}
			uuid, _ := userMap["uuid"].(string)
			password, _ := userMap["password"].(string)
			shouldReplace := false
			for _, add := range adds {
				if add.UUID != "" && uuid == add.UUID {
					shouldReplace = true
					break
				}
				if add.Password != "" && password == add.Password {
					shouldReplace = true
					break
				}
			}
			if !shouldReplace {
				newUsers = append(newUsers, u)
			}
		}

		// 添加用户
		for _, add := range adds {
			user := map[string]interface{}{
				"name": add.Email,
			}
			if add.UUID != "" {
				user["uuid"] = add.UUID
			}
			if add.Password != "" {
				user["password"] = add.Password
			}
			newUsers = append(newUsers, user)
		}

		ib["users"] = newUsers
		inbounds[i] = ib
	}

	cfg["inbounds"] = inbounds
	return json.Marshal(cfg)
}

// SetSpeedLimiter 设置 per-user 限速器。
// 传入 nil 可禁用限速。此方法可在运行时调用，已有连接会动态读取最新限速器。
// 如果 tracker 尚未创建（Start 未调用），限速器会存储并在 Start 时注入。
func (s *NativeSingbox) SetSpeedLimiter(l RateLimiter) {
	s.mu.Lock()
	s.speedLimiter = l
	if s.tracker != nil {
		s.tracker.SetSpeedLimiter(l)
	}
	s.mu.Unlock()
}

// SetDeviceChecker 设置 per-user 设备数检查器。
// 传入 nil 可禁用设备数限制。此方法可在运行时调用。
// 如果 tracker 尚未创建（Start 未调用），检查器会存储并在 Start 时注入。
func (s *NativeSingbox) SetDeviceChecker(dc DeviceChecker) {
	s.mu.Lock()
	s.deviceChecker = dc
	if s.tracker != nil {
		s.tracker.SetDeviceChecker(dc)
	}
	s.mu.Unlock()
}

// SetIPLimiter 设置 per-user IP 限制检查器。
// 传入 nil 可禁用 IP 限制。此方法可在运行时调用（热更新 IP 黑白名单/IP数限制）。
// 如果 tracker 尚未创建（Start 未调用），检查器会存储并在 Start 时注入。
func (s *NativeSingbox) SetIPLimiter(ic IPChecker) {
	s.mu.Lock()
	s.ipChecker = ic
	if s.tracker != nil {
		s.tracker.SetIPLimiter(ic)
	}
	s.mu.Unlock()
}

// GetTrafficStats 获取 per-user 流量统计（增量值，读取后清零）。
//
// 通过 ConnTracker 的 GetAndReset 原子读取所有用户流量并清零，
// 返回自上次调用以来的增量字节数。
//
// 数据流：
//   sing-box 路由器 → RoutedConnection → trackedConn.Read/Write → atomic.Int64 累加
//   → GetAndReset (Swap(0)) → TrafficStat → 上报到 traffic-service
//
// 用户标识：sing-box 的 InboundContext.User 字段（对应配置中 inbound users 的 name 字段）。
// name 字段值通常为用户 email（由 kernelrender/singbox.go 的 renderUsersMultiClient 设置）。
// GetTrafficStats 会通过 nameToUUID 映射将 name 转换为真实 UUID，与 xray 侧的 credential 对齐。
func (s *NativeSingbox) GetTrafficStats(ctx context.Context) (map[string]TrafficStat, error) {
	if s.tracker == nil {
		return make(map[string]TrafficStat), nil
	}

	traffic := s.tracker.GetAndReset()
	if len(traffic) == 0 {
		return make(map[string]TrafficStat), nil
	}

	s.mu.Lock()
	nameToUUID := s.nameToUUID
	s.mu.Unlock()

	stats := make(map[string]TrafficStat, len(traffic))
	for userID, t := range traffic {
		// 优先用 nameToUUID 映射获取真实 UUID；映射缺失时回退到 userID 本身
		uuid := userID
		if mapped, ok := nameToUUID[userID]; ok && mapped != "" {
			uuid = mapped
		}
		stats[userID] = TrafficStat{
			Email:    userID, // name 字段值（通常为 email）
			UUID:     uuid,   // 真实 UUID（从配置提取），作为上报 credential
			Upload:   t[0],
			Download: t[1],
		}
	}

	s.logger.Debug("sing-box traffic stats collected",
		"user_count", len(stats),
		"total_upload", sumUpload(stats),
		"total_download", sumDownload(stats))
	return stats, nil
}

// GetTrafficStatsNoReset 非破坏性读取 per-user 流量统计（不清零计数器）。
//
// 用于容错上报模式：调用方计算与上次上报的差值，仅在上报成功后更新基线。
// 上报失败时基线不变，下次读取自动包含未上报的流量，避免数据丢失。
func (s *NativeSingbox) GetTrafficStatsNoReset(ctx context.Context) (map[string]TrafficStat, error) {
	if s.tracker == nil {
		return make(map[string]TrafficStat), nil
	}

	traffic := s.tracker.Get()
	if len(traffic) == 0 {
		return make(map[string]TrafficStat), nil
	}

	s.mu.Lock()
	nameToUUID := s.nameToUUID
	s.mu.Unlock()

	stats := make(map[string]TrafficStat, len(traffic))
	for userID, t := range traffic {
		uuid := userID
		if mapped, ok := nameToUUID[userID]; ok && mapped != "" {
			uuid = mapped
		}
		stats[userID] = TrafficStat{
			Email:    userID,
			UUID:     uuid,
			Upload:   t[0],
			Download: t[1],
		}
	}
	return stats, nil
}

// extractSingboxNameToUUID 从 sing-box 配置 JSON 中提取 inbound users 的 name→UUID 映射。
// 遍历所有 inbound 的 users 数组，将 name 字段映射到 uuid 字段。
// 对于没有 uuid 字段的协议（如 Trojan/Hysteria2/AnyTLS），不加入映射（UUID 回退为 name 本身）。
func extractSingboxNameToUUID(configBytes []byte) map[string]string {
	var cfg struct {
		Inbounds []struct {
			Users []struct {
				Name string `json:"name"`
				UUID string `json:"uuid"`
			} `json:"users"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(configBytes, &cfg); err != nil {
		return nil
	}
	m := make(map[string]string)
	for _, inb := range cfg.Inbounds {
		for _, u := range inb.Users {
			if u.Name != "" && u.UUID != "" {
				m[u.Name] = u.UUID
			}
		}
	}
	return m
}

// sumUpload 计算所有用户上传字节总和（用于日志）。
func sumUpload(stats map[string]TrafficStat) int64 {
	var total int64
	for _, s := range stats {
		total += s.Upload
	}
	return total
}

// sumDownload 计算所有用户下载字节总和（用于日志）。
func sumDownload(stats map[string]TrafficStat) int64 {
	var total int64
	for _, s := range stats {
		total += s.Download
	}
	return total
}

// Status 返回运行时状态。
func (s *NativeSingbox) Status(ctx context.Context) (*PluginStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	status := &PluginStatus{
		Version:      "sing-box (native)",
		ActiveConns:  -1, // 暂无精确连接数
		RestartCount: s.restartCount.Load(),
		ConfigHash:   s.configHash,
	}

	if s.activeBox != nil {
		status.Running = true
		status.Uptime = int64(time.Since(s.startedAt).Seconds())
		status.StartedAt = s.startedAt
	} else {
		status.Running = false
	}

	return status, nil
}

// Validate 在内存中校验配置字节码（不启动内核）。
func (s *NativeSingbox) Validate(configBytes []byte) error {
	if len(configBytes) == 0 {
		return fmt.Errorf("native-singbox: empty config")
	}
	// 注入 sing-box 协议注册表（sing-box 1.11+ 要求）
	sbCtx := include.Context(context.Background())
	var options option.Options
	if err := options.UnmarshalJSONContext(sbCtx, configBytes); err != nil {
		return fmt.Errorf("native-singbox: config validation failed: %w", err)
	}
	return nil
}

// rewriteClashAPIPort 强制覆写 sing-box 配置中 experimental.clash_api.external_controller 端口。
func rewriteClashAPIPort(configBytes []byte, targetPort int) ([]byte, error) {
	var cfg map[string]interface{}
	if err := json.Unmarshal(configBytes, &cfg); err != nil {
		return nil, fmt.Errorf("parse sing-box config: %w", err)
	}

	experimental, ok := cfg["experimental"].(map[string]interface{})
	if !ok {
		experimental = make(map[string]interface{})
		cfg["experimental"] = experimental
	}
	clashAPI, ok := experimental["clash_api"].(map[string]interface{})
	if !ok {
		clashAPI = make(map[string]interface{})
		experimental["clash_api"] = clashAPI
	}
	clashAPI["external_controller"] = fmt.Sprintf("127.0.0.1:%d", targetPort)
	if _, exists := clashAPI["external_controller_listen"]; !exists {
		clashAPI["external_controller_listen"] = "127.0.0.1"
	}

	return json.Marshal(cfg)
}
