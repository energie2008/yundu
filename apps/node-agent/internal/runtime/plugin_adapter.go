package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/airport-panel/node-agent/internal/executor"
	"github.com/airport-panel/node-agent/internal/limiter"
)

// PluginAdapter 将 RuntimePlugin 适配为 executor.RuntimeExecutor 接口。
// 这样原生内嵌模式可以无缝接入现有的 applyConfig 流程（写文件、DryRun、PreCheckEdge、HotDiff 等），
// 同时利用 RuntimePlugin 的内存启动、零断流热更能力。
//
// 工作模式：
//   - Validate/DryRun: 调用 plugin.Validate（内存校验，不调用外部进程）
//   - Apply: 将配置写入磁盘（兼容现有 pipeline LKG/nginx/firewall），同时缓存到内存
//   - Reload: 调用 plugin.Start（内存重启，不创建子进程）
//   - AlterInbound: 调用 plugin.UpdateUsers（零断流热更）
//   - Stop/Status: 委托给 plugin
//
// 限速器复用 executor.LimiterIntegration，避免重复实现。
type PluginAdapter struct {
	plugin      RuntimePlugin
	logger      *slog.Logger
	configDir   string
	configPath  string
	runtimeType string

	mu           sync.RWMutex
	currentBytes []byte
	lastApplied  time.Time
	*executor.LimiterIntegration // 复用现有限速器集成
}

// NewPluginAdapter 创建 RuntimePlugin 到 RuntimeExecutor 的适配器。
func NewPluginAdapter(plugin RuntimePlugin, configDir, configPath, runtimeType string, logger *slog.Logger) *PluginAdapter {
	li := executor.NewLimiterIntegration(logger)
	a := &PluginAdapter{
		plugin:             plugin,
		logger:             logger.With("adapter", "plugin-executor"),
		configDir:          configDir,
		configPath:         configPath,
		runtimeType:        runtimeType,
		LimiterIntegration: li,
	}
	// 将 SpeedLimiter/DeviceChecker/IPChecker 注入到原生运行时，实现 per-connection 限速/设备限制/IP限制。
	// SpeedLimiter 实例在 LimiterIntegration 中创建，状态由 syncLimiters 更新。
	// DeviceChecker 通过 SingboxDeviceChecker 适配 DeviceLimiter + 设备限制查询。
	// IPChecker 通过 SingboxIPChecker 适配 IPLimiter + IP数限制查询。
	// 原生 sing-box 的 ConnTracker 通过这些实例在连接级执行限速/设备检查/IP检查。
	if mp, ok := plugin.(*MultiRuntimePlugin); ok {
		mp.SetSingboxSpeedLimiter(li.SpeedLimiter())
		dc := executor.NewSingboxDeviceChecker(li.DeviceLimiter(), li.GetDeviceLimit)
		mp.SetSingboxDeviceChecker(dc)
		ipc := executor.NewSingboxIPChecker(li.IPLimiter(), li.GetIPLimit)
		mp.SetSingboxIPLimiter(ipc)
		logger.Info("speed limiter, device checker and ip checker wired to native sing-box runtime")
	}
	return a
}

func (a *PluginAdapter) Validate(configContent string) error {
	if configContent == "" {
		return fmt.Errorf("plugin-adapter: empty config")
	}
	return a.plugin.Validate([]byte(configContent))
}

func (a *PluginAdapter) Apply(configPath string, content string) error {
	a.logger.Info("applying config via native plugin", "path", configPath, "size", len(content))

	// 解析限速器元数据
	a.ParseLimiterConfig(content)

	// 写入磁盘文件（保留兼容：pipeline LKG、nginx 同步、firewall 等可能读取）
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		a.logger.Error("failed to create config directory", "error", err)
		return err
	}
	tmpPath := configPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, configPath); err != nil {
		return err
	}

	// 缓存到内存
	a.mu.Lock()
	a.currentBytes = []byte(content)
	a.configPath = configPath
	a.mu.Unlock()

	a.logger.Info("config written to disk and cached in memory")
	return nil
}

func (a *PluginAdapter) DryRun(ctx context.Context, configPath string) error {
	a.logger.Info("dry-running via native plugin (in-memory validation)")

	data, err := os.ReadFile(configPath)
	if err != nil {
		a.mu.RLock()
		data = a.currentBytes
		a.mu.RUnlock()
		if len(data) == 0 {
			return fmt.Errorf("plugin-adapter: no config to dry-run")
		}
	}

	// 剥离元数据字段后进行内存校验
	cleanBytes, stripErr := stripMetaFields(data)
	if stripErr != nil {
		cleanBytes = data
	}

	// 内存校验（不调用外部 xray/sing-box 进程）
	return a.plugin.Validate(cleanBytes)
}

func (a *PluginAdapter) Reload(ctx context.Context, configPath string) error {
	a.logger.Info("reloading via native plugin (in-memory restart)")

	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		a.mu.RLock()
		data = a.currentBytes
		a.mu.RUnlock()
		if len(data) == 0 {
			return fmt.Errorf("plugin-adapter: no config for reload")
		}
	}

	// 更新限速器
	a.ParseLimiterConfig(string(data))

	// 剥离特殊字段（_nginx_vhosts, _limiter 等），避免内核 JSON 解析失败
	cleanBytes, err := stripMetaFields(data)
	if err != nil {
		a.logger.Warn("failed to strip meta fields, using raw config", "error", err)
		cleanBytes = data
	}

	// 内存重启（旧实例优雅关闭，新实例在同一进程内启动）
	if err := a.plugin.Start(ctx, cleanBytes); err != nil {
		return fmt.Errorf("plugin-adapter: native reload failed: %w", err)
	}

	a.mu.Lock()
	a.currentBytes = cleanBytes
	a.lastApplied = time.Now()
	a.mu.Unlock()

	a.logger.Info("native plugin reload completed (zero-downtime)")
	return nil
}

func (a *PluginAdapter) Status(ctx context.Context) (*executor.RuntimeStatus, error) {
	ps, err := a.plugin.Status(ctx)
	if err != nil {
		return nil, err
	}

	status := &executor.RuntimeStatus{
		Running:      ps.Running,
		PID:          0, // 原生模式无独立子进程
		Version:      ps.Version,
		ConfigHash:   ps.ConfigHash,
		RestartCount: ps.RestartCount,
	}
	if ps.Running {
		status.Uptime = ps.Uptime
		status.StartedAt = ps.StartedAt
	}
	return status, nil
}

func (a *PluginAdapter) Stop(ctx context.Context) error {
	return a.plugin.Stop(ctx)
}

func (a *PluginAdapter) Rollback() error {
	a.logger.Info("rollback requested for native plugin")
	// LKG 回滚由 pipeline 处理（从 .bak 文件恢复后调用 Reload）
	backupPath := a.configPath + ".bak"
	if _, err := os.Stat(backupPath); err == nil {
		if err := os.Rename(backupPath, a.configPath); err != nil {
			return err
		}
	}
	return nil
}

// AlterInbound 通过 RuntimePlugin.UpdateUsers 实现零断流用户增删。
func (a *PluginAdapter) AlterInbound(ctx context.Context, users []executor.AlterUser) error {
	if len(users) == 0 {
		return nil
	}

	var adds []User
	var dels []string
	added, removed, modified := 0, 0, 0

	for _, u := range users {
		switch u.Op {
		case executor.AlterUserAdded, executor.AlterUserModified:
			added++
			user := User{
				Email: u.Email,
				Level: 0,
				Extra: map[string]interface{}{
					"inbound_tag": u.InboundTag,
				},
			}
			if u.Account != nil {
				if id, ok := u.Account["id"].(string); ok && id != "" {
					user.UUID = id
				}
				if uuid, ok := u.Account["uuid"].(string); ok && uuid != "" {
					user.UUID = uuid
				}
				if password, ok := u.Account["password"].(string); ok && password != "" {
					user.Password = password
				}
				if level, ok := u.Account["level"].(int); ok {
					user.Level = level
				} else if levelf, ok := u.Account["level"].(float64); ok {
					user.Level = int(levelf)
				}
				if flow, ok := u.Account["flow"].(string); ok && flow != "" {
					user.Extra["flow"] = flow
				}
			}
			adds = append(adds, user)
		case executor.AlterUserRemoved:
			removed++
			dels = append(dels, u.Email)
		}
		_ = modified // modified 统计在 added 中
	}

	a.logger.Info("alter inbound via native plugin",
		"added", added, "removed", removed)

	return a.plugin.UpdateUsers(ctx, adds, dels)
}

// GetCurrentConfigBytes 返回当前内存中的配置字节（用于 LKG 内存回滚）。
func (a *PluginAdapter) GetCurrentConfigBytes() []byte {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result := make([]byte, len(a.currentBytes))
	copy(result, a.currentBytes)
	return result
}

// StartNative 从内存字节直接启动原生内核（用于 applyNative 路径，跳过磁盘写入）。
func (a *PluginAdapter) StartNative(ctx context.Context, configBytes []byte) error {
	cleanBytes, err := stripMetaFields(configBytes)
	if err != nil {
		a.logger.Warn("failed to strip meta fields, using raw config", "error", err)
		cleanBytes = configBytes
	}
	if err := a.plugin.Start(ctx, cleanBytes); err != nil {
		return err
	}
	a.mu.Lock()
	a.currentBytes = cleanBytes
	a.lastApplied = time.Now()
	a.mu.Unlock()
	return nil
}

// stripMetaFields 从配置 JSON 中剥离 _nginx_vhosts / _limiter / _traffic_quota / _singbox_config / _chain_bridges 等元数据字段，
// 防止内核 JSON 解析失败。
func stripMetaFields(data []byte) ([]byte, error) {
	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	delete(cfg, "_nginx_vhosts")
	delete(cfg, "_limiter")
	delete(cfg, "_traffic_quota")
	delete(cfg, "_singbox_config")
	delete(cfg, "_chain_bridges")
	return json.Marshal(cfg)
}

// 确保 PluginAdapter 实现了 LimiterUpdater/DeviceLimiterProvider/IPLimiterProvider 接口。
var _ executor.LimiterUpdater = (*PluginAdapter)(nil)
var _ executor.DeviceLimiterProvider = (*PluginAdapter)(nil)
var _ executor.IPLimiterProvider = (*PluginAdapter)(nil)
var _ executor.RuntimeExecutor = (*PluginAdapter)(nil)

// 导入未使用的 limiter 包以保持引用（LimiterIntegration 已使用 limiter 包）。
var _ *limiter.SpeedLimiter = nil
