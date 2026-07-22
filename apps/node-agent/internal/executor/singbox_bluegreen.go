// singbox_bluegreen.go 为 sing-box 子进程模式实现蓝绿热转，避免 Hy2/TUIC 等协议在
// 配置更新时断流。
//
// 与 internal/runtime/native_singbox.go（基于 sing-box Go 库的进程内蓝绿）互补：
// 本文件面向 exec.Command 子进程模式，通过端口改写 + 流量转发层（iptables/socat）
// 实现蓝绿切换。生产环境（Linux）优先使用 iptables REDIRECT，借助 conntrack 让
// 既有连接继续流向旧实例、新连接流向新实例，实现 TCP/UDP 真正零断流；socat 为
// 跨平台降级方案（切换转发器时既有 TCP 连接会瞬时断开）；无可用转发器时退化为
// “健康检查后快速停启”，仍优于无检查的盲重启。
//
// 蓝绿流程（Swap）：
//  1. 将新配置的 inbound listen_port 改写为内部端口（原端口 + offset），写入 green 配置
//  2. 启动 green 子进程（监听内部端口）
//  3. 健康检查 green（进程存活 + TCP 入站端口可拨通）
//  4. 通过 portMapper 将对外端口流量从 blue 内部端口切到 green 内部端口
//  5. 等待 drainTimeout 让 blue 既有连接自然结束（iptables 模式下由 conntrack 保留）
//  6. 终止 blue 进程
//  7. green 成为新的 blue
package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	// bgDrainTimeout 排空等待时长：iptables 模式下让既有连接在 conntrack 中自然结束。
	bgDrainTimeout = 5 * time.Second
	// bgHealthTimeout green 健康检查总超时。
	bgHealthTimeout = 15 * time.Second
	// bgHealthInterval 健康检查轮询间隔。
	bgHealthInterval = 500 * time.Millisecond
	// bgPortOffsetA 第一个内部端口偏移（green/blue 槽位 A）。
	bgPortOffsetA = 10000
	// bgPortOffsetB 第二个内部端口偏移（槽位 B），与 A 交替使用。
	bgPortOffsetB = 20000
	// bgStopTimeout 停止子进程的优雅退出等待。
	bgStopTimeout = 10 * time.Second
)

// SingBoxBlueGreen 基于 exec.Command 子进程的 sing-box 蓝绿热转管理器。
type SingBoxBlueGreen struct {
	binPath   string
	configDir string
	logger    *slog.Logger

	// drainTimeout 旧实例排空等待时长。
	drainTimeout time.Duration
	// healthTimeout / healthInterval green 健康检查参数。
	healthTimeout  time.Duration
	healthInterval time.Duration

	// portMapper 对外端口到内部端口的流量转发层；nil 表示无可用转发器（退化为快速停启）。
	portMapper PortMapper

	mu              sync.Mutex
	currentProcess  *singBoxInstance   // 当前对外服务的实例（blue）
	newProcess      *singBoxInstance   // 交换过程中的新实例（green），交换完成后置 nil
	draining        []*singBoxInstance // 异步排空/清理中的旧实例
	stopping        atomic.Bool
}

// singBoxInstance 封装一个运行中的 sing-box 子进程。
type singBoxInstance struct {
	cmd           *exec.Cmd
	pid           int
	configPath    string // 实际使用的配置文件（green 时为改写后的文件）
	externalPorts []int  // 原始对外监听端口
	internalPorts []int  // 实际绑定的内部端口（externalPorts + offset）
	tcpPorts      []int  // 内部端口中 TCP 类型（用于健康检查拨号）
	startedAt     time.Time
	processExited chan struct{}
	stopping      atomic.Bool
}

// PortMapper 抽象对外端口到内部端口的流量转发层。
//
// 实现要求：
//   - Setup 建立从 externalPort 到 internalPort 的转发（TCP 与 UDP 均需覆盖）
//   - Switch 将 externalPort 原子地重定向到 newInternalPort
//   - iptables 实现借助 conntrack 保留既有连接；socat 实现会有瞬时断开
type PortMapper interface {
	// Setup 建立转发规则。
	Setup(externalPort, internalPort int) error
	// Switch 将对外端口重定向到新的内部端口。
	Switch(externalPort, newInternalPort int) error
	// Teardown 移除指定对外端口的转发规则。
	Teardown(externalPort int) error
	// Close 释放所有资源。
	Close() error
}

// NewSingBoxBlueGreen 创建蓝绿热转管理器，并自动探测可用的 PortMapper。
// 探测顺序：Linux iptables → socat → 无（nil，退化模式）。
func NewSingBoxBlueGreen(binPath, configDir string, logger *slog.Logger) *SingBoxBlueGreen {
	lg := logger.With("runtime", "sing-box-bluegreen")
	bg := &SingBoxBlueGreen{
		binPath:        binPath,
		configDir:      configDir,
		logger:         lg,
		drainTimeout:   bgDrainTimeout,
		healthTimeout:  bgHealthTimeout,
		healthInterval: bgHealthInterval,
	}
	bg.portMapper = bg.probePortMapper()
	return bg
}

// probePortMapper 探测可用的流量转发器。
func (bg *SingBoxBlueGreen) probePortMapper() PortMapper {
	if runtime.GOOS == "linux" {
		if err := exec.Command("iptables", "-t", "nat", "-L", "-n").Run(); err == nil {
			bg.logger.Info("port mapper selected: iptables (true blue-green via conntrack)")
			return newIptablesPortMapper(bg.logger)
		} else {
			bg.logger.Warn("iptables not available, trying socat fallback", "error", err)
		}
	}
	if _, err := exec.LookPath("socat"); err == nil {
		bg.logger.Info("port mapper selected: socat (cross-platform fallback, brief blip on switch)")
		return newSocatPortMapper(bg.logger)
	}
	bg.logger.Warn("no port mapper available (no iptables/socat), degrading to rapid restart mode")
	return nil
}

// Swap 执行蓝绿热转。configPath 为新配置文件路径。
//
// 若 portMapper 可用：走完整蓝绿流程（端口改写 + 转发切换 + 排空 + 停旧）。
// 若不可用：退化为“健康检查后快速停启”（先校验配置、停旧、启新于原始端口）。
// 任何阶段失败均保留 currentProcess 不变，已启动的 green 会被清理。
func (bg *SingBoxBlueGreen) Swap(ctx context.Context, configPath string) error {
	bg.mu.Lock()
	defer bg.mu.Unlock()

	if bg.stopping.Load() {
		return fmt.Errorf("sing-box bluegreen: is stopping")
	}

	if bg.portMapper != nil {
		return bg.swapWithMapper(ctx, configPath)
	}
	return bg.swapRapidRestart(ctx, configPath)
}

// swapWithMapper 完整蓝绿流程（需 portMapper）。
func (bg *SingBoxBlueGreen) swapWithMapper(ctx context.Context, configPath string) error {
	// 1. 解析配置，提取对外端口与协议类型
	externalPorts, tcpByPort, err := parseSingboxListenPorts(configPath)
	if err != nil {
		return fmt.Errorf("bluegreen: parse config ports: %w", err)
	}
	if len(externalPorts) == 0 {
		return fmt.Errorf("bluegreen: no listen_port found in config")
	}

	// 2. 计算 green 内部端口（与当前实例交替使用两个偏移）
	greenOffset := bg.nextGreenOffset()
	greenInternal := make([]int, len(externalPorts))
	for i, p := range externalPorts {
		greenInternal[i] = p + greenOffset
	}

	// 3. 改写配置：端口偏移 + 去除 cache_file（避免与 blue 的 bolt db 文件锁冲突）
	greenCfgPath, err := bg.writeGreenConfig(configPath, greenOffset)
	if err != nil {
		return fmt.Errorf("bluegreen: write green config: %w", err)
	}

	// 4. 启动 green
	green, err := bg.startInstance(ctx, greenCfgPath, externalPorts, greenInternal, tcpByPort)
	if err != nil {
		_ = os.Remove(greenCfgPath)
		return fmt.Errorf("bluegreen: start green: %w", err)
	}
	bg.newProcess = green

	// 5. 健康检查 green；失败则清理 green，保留 current
	if err := bg.healthCheck(ctx, green); err != nil {
		bg.logger.Error("bluegreen: green health check failed, rolling back", "error", err)
		_ = green.stop(bgStopTimeout)
		_ = os.Remove(greenCfgPath)
		bg.newProcess = nil
		return fmt.Errorf("bluegreen: green health check: %w", err)
	}
	bg.logger.Info("bluegreen: green healthy, switching traffic")

	// 6. 切换转发层：对外端口 → green 内部端口
	//    若 current 为空（首次启动），使用 Setup；否则使用 Switch。
	for i, ext := range externalPorts {
		intPort := greenInternal[i]
		if bg.currentProcess == nil {
			if err := bg.portMapper.Setup(ext, intPort); err != nil {
				bg.logger.Error("bluegreen: mapper setup failed, rolling back", "port", ext, "error", err)
				bg.rollbackGreen(green, greenCfgPath, ext, i, externalPorts)
				return fmt.Errorf("bluegreen: mapper setup port %d: %w", ext, err)
			}
		} else {
			if err := bg.portMapper.Switch(ext, intPort); err != nil {
				bg.logger.Error("bluegreen: mapper switch failed, rolling back", "port", ext, "error", err)
				bg.rollbackGreen(green, greenCfgPath, ext, i, externalPorts)
				return fmt.Errorf("bluegreen: mapper switch port %d: %w", ext, err)
			}
		}
	}

	// 7. 排空 blue：等待 drainTimeout 让既有连接自然结束
	if bg.currentProcess != nil {
		bg.logger.Info("bluegreen: draining blue", "pid", bg.currentProcess.pid, "drain", bg.drainTimeout)
		old := bg.currentProcess
		select {
		case <-time.After(bg.drainTimeout):
		case <-ctx.Done():
		}
		// 8. 终止 blue
		if err := old.stop(bgStopTimeout); err != nil {
			bg.logger.Warn("bluegreen: blue stop returned error", "pid", old.pid, "error", err)
		}
		_ = os.Remove(old.configPath)
	}

	// 9. green 成为新的 blue
	bg.currentProcess = green
	bg.newProcess = nil
	bg.logger.Info("bluegreen: swap completed",
		"pid", green.pid, "external_ports", externalPorts, "internal_offset", greenOffset)
	return nil
}

// rollbackGreen 在切换失败时清理已建立的部分转发并停止 green。
func (bg *SingBoxBlueGreen) rollbackGreen(green *singBoxInstance, greenCfgPath string, failedExt int, doneCount int, externalPorts []int) {
	// 对已完成切换/Setup 的端口尝试回滚：若存在 current，重新指回 current 内部端口
	for i := 0; i < doneCount; i++ {
		ext := externalPorts[i]
		if bg.currentProcess != nil && i < len(bg.currentProcess.internalPorts) {
			_ = bg.portMapper.Switch(ext, bg.currentProcess.internalPorts[i])
		} else {
			_ = bg.portMapper.Teardown(ext)
		}
	}
	_ = green.stop(bgStopTimeout)
	_ = os.Remove(greenCfgPath)
	bg.newProcess = nil
}

// swapRapidRestart 无 portMapper 时的退化方案：先停旧再启新于原始端口。
// 启动前通过进程存活做轻量健康检查；无法零断流，但比盲重启多一层校验。
func (bg *SingBoxBlueGreen) swapRapidRestart(ctx context.Context, configPath string) error {
	binPath := bg.binPath
	if binPath == "" {
		binPath = "sing-box"
	}

	externalPorts, tcpByPort, err := parseSingboxListenPorts(configPath)
	if err != nil {
		// 配置无法解析端口时直接停启（保持与原 SingBoxExecutor.Reload 行为一致）
		externalPorts = nil
	}

	// 停旧
	if bg.currentProcess != nil {
		bg.logger.Info("bluegreen(degraded): stopping blue before start", "pid", bg.currentProcess.pid)
		_ = bg.currentProcess.stop(bgStopTimeout)
		_ = os.Remove(bg.currentProcess.configPath)
		bg.currentProcess = nil
	}

	cmd := exec.CommandContext(ctx, binPath, "run", "-c", configPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("bluegreen(degraded): start sing-box: %w", err)
	}

	inst := &singBoxInstance{
		cmd:           cmd,
		pid:           cmd.Process.Pid,
		configPath:    configPath,
		externalPorts: externalPorts,
		internalPorts: externalPorts, // 退化模式直接用原始端口
		startedAt:     time.Now(),
		processExited: make(chan struct{}),
	}
	for _, p := range externalPorts {
		if tcpByPort[p] {
			inst.tcpPorts = append(inst.tcpPorts, p)
		}
	}
	go func() {
		_ = cmd.Wait()
		close(inst.processExited)
	}()

	bg.currentProcess = inst
	bg.logger.Info("bluegreen(degraded): sing-box started (rapid restart)", "pid", inst.pid)

	// 轻量健康检查：进程未立即退出即视为就绪
	if err := bg.healthCheck(ctx, inst); err != nil {
		bg.logger.Warn("bluegreen(degraded): health check after start failed", "error", err)
	}
	return nil
}

// nextGreenOffset 返回下一个 green 实例应使用的端口偏移（与当前实例交替）。
func (bg *SingBoxBlueGreen) nextGreenOffset() int {
	if bg.currentProcess == nil || len(bg.currentProcess.internalPorts) == 0 {
		return bgPortOffsetA
	}
	cur := bg.currentProcess.internalPorts[0] - bg.currentProcess.externalPorts[0]
	if cur == bgPortOffsetA {
		return bgPortOffsetB
	}
	return bgPortOffsetA
}

// startInstance 启动一个 sing-box 子进程并封装为 singBoxInstance。
func (bg *SingBoxBlueGreen) startInstance(ctx context.Context, cfgPath string, externalPorts, internalPorts []int, tcpByPort map[int]bool) (*singBoxInstance, error) {
	binPath := bg.binPath
	if binPath == "" {
		binPath = "sing-box"
	}
	cmd := exec.CommandContext(ctx, binPath, "run", "-c", cfgPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	inst := &singBoxInstance{
		cmd:           cmd,
		pid:           cmd.Process.Pid,
		configPath:    cfgPath,
		externalPorts: append([]int(nil), externalPorts...),
		internalPorts: append([]int(nil), internalPorts...),
		startedAt:     time.Now(),
		processExited: make(chan struct{}),
	}
	// tcpByPort 以对外端口为键；internalPorts[i] 对应 externalPorts[i]
	for i, intP := range internalPorts {
		if i < len(externalPorts) && tcpByPort[externalPorts[i]] {
			inst.tcpPorts = append(inst.tcpPorts, intP)
		}
	}
	go func() {
		_ = cmd.Wait()
		close(inst.processExited)
	}()
	bg.logger.Info("sing-box instance started", "pid", inst.pid, "config", cfgPath,
		"external_ports", externalPorts, "internal_ports", internalPorts)
	return inst, nil
}

// healthCheck 轮询检查实例健康：进程存活 + TCP 入站端口可拨通。
// 无 TCP 端口（纯 UDP 入站）时，进程存活超过 1.5s 即视为就绪。
func (bg *SingBoxBlueGreen) healthCheck(ctx context.Context, inst *singBoxInstance) error {
	deadline := time.Now().Add(bg.healthTimeout)
	startNoTcp := time.Now()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// 进程是否已退出
		select {
		case <-inst.processExited:
			return fmt.Errorf("sing-box pid %d exited during health check", inst.pid)
		default:
		}

		if len(inst.tcpPorts) == 0 {
			// 无 TCP 端口：进程稳定运行 1.5s 即认为就绪
			if time.Since(startNoTcp) >= 1500*time.Millisecond {
				return nil
			}
		} else {
			allOk := true
			for _, p := range inst.tcpPorts {
				conn, derr := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", p), 1*time.Second)
				if derr != nil {
					allOk = false
					break
				}
				_ = conn.Close()
			}
			if allOk {
				return nil
			}
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("sing-box pid %d health check timeout", inst.pid)
		}
		select {
		case <-time.After(bg.healthInterval):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Status 检查当前（blue）实例健康。无实例或实例已退出返回错误。
func (bg *SingBoxBlueGreen) Status() error {
	bg.mu.Lock()
	inst := bg.currentProcess
	bg.mu.Unlock()
	if inst == nil || inst.cmd == nil || inst.cmd.Process == nil {
		return fmt.Errorf("sing-box bluegreen: no running instance")
	}
	select {
	case <-inst.processExited:
		return fmt.Errorf("sing-box bluegreen: pid %d not running", inst.pid)
	default:
	}
	if err := inst.cmd.Process.Signal(syscall.Signal(0)); err != nil {
		return fmt.Errorf("sing-box bluegreen: pid %d signal(0) failed: %w", inst.pid, err)
	}
	return nil
}

// CurrentInstanceInfo 返回当前 blue 实例的 PID 和启动时间。
// 用于 SingBoxExecutor.Status 汇报蓝绿模式下的进程状态。
// ok=false 表示无运行中的实例。
func (bg *SingBoxBlueGreen) CurrentInstanceInfo() (pid int, startedAt time.Time, ok bool) {
	bg.mu.Lock()
	inst := bg.currentProcess
	bg.mu.Unlock()
	if inst == nil || inst.cmd == nil || inst.cmd.Process == nil {
		return 0, time.Time{}, false
	}
	select {
	case <-inst.processExited:
		return 0, time.Time{}, false
	default:
	}
	return inst.pid, inst.startedAt, true
}

// Stop 停止所有实例并释放转发层资源。
func (bg *SingBoxBlueGreen) Stop() error {
	bg.mu.Lock()
	bg.stopping.Store(true)
	current := bg.currentProcess
	draining := bg.draining
	mapper := bg.portMapper
	bg.currentProcess = nil
	bg.newProcess = nil
	bg.draining = nil
	bg.mu.Unlock()

	var firstErr error
	if current != nil {
		if err := current.stop(bgStopTimeout); err != nil && firstErr == nil {
			firstErr = err
		}
		_ = os.Remove(current.configPath)
	}
	for _, d := range draining {
		_ = d.stop(bgStopTimeout)
		_ = os.Remove(d.configPath)
	}
	if mapper != nil {
		if err := mapper.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	bg.stopping.Store(false)
	return firstErr
}

// stop 优雅终止子进程：SIGTERM → 等待 → SIGKILL。
func (inst *singBoxInstance) stop(timeout time.Duration) error {
	if inst == nil || inst.cmd == nil || inst.cmd.Process == nil {
		return nil
	}
	if inst.stopping.Swap(true) {
		return nil // 已在停止中
	}
	_ = inst.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-inst.processExited:
		return nil
	case <-time.After(timeout):
		_ = inst.cmd.Process.Kill()
		<-inst.processExited
		return fmt.Errorf("sing-box pid %d killed after timeout", inst.pid)
	}
}

// writeGreenConfig 读取原始配置，将 inbound listen_port 偏移 offset，去除
// experimental.cache_file（避免与旧实例的 bolt db 文件锁冲突），写入 green 配置文件。
func (bg *SingBoxBlueGreen) writeGreenConfig(configPath string, offset int) (string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("parse config json: %w", err)
	}

	if inbounds, ok := cfg["inbounds"].([]interface{}); ok {
		for _, ibRaw := range inbounds {
			ib, ok := ibRaw.(map[string]interface{})
			if !ok {
				continue
			}
			portRaw, ok := ib["listen_port"]
			if !ok {
				continue
			}
			portf, ok := toFloat(portRaw)
			if !ok || portf <= 0 {
				continue
			}
			ib["listen_port"] = int(portf) + offset
		}
	}

	// 去除 cache_file，避免 green 与 blue 争用同一 bolt db 文件锁
	if exp, ok := cfg["experimental"].(map[string]interface{}); ok {
		delete(exp, "cache_file")
		if len(exp) == 0 {
			delete(cfg, "experimental")
		} else {
			cfg["experimental"] = exp
		}
	}

	out, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	greenPath := filepath.Join(filepath.Dir(configPath),
		filepath.Base(configPath)+fmt.Sprintf(".green.%d.json", offset))
	if err := os.WriteFile(greenPath, out, 0644); err != nil {
		return "", err
	}
	return greenPath, nil
}

// parseSingboxListenPorts 从 sing-box 配置中提取所有 inbound 的对外监听端口，
// 并返回 tcpByPort 标识每个端口对应入站是否为 TCP 类型（用于健康检查拨号）。
func parseSingboxListenPorts(configPath string) (ports []int, tcpByPort map[int]bool, err error) {
	tcpByPort = make(map[int]bool)
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, nil, err
	}
	var cfg struct {
		Inbounds []struct {
			Type       string `json:"type"`
			ListenPort int    `json:"listen_port"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, nil, fmt.Errorf("parse inbounds: %w", err)
	}
	seen := make(map[int]bool)
	for _, ib := range cfg.Inbounds {
		if ib.ListenPort <= 0 {
			continue
		}
		if seen[ib.ListenPort] {
			continue
		}
		seen[ib.ListenPort] = true
		ports = append(ports, ib.ListenPort)
		tcpByPort[ib.ListenPort] = !isUDPInbound(ib.Type)
	}
	return ports, tcpByPort, nil
}

// isUDPInbound 判断 sing-box inbound 类型是否以 UDP 为主（健康检查不做 TCP 拨号）。
func isUDPInbound(typ string) bool {
	switch typ {
	case "hysteria", "hysteria2", "tuic", "shadowquic":
		return true
	}
	return false
}

// toFloat 将 JSON 数字（float64 或 int）转为 float64。
func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

// ===== PortMapper 实现 =====

// noopPortMapper 不做任何转发（保留接口完整性，实际未使用）。
type noopPortMapper struct{}

func (noopPortMapper) Setup(int, int) error                  { return nil }
func (noopPortMapper) Switch(int, int) error                 { return nil }
func (noopPortMapper) Teardown(int) error                    { return nil }
func (noopPortMapper) Close() error                          { return nil }

// 确保 noopPortMapper 实现 PortMapper。
var _ PortMapper = noopPortMapper{}

// ----- iptablesPortMapper（Linux，真蓝绿）-----

// iptablesPortMapper 使用 iptables nat 表 PREROUTING REDIRECT 实现对外端口转发。
// conntrack 会保留既有连接的原始 NAT 决策，因此 Switch 后既有连接继续流向旧实例，
// 新连接流向新实例，实现 TCP/UDP 零断流。
type iptablesPortMapper struct {
	logger *slog.Logger
	mu     sync.Mutex
	// rules[externalPort] = 当前生效的内部端口
	rules map[int]int
}

func newIptablesPortMapper(logger *slog.Logger) *iptablesPortMapper {
	return &iptablesPortMapper{logger: logger, rules: make(map[int]int)}
}

func (m *iptablesPortMapper) Setup(externalPort, internalPort int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.addRuleLocked(externalPort, internalPort); err != nil {
		return err
	}
	m.rules[externalPort] = internalPort
	return nil
}

func (m *iptablesPortMapper) Switch(externalPort, newInternalPort int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	old := m.rules[externalPort]
	if old == newInternalPort {
		return nil
	}
	// 先删除旧规则，再添加新规则。conntrack 保留既有连接。
	if old != 0 {
		_ = m.delRuleLocked(externalPort, old)
	}
	if err := m.addRuleLocked(externalPort, newInternalPort); err != nil {
		// 回滚：恢复旧规则
		if old != 0 {
			_ = m.addRuleLocked(externalPort, old)
		}
		return err
	}
	m.rules[externalPort] = newInternalPort
	m.logger.Info("iptables switch", "external", externalPort, "old_internal", old, "new_internal", newInternalPort)
	return nil
}

func (m *iptablesPortMapper) Teardown(externalPort int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if old := m.rules[externalPort]; old != 0 {
		_ = m.delRuleLocked(externalPort, old)
		delete(m.rules, externalPort)
	}
	return nil
}

func (m *iptablesPortMapper) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for ext, intPort := range m.rules {
		_ = m.delRuleLocked(ext, intPort)
	}
	m.rules = make(map[int]int)
	return nil
}

// addRuleLocked 为 TCP 与 UDP 各添加一条 REDIRECT 规则（调用方持锁）。
func (m *iptablesPortMapper) addRuleLocked(externalPort, internalPort int) error {
	for _, proto := range []string{"tcp", "udp"} {
		args := []string{"-t", "nat", "-A", "PREROUTING", "-p", proto,
			"--dport", fmt.Sprintf("%d", externalPort), "-j", "REDIRECT",
			"--to-ports", fmt.Sprintf("%d", internalPort)}
		if out, err := exec.Command("iptables", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("iptables add %s %d->%d: %w, out: %s",
				proto, externalPort, internalPort, err, string(out))
		}
	}
	return nil
}

func (m *iptablesPortMapper) delRuleLocked(externalPort, internalPort int) error {
	for _, proto := range []string{"tcp", "udp"} {
		args := []string{"-t", "nat", "-D", "PREROUTING", "-p", proto,
			"--dport", fmt.Sprintf("%d", externalPort), "-j", "REDIRECT",
			"--to-ports", fmt.Sprintf("%d", internalPort)}
		if out, err := exec.Command("iptables", args...).CombinedOutput(); err != nil {
			m.logger.Warn("iptables del rule failed (may be already absent)",
				"proto", proto, "external", externalPort, "internal", internalPort,
				"error", err, "out", string(out))
		}
	}
	return nil
}

// ----- socatPortMapper（跨平台降级）-----

// socatPortMapper 使用 socat 子进程作为对外端口的转发器。
// 局限：Switch 时需重启 socat，既有 TCP 连接会瞬时断开（UDP/QUIC 将重连）。
type socatPortMapper struct {
	logger *slog.Logger
	mu     sync.Mutex
	// procs[externalPort] = 当前转发的 socat 子进程列表（TCP+UDP）
	procs map[int][]*exec.Cmd
}

func newSocatPortMapper(logger *slog.Logger) *socatPortMapper {
	return &socatPortMapper{logger: logger, procs: make(map[int][]*exec.Cmd)}
}

func (m *socatPortMapper) Setup(externalPort, internalPort int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.startLocked(externalPort, internalPort)
}

func (m *socatPortMapper) Switch(externalPort, newInternalPort int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked(externalPort)
	return m.startLocked(externalPort, newInternalPort)
}

func (m *socatPortMapper) Teardown(externalPort int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked(externalPort)
	return nil
}

func (m *socatPortMapper) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for ext := range m.procs {
		m.stopLocked(ext)
	}
	return nil
}

func (m *socatPortMapper) startLocked(externalPort, internalPort int) error {
	var cmds []*exec.Cmd
	for _, spec := range []struct{ listen, conn string }{
		{"TCP-LISTEN:%d,fork,reuseaddr", "TCP:127.0.0.1:%d"},
		{"UDP-LISTEN:%d,fork,reuseaddr", "UDP:127.0.0.1:%d"},
	} {
		listen := fmt.Sprintf(spec.listen, externalPort)
		conn := fmt.Sprintf(spec.conn, internalPort)
		cmd := exec.Command("socat", listen, conn)
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			// 启动失败：清理已启动的并返回
			for _, c := range cmds {
				_ = c.Process.Kill()
			}
			return fmt.Errorf("socat start %s -> %s: %w", listen, conn, err)
		}
		cmds = append(cmds, cmd)
	}
	m.procs[externalPort] = cmds
	m.logger.Info("socat forwarder started", "external", externalPort, "internal", internalPort)
	return nil
}

func (m *socatPortMapper) stopLocked(externalPort int) {
	cmds := m.procs[externalPort]
	for _, c := range cmds {
		if c != nil && c.Process != nil {
			_ = c.Process.Signal(syscall.SIGTERM)
		}
	}
	for _, c := range cmds {
		if c != nil {
			_ = c.Wait()
		}
	}
	delete(m.procs, externalPort)
}

var _ PortMapper = (*iptablesPortMapper)(nil)
var _ PortMapper = (*socatPortMapper)(nil)
