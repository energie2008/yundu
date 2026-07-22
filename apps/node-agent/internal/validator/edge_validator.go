package validator

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"log/slog"

	"github.com/airport-panel/node-agent/internal/machine"
)

// PreCheckResult 是预检结果
type PreCheckResult struct {
	Passed   bool     `json:"passed"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Duration string   `json:"duration"`
}

// EdgeValidator 执行边缘侧预检
type EdgeValidator struct {
	logger *slog.Logger
}

// NewEdgeValidator 创建新的边缘预检器
func NewEdgeValidator(logger *slog.Logger) *EdgeValidator {
	if logger == nil {
		logger = slog.Default()
	}
	return &EdgeValidator{logger: logger}
}

// PreCheckEdge 执行完整的边缘预检流程
// configJSON 是解密后的 xray/sing-box 配置 JSON
// kernelType 是 "xray" 或 "sing-box"
func (v *EdgeValidator) PreCheckEdge(ctx context.Context, configJSON []byte, kernelType string) (*PreCheckResult, error) {
	start := time.Now()
	result := &PreCheckResult{Passed: true}

	// 获取自身已监听的端口（用于delta reload场景：旧内核实例仍占用端口，属于正常情况）
	selfPorts := v.getSelfListeningPorts()

	// Phase 1: 端口强占检测
	ports, err := extractInboundPorts(configJSON)
	if err != nil {
		result.Passed = false
		result.Errors = append(result.Errors, fmt.Sprintf("extract inbound ports: %v", err))
		result.Duration = time.Since(start).String()
		return result, nil
	}

	// 去重：Xray/sing-box允许多个inbound共享同一端口（不同协议/SNI分流）
	ports = deduplicatePorts(ports)

	v.logger.Info("edge pre-check: extracted inbound ports", "count", len(ports), "ports", ports, "self_listening", selfPorts)

	for _, port := range ports {
		if port <= 0 || port > 65535 {
			continue
		}
		// 跳过内部 API 端口（使用范围判断）
		if machine.IsInternalAPIPort(port) {
			continue
		}

		// 如果端口已经被自身进程监听（delta reload场景），跳过检测
		if _, ok := selfPorts[port]; ok {
			v.logger.Debug("edge pre-check: port already held by self (reload scenario)", "port", port)
			continue
		}

		addr := fmt.Sprintf(":%d", port)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			result.Passed = false
			result.Errors = append(result.Errors, fmt.Sprintf("port %d is already in use: %v", port, err))
			v.logger.Warn("edge pre-check: port conflict detected", "port", port, "error", err)
			continue
		}
		listener.Close()
		v.logger.Debug("edge pre-check: port available", "port", port)
	}

	// Phase 2: JSON 语法校验（基础校验，内核 -test 由 executor DryRun 完成）
	var cfg map[string]interface{}
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		result.Passed = false
		result.Errors = append(result.Errors, fmt.Sprintf("config JSON syntax error: %v", err))
		result.Duration = time.Since(start).String()
		return result, nil
	}

	// Phase 3: 内核特定校验
	switch kernelType {
	case "xray":
		if errs := validateXrayConfig(cfg); len(errs) > 0 {
			result.Warnings = append(result.Warnings, errs...)
		}
	case "sing-box":
		if errs := validateSingboxConfig(cfg); len(errs) > 0 {
			result.Warnings = append(result.Warnings, errs...)
		}
	}

	result.Duration = time.Since(start).String()
	if result.Passed {
		v.logger.Info("edge pre-check passed", "ports", len(ports), "duration", result.Duration)
	} else {
		v.logger.Warn("edge pre-check failed", "errors", result.Errors, "duration", result.Duration)
	}

	return result, nil
}

// extractInboundPorts 从配置 JSON 中提取所有 inbound 监听端口
func extractInboundPorts(configJSON []byte) ([]int, error) {
	var cfg map[string]interface{}
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return nil, err
	}

	var ports []int

	// xray 格式: inbounds[].port
	if inbounds, ok := cfg["inbounds"].([]interface{}); ok {
		for _, ib := range inbounds {
			if m, ok := ib.(map[string]interface{}); ok {
				if port, ok := toInt(m["port"]); ok {
					ports = append(ports, port)
				}
			}
		}
	}

	// sing-box 格式: inbounds[].listen_port
	if inbounds, ok := cfg["inbounds"].([]interface{}); ok {
		for _, ib := range inbounds {
			if m, ok := ib.(map[string]interface{}); ok {
				if port, ok := toInt(m["listen_port"]); ok {
					ports = append(ports, port)
				}
			}
		}
	}

	return ports, nil
}

// validateXrayConfig 对 xray 配置做基础语义校验
func validateXrayConfig(cfg map[string]interface{}) []string {
	var warnings []string

	inbounds, ok := cfg["inbounds"].([]interface{})
	if !ok || len(inbounds) == 0 {
		warnings = append(warnings, "xray config has no inbounds")
		return warnings
	}

	for i, ib := range inbounds {
		m, ok := ib.(map[string]interface{})
		if !ok {
			warnings = append(warnings, fmt.Sprintf("inbound[%d] is not an object", i))
			continue
		}

		protocol, _ := m["protocol"].(string)
		if protocol == "" {
			warnings = append(warnings, fmt.Sprintf("inbound[%d] missing protocol", i))
		}

		tag, _ := m["tag"].(string)
		if tag == "" {
			warnings = append(warnings, fmt.Sprintf("inbound[%d] (%s) missing tag", i, protocol))
		}

		// 检查 streamSettings 中的 security
		if ss, ok := m["streamSettings"].(map[string]interface{}); ok {
			security, _ := ss["security"].(string)
			if security == "tls" {
				tlsSettings, _ := ss["tlsSettings"].(map[string]interface{})
				if tlsSettings == nil {
					warnings = append(warnings, fmt.Sprintf("inbound[%d] (%s) has tls security but no tlsSettings", i, tag))
				}
			}
		}
	}

	return warnings
}

// validateSingboxConfig 对 sing-box 配置做基础语义校验
func validateSingboxConfig(cfg map[string]interface{}) []string {
	var warnings []string

	inbounds, ok := cfg["inbounds"].([]interface{})
	if !ok || len(inbounds) == 0 {
		warnings = append(warnings, "sing-box config has no inbounds")
		return warnings
	}

	for i, ib := range inbounds {
		m, ok := ib.(map[string]interface{})
		if !ok {
			warnings = append(warnings, fmt.Sprintf("inbound[%d] is not an object", i))
			continue
		}

		typeStr, _ := m["type"].(string)
		if typeStr == "" {
			warnings = append(warnings, fmt.Sprintf("inbound[%d] missing type", i))
		}

		tag, _ := m["tag"].(string)
		if tag == "" {
			warnings = append(warnings, fmt.Sprintf("inbound[%d] (%s) missing tag", i, typeStr))
		}
	}

	return warnings
}

// toInt 尝试将 interface{} 转换为 int
func toInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil
	case string:
		// 尝试解析字符串端口
		if !strings.Contains(n, "-") {
			var i int
			_, err := fmt.Sscanf(n, "%d", &i)
			return i, err == nil
		}
	}
	return 0, false
}

// deduplicatePorts 对端口列表去重，保持原有顺序
func deduplicatePorts(ports []int) []int {
	seen := make(map[int]bool)
	result := make([]int, 0, len(ports))
	for _, p := range ports {
		if !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}
	return result
}

// getSelfListeningPorts 获取当前进程及其子进程（如 xray/sing-box 内核子进程）已监听的TCP端口集合。
// 实现方式：
//  1. 递归收集当前进程及所有子进程的 PID（通过 /proc/*/stat 的 PPID 链）
//  2. 读取这些 PID 的 /proc/<pid>/fd/* 找出所有 socket inode
//  3. 在 /proc/net/tcp 和 tcp6 中匹配 LISTEN 状态且 inode 属于上述集合的端口
//
// 这样可以正确识别 delta reload 场景下旧内核子进程仍占用的端口，避免误报端口冲突。
// 非Linux平台返回空集合（降级为原始行为，仅靠去重避免重复inbound误报）。
func (v *EdgeValidator) getSelfListeningPorts() map[int]bool {
	selfPorts := make(map[int]bool)

	// 1. 递归收集当前进程及所有子进程的 PID
	pids := collectManagedPIDs()

	// 2. 收集这些进程持有的 socket inode
	selfInodes := make(map[uint64]bool)
	for _, pid := range pids {
		fdDir := fmt.Sprintf("/proc/%d/fd", pid)
		entries, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			link, err := os.Readlink(fdDir + "/" + entry.Name())
			if err != nil {
				continue
			}
			// socket:[123456]
			if strings.HasPrefix(link, "socket:[") {
				inodeStr := strings.TrimPrefix(link, "socket:[")
				inodeStr = strings.TrimSuffix(inodeStr, "]")
				inode, err := strconv.ParseUint(inodeStr, 10, 64)
				if err == nil {
					selfInodes[inode] = true
				}
			}
		}
	}

	// 3. 解析 /proc/net/tcp 和 /proc/net/tcp6，找出LISTEN状态且inode属于自身的端口
	parseNetTCP := func(path string) {
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "sl") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 10 {
				continue
			}
			// fields[3] = st (0A=LISTEN), fields[1]=local_addr, fields[9]=inode
			if fields[3] != "0A" {
				continue
			}
			inode, err := strconv.ParseUint(fields[9], 10, 64)
			if err != nil || !selfInodes[inode] {
				continue
			}
			parts := strings.Split(fields[1], ":")
			if len(parts) != 2 {
				continue
			}
			port, err := strconv.ParseInt(parts[1], 16, 32)
			if err == nil && port > 0 && port < 65536 {
				selfPorts[int(port)] = true
			}
		}
	}

	parseNetTCP("/proc/net/tcp")
	parseNetTCP("/proc/net/tcp6")

	return selfPorts
}

// collectManagedPIDs 递归收集当前进程及所有子进程的 PID。
// 通过遍历 /proc/*/stat 的 PPID 字段构建进程树。
func collectManagedPIDs() []int {
	// 获取当前进程 PID
	selfPID := os.Getpid()
	pidSet := make(map[int]bool)
	pidSet[selfPID] = true

	// 遍历 /proc 找出所有子进程（递归）
	// 使用 BFS：不断扫描 /proc，找到 PPID 在集合中的进程，直到没有新增
	for {
		newFound := false
		entries, err := os.ReadDir("/proc")
		if err != nil {
			break
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			pid, err := strconv.Atoi(entry.Name())
			if err != nil {
				continue
			}
			if pidSet[pid] {
				continue
			}
			// 读取 /proc/<pid>/stat 获取 PPID
			// stat 格式: pid (comm) state ppid ...
			// comm 可能包含空格和括号，需要从最后一个 ')' 后面解析
			statData, err := os.ReadFile("/proc/" + entry.Name() + "/stat")
			if err != nil {
				continue
			}
			statStr := string(statData)
			lastParen := strings.LastIndex(statStr, ")")
			if lastParen < 0 {
				continue
			}
			rest := strings.Fields(statStr[lastParen+1:])
			if len(rest) < 2 {
				continue
			}
			// rest[0]=state, rest[1]=ppid
			ppid, err := strconv.Atoi(rest[1])
			if err != nil {
				continue
			}
			if pidSet[ppid] {
				pidSet[pid] = true
				newFound = true
			}
		}
		if !newFound {
			break
		}
	}

	pids := make([]int, 0, len(pidSet))
	for pid := range pidSet {
		pids = append(pids, pid)
	}
	return pids
}
