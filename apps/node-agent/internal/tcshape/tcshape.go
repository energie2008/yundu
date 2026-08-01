// Package tcshape 为 xray 直连节点提供基于 Linux tc/iptables 的每用户带宽整形。
//
// 为什么需要它：xray-core 的 policy 不支持 up_mbps/down_mbps 字段，
// 且 native xray 在进程内没有连接级拦截点，无法像 sing-box ConnTracker 那样
// 在 Read/Write 路径上做令牌桶限速。对直连（listen != 127.0.0.1）的 xray
// inbound，可以按用户来源 IP 用 HTB 整形：
//   - 下载（server -> client）：主网卡 egress HTB
//   - 上传（client -> server）：IFB 设备 + ingress mirror 后 HTB
//
// 限制：CDN/Tunnel 节点（nginx/cloudflared 从 127.0.0.1 回源）无法按用户 IP
// 区分，这类节点不参与 tc 整形，仍由 sing-box ConnTracker 负责限速。
package tcshape

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Rule 描述单个来源 IP 的上下行带宽限制。
type Rule struct {
	ID       int    `json:"id"`
	IP       string `json:"ip"`
	UpKbps   int    `json:"up_kbps"`
	DownKbps int    `json:"down_kbps"`
	Ports    []int  `json:"ports"`
}

// Plan 是一次需要生效的整形计划。
type Plan struct {
	Rules []Rule `json:"rules"`
}

// Runner 抽象命令执行，便于单测。
type Runner interface {
	Run(ctx context.Context, name string, args ...string) error
}

// ExecRunner 使用 os/exec 执行系统命令。
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) error {
	return exec.CommandContext(ctx, name, args...).Run()
}

// Manager 管理 HTB 整形规则的增删（幂等，支持状态持久化恢复）。
type Manager struct {
	dev       string
	ifb       string
	serverIP  string
	statePath string
	logger    *slog.Logger
	runner    Runner

	mu     sync.Mutex
	active map[int]Rule
	nextID int
}

// Options 是 Manager 的构造参数。
type Options struct {
	Dev       string // 主网卡（下载整形）
	Ifb       string // IFB 网卡（上传整形），默认 ifb0
	ServerIP  string // 本机对外 IP（iptables OUTPUT -s 使用）
	StateDir  string // 状态文件目录
	Logger    *slog.Logger
	Runner    Runner
}

// NewManager 创建 Manager。Dev/ServerIP 为空时通过 ip route 自动探测。
func NewManager(opts Options) (*Manager, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Runner == nil {
		opts.Runner = ExecRunner{}
	}
	if opts.Dev == "" || opts.ServerIP == "" {
		dev, ip, err := detectRoute(opts.Runner)
		if err != nil {
			return nil, err
		}
		if opts.Dev == "" {
			opts.Dev = dev
		}
		if opts.ServerIP == "" {
			opts.ServerIP = ip
		}
	}
	if opts.Ifb == "" {
		opts.Ifb = "ifb0"
	}
	m := &Manager{
		dev:       opts.Dev,
		ifb:       opts.Ifb,
		serverIP:  opts.ServerIP,
		statePath: filepath.Join(opts.StateDir, "tc_limits.json"),
		logger:    opts.Logger.With("component", "tc-shape"),
		runner:    opts.Runner,
		active:    make(map[int]Rule),
		nextID:    1000,
	}
	m.loadState()
	return m, nil
}

// Dev 返回主网卡名。
func (m *Manager) Dev() string { return m.dev }

// ServerIP 返回本机 IP。
func (m *Manager) ServerIP() string { return m.serverIP }

// EnsureBase 创建 HTB 根队列与 IFB 镜像（幂等，已存在时忽略错误）。
func (m *Manager) EnsureBase(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ctx = context.WithoutCancel(ctx)
	cmds := [][]string{
		{"tc", "qdisc", "add", "dev", m.dev, "root", "handle", "1:", "htb", "default", "1"},
		{"tc", "class", "add", "dev", m.dev, "parent", "1:", "classid", "1:1", "htb", "rate", "10gbit", "ceil", "10gbit"},
		{"ip", "link", "add", "dev", m.ifb, "type", "ifb"},
		{"ip", "link", "set", "dev", m.ifb, "up"},
		{"tc", "qdisc", "add", "dev", m.ifb, "root", "handle", "1:", "htb", "default", "1"},
		{"tc", "class", "add", "dev", m.ifb, "parent", "1:", "classid", "1:1", "htb", "rate", "10gbit", "ceil", "10gbit"},
		{"tc", "qdisc", "add", "dev", m.dev, "handle", "ffff:", "ingress"},
		{"tc", "filter", "add", "dev", m.dev, "parent", "ffff:", "protocol", "ip", "prio", "1", "u32", "match", "u32", "0", "0", "action", "mirred", "egress", "redirect", "dev", m.ifb},
	}
	var firstErr error
	for _, c := range cmds {
		if err := m.run(ctx, c...); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	if firstErr != nil {
		// 网络整形属于尽力而为：基础队列创建失败不阻断节点运行
		m.logger.Warn("tc base setup incomplete (some commands may already exist)",
			"dev", m.dev, "ifb", m.ifb, "error", firstErr)
	}
	return nil
}

// Apply 将计划应用到 tc/iptables（增量 diff，幂等）。
func (m *Manager) Apply(ctx context.Context, plan Plan) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.ensureBaseLocked(ctx); err != nil {
		return err
	}

	sort.Slice(plan.Rules, func(i, j int) bool { return plan.Rules[i].IP < plan.Rules[j].IP })
	planned := make(map[int]Rule, len(plan.Rules))
	ipToID := make(map[string]int, len(plan.Rules))
	for _, r := range plan.Rules {
		if id, ok := ipToID[r.IP]; ok {
			// 同一 IP 只保留一条规则（合并端口）
			old := planned[id]
			old.Ports = mergePorts(old.Ports, r.Ports)
			if r.UpKbps > old.UpKbps {
				old.UpKbps = r.UpKbps
			}
			if r.DownKbps > old.DownKbps {
				old.DownKbps = r.DownKbps
			}
			planned[id] = old
			continue
		}
		id := m.nextFreeID()
		r.ID = id
		planned[id] = r
		ipToID[r.IP] = id
	}

	// 移除已消失的规则
	for id, rule := range m.active {
		if _, ok := planned[id]; !ok {
			m.removeLocked(ctx, rule)
			delete(m.active, id)
		}
	}

	// 新增/更新规则
	for id, rule := range planned {
		old, exists := m.active[id]
		if exists && sameRule(old, rule) {
			continue
		}
		if exists {
			m.removeLocked(ctx, old)
		}
		if err := m.addLocked(ctx, rule); err != nil {
			m.logger.Warn("failed to apply tc rule", "ip", rule.IP, "error", err)
			continue
		}
		m.active[id] = rule
	}
	m.saveState()
	return nil
}

// Clear 清理所有已应用的规则（agent 退出/禁用时调用）。
func (m *Manager) Clear(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, rule := range m.active {
		m.removeLocked(ctx, rule)
		delete(m.active, id)
	}
	m.saveState()
	return nil
}

func (m *Manager) ensureBaseLocked(ctx context.Context) error {
	ctx = context.WithoutCancel(ctx)
	cmds := [][]string{
		{"tc", "qdisc", "add", "dev", m.dev, "root", "handle", "1:", "htb", "default", "1"},
		{"tc", "class", "add", "dev", m.dev, "parent", "1:", "classid", "1:1", "htb", "rate", "10gbit", "ceil", "10gbit"},
		{"ip", "link", "add", "dev", m.ifb, "type", "ifb"},
		{"ip", "link", "set", "dev", m.ifb, "up"},
		{"tc", "qdisc", "add", "dev", m.ifb, "root", "handle", "1:", "htb", "default", "1"},
		{"tc", "class", "add", "dev", m.ifb, "parent", "1:", "classid", "1:1", "htb", "rate", "10gbit", "ceil", "10gbit"},
		{"tc", "qdisc", "add", "dev", m.dev, "handle", "ffff:", "ingress"},
		{"tc", "filter", "add", "dev", m.dev, "parent", "ffff:", "protocol", "ip", "prio", "1", "u32", "match", "u32", "0", "0", "action", "mirred", "egress", "redirect", "dev", m.ifb},
	}
	var failed int
	for _, c := range cmds {
		if err := m.run(ctx, c...); err != nil {
			failed++
		}
	}
	// 幂等初始化：已存在的 qdisc/class/filter 返回错误属正常现象，
	// 不阻断后续规则应用；全部失败时才视为整形不可用。
	if failed == len(cmds) {
		return fmt.Errorf("tc base setup failed entirely (dev=%s ifb=%s)", m.dev, m.ifb)
	}
	return nil
}

func (m *Manager) addLocked(ctx context.Context, r Rule) error {
	ctx = context.WithoutCancel(ctx)
	id := r.ID
	// iptables 标记：按来源 IP + 目标端口标记上行，按目标 IP + 源端口标记下行
	for _, chunk := range portChunks(r.Ports, 15) {
		ports := strings.Join(portStrs(chunk), ",")
		for _, proto := range []string{"tcp", "udp"} {
			if err := m.iptablesAdd(ctx, "PREROUTING", "-s", r.IP, "-p", proto, "-m", "multiport", "--dports", ports, "-j", "MARK", "--set-mark", fmt.Sprintf("%d", id)); err != nil {
				return err
			}
			if err := m.iptablesAdd(ctx, "OUTPUT", "-d", r.IP, "-p", proto, "-m", "multiport", "--sports", ports, "-j", "MARK", "--set-mark", fmt.Sprintf("%d", id)); err != nil {
				return err
			}
		}
	}
	// 下载：主网卡 HTB class + filter
	if r.DownKbps > 0 {
		if err := m.addClass(ctx, m.dev, id, r.DownKbps); err != nil {
			return err
		}
	}
	// 上传：IFB HTB class + filter
	if r.UpKbps > 0 {
		if err := m.addClass(ctx, m.ifb, id, r.UpKbps); err != nil {
			return err
		}
	}
	m.logger.Info("tc rule applied",
		"ip", r.IP, "id", id, "up_kbps", r.UpKbps, "down_kbps", r.DownKbps,
		"ports", fmt.Sprint(r.Ports))
	return nil
}

func (m *Manager) removeLocked(ctx context.Context, r Rule) {
	ctx = context.WithoutCancel(ctx)
	id := r.ID
	for _, chunk := range portChunks(r.Ports, 15) {
		ports := strings.Join(portStrs(chunk), ",")
		for _, proto := range []string{"tcp", "udp"} {
			m.run(ctx, "iptables", "-t", "mangle", "-D", "PREROUTING", "-s", r.IP, "-p", proto, "-m", "multiport", "--dports", ports, "-j", "MARK", "--set-mark", fmt.Sprintf("%d", id))
			m.run(ctx, "iptables", "-t", "mangle", "-D", "OUTPUT", "-d", r.IP, "-p", proto, "-m", "multiport", "--sports", ports, "-j", "MARK", "--set-mark", fmt.Sprintf("%d", id))
		}
	}
	m.run(ctx, "tc", "filter", "del", "dev", m.dev, "parent", "1:", "prio", "1", "handle", fmt.Sprintf("%d", id), "fw")
	m.run(ctx, "tc", "class", "del", "dev", m.dev, "classid", fmt.Sprintf("1:%d", id))
	m.run(ctx, "tc", "filter", "del", "dev", m.ifb, "parent", "1:", "prio", "1", "handle", fmt.Sprintf("%d", id), "fw")
	m.run(ctx, "tc", "class", "del", "dev", m.ifb, "classid", fmt.Sprintf("1:%d", id))
	m.logger.Debug("tc rule removed", "ip", r.IP, "id", id)
}

func (m *Manager) addClass(ctx context.Context, dev string, id, kbps int) error {
	m.run(ctx, "tc", "class", "del", "dev", dev, "classid", fmt.Sprintf("1:%d", id))
	if err := m.run(ctx, "tc", "class", "add", "dev", dev, "parent", "1:1", "classid", fmt.Sprintf("1:%d", id), "htb", "rate", fmt.Sprintf("%dkbit", kbps), "ceil", fmt.Sprintf("%dkbit", kbps)); err != nil {
		return err
	}
	m.run(ctx, "tc", "filter", "del", "dev", dev, "parent", "1:", "prio", "1", "handle", fmt.Sprintf("%d", id), "fw")
	return m.run(ctx, "tc", "filter", "add", "dev", dev, "parent", "1:", "protocol", "ip", "prio", "1", "handle", fmt.Sprintf("%d", id), "fw", "flowid", fmt.Sprintf("1:%d", id))
}

func (m *Manager) iptablesAdd(ctx context.Context, chain string, args ...string) error {
	full := append([]string{"-t", "mangle", "-C", chain}, args...)
	if m.run(ctx, append([]string{"iptables"}, full...)...) == nil {
		return nil // 规则已存在
	}
	addArgs := append([]string{"-t", "mangle", "-A", chain}, args...)
	return m.run(ctx, append([]string{"iptables"}, addArgs...)...)
}

func (m *Manager) run(ctx context.Context, args ...string) error {
	if len(args) == 0 {
		return nil
	}
	runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	name := args[0]
	if err := m.runner.Run(runCtx, name, args[1:]...); err != nil {
		m.logger.Debug("command failed (may be idempotent)", "cmd", name, "args", strings.Join(args[1:], " "), "error", err)
		return err
	}
	return nil
}

func (m *Manager) nextFreeID() int {
	for {
		if _, ok := m.active[m.nextID]; !ok {
			id := m.nextID
			m.nextID++
			return id
		}
		m.nextID++
	}
}

func (m *Manager) loadState() {
	data, err := os.ReadFile(m.statePath)
	if err != nil {
		return
	}
	var st struct {
		NextID int    `json:"next_id"`
		Rules  []Rule `json:"rules"`
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return
	}
	if st.NextID > m.nextID {
		m.nextID = st.NextID
	}
	for _, r := range st.Rules {
		if r.ID >= m.nextID {
			m.nextID = r.ID + 1
		}
		m.active[r.ID] = r
	}
}

func (m *Manager) saveState() {
	st := struct {
		NextID int    `json:"next_id"`
		Rules  []Rule `json:"rules"`
	}{
		NextID: m.nextID,
		Rules:  make([]Rule, 0, len(m.active)),
	}
	for _, r := range m.active {
		st.Rules = append(st.Rules, r)
	}
	sort.Slice(st.Rules, func(i, j int) bool { return st.Rules[i].ID < st.Rules[j].ID })
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return
	}
	tmp := m.statePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err == nil {
		_ = os.Rename(tmp, m.statePath)
	}
}

func detectRoute(runner Runner) (string, string, error) {
	ctx := context.Background()
	out, err := exec.CommandContext(ctx, "ip", "route", "get", "1.1.1.1").Output()
	if err != nil {
		return "", "", fmt.Errorf("detect default route: %w", err)
	}
	fields := strings.Fields(string(out))
	dev, src := "", ""
	for i := 0; i < len(fields)-1; i++ {
		switch fields[i] {
		case "dev":
			dev = fields[i+1]
		case "src":
			src = fields[i+1]
		}
	}
	if dev == "" || src == "" {
		return "", "", fmt.Errorf("cannot parse ip route output: %s", strings.TrimSpace(string(out)))
	}
	return dev, src, nil
}

func portStrs(ports []int) []string {
	out := make([]string, 0, len(ports))
	for _, p := range ports {
		out = append(out, fmt.Sprintf("%d", p))
	}
	return out
}

func portChunks(ports []int, size int) [][]int {
	if len(ports) == 0 {
		return nil
	}
	var out [][]int
	for i := 0; i < len(ports); i += size {
		end := i + size
		if end > len(ports) {
			end = len(ports)
		}
		out = append(out, ports[i:end])
	}
	return out
}

func mergePorts(a, b []int) []int {
	seen := make(map[int]bool, len(a)+len(b))
	var out []int
	for _, p := range append(append([]int{}, a...), b...) {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Ints(out)
	return out
}

func sameRule(a, b Rule) bool {
	return a.IP == b.IP && a.UpKbps == b.UpKbps && a.DownKbps == b.DownKbps &&
		len(a.Ports) == len(b.Ports) && portsEqual(a.Ports, b.Ports)
}

func portsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	aa := append([]int{}, a...)
	bb := append([]int{}, b...)
	sort.Ints(aa)
	sort.Ints(bb)
	for i := range aa {
		if aa[i] != bb[i] {
			return false
		}
	}
	return true
}
