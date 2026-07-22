package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/airport-panel/node-service/internal/model"
	"github.com/airport-panel/node-service/internal/repo"
)

// 阶段2.2: argo_tunnel 节点一致性巡检
//
// 防止后续有人手动改 DB 字段绕开渲染逻辑，回到"手工打补丁"的老问题。
// 定时扫描所有 exposure_mode=argo_tunnel 的节点，校验三要素：
//  1. DB security_type = "none"（服务端无 TLS，cloudflared HTTP 回源）
//  2. config_json.security_type = "tls"（客户端 TLS，CF 边缘强制）
//  3. 渲染层 listen = "127.0.0.1"（不监听公网，安全约束）
//
// 任何一项不满足就告警（slog.Error），不自动修复（避免与渲染层冲突）。
// 检查间隔：5 分钟（比配置下发周期 30 秒慢，避免误报瞬时状态）。

// ArgoTunnelConsistencyChecker argo_tunnel 节点一致性巡检器
type ArgoTunnelConsistencyChecker struct {
	nodeRepo *repo.NodeRepo
	logger   *slog.Logger
}

// NewArgoTunnelConsistencyChecker 创建一致性巡检器
func NewArgoTunnelConsistencyChecker(nodeRepo *repo.NodeRepo, logger *slog.Logger) *ArgoTunnelConsistencyChecker {
	if logger == nil {
		logger = slog.Default()
	}
	return &ArgoTunnelConsistencyChecker{
		nodeRepo: nodeRepo,
		logger:   logger,
	}
}

// Start 启动定时巡检（每 5 分钟一轮）
// 启动后立即执行一次，避免冷启动等待
func (c *ArgoTunnelConsistencyChecker) Start(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	c.runRound(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.runRound(ctx)
		}
	}
}

// runRound 执行一轮巡检
func (c *ArgoTunnelConsistencyChecker) runRound(ctx context.Context) {
	if c.nodeRepo == nil {
		c.logger.Warn("argo_tunnel consistency check skipped: nodeRepo not injected")
		return
	}
	// 拉取所有未删除节点（不分页，argo_tunnel 节点数量少）
	nodes, _, err := c.nodeRepo.List(ctx, 1, 500, "", "", "", "", nil)
	if err != nil {
		c.logger.Error("argo_tunnel consistency check: list nodes failed", "error", err)
		return
	}

	violations := 0
	for _, node := range nodes {
		if !isArgoTunnelExposureNode(node) {
			continue
		}
		if violation := c.checkNode(node); violation != "" {
			violations++
			c.logger.Error("argo_tunnel consistency violation",
				"node_id", node.ID,
				"node_code", node.Code,
				"violation", violation,
				"action", "需人工核查或重新触发配置下发",
			)
		}
	}
	if violations > 0 {
		c.logger.Error("argo_tunnel consistency check completed with violations",
			"total_argo_nodes", countArgoTunnelNodes(nodes),
			"violations", violations,
		)
	} else {
		c.logger.Info("argo_tunnel consistency check passed",
			"total_argo_nodes", countArgoTunnelNodes(nodes),
		)
	}
}

// checkNode 校验单个节点的五要素，返回违规描述（空字符串表示通过）
//
// P1-6 修复：校验器与实际实现保持一致。
// argo_tunnel 节点的 TLS 分离架构：
//   - DB security_type = "none"（服务端 xray inbound 无 TLS，cloudflared 明文 HTTP 回源）
//   - config_json.security_type = "tls"（客户端面向 CF 边缘 TLS）
//   - config_json.security = "tls" / config_json.tls = 1（客户端字段双写）
//
// 旧版检查器期望 DB security_type="tls"（方案1: noTLSVerify 回源），
// 但实际实现仍为剥离方案（security=none），两者不一致会导致每次巡检误报。
func (c *ArgoTunnelConsistencyChecker) checkNode(node *model.Node) string {
	// 1. DB security_type 必须为 "none"（服务端剥离 TLS，cloudflared 明文 HTTP 回源）
	if node.SecurityType == nil || *node.SecurityType != "none" {
		secType := "nil"
		if node.SecurityType != nil {
			secType = *node.SecurityType
		}
		return "DB security_type 应为 none（服务端剥离），实际为 " + secType
	}
	// 2. config_json.security_type 必须为 "tls"（客户端面向 CF 边缘 TLS）
	if node.ConfigJSON == nil {
		return "config_json 为空"
	}
	cjSec, _ := node.ConfigJSON["security_type"].(string)
	if cjSec != "tls" {
		return "config_json.security_type 应为 tls（客户端），实际为 " + cjSec
	}
	// P1-6: 验证 config_json.security 和 config_json.tls 双写一致性
	cjSecurity, _ := node.ConfigJSON["security"].(string)
	if cjSecurity != "tls" {
		return "config_json.security 应为 tls（客户端双写），实际为 " + cjSecurity
	}
	cjTLS, hasTLS := node.ConfigJSON["tls"]
	if !hasTLS {
		return "config_json.tls 字段缺失（客户端应为 1）"
	}
	tlsVal := 0
	switch v := cjTLS.(type) {
	case float64:
		tlsVal = int(v)
	case int:
		tlsVal = v
	case int64:
		tlsVal = int(v)
	}
	if tlsVal != 1 {
		return "config_json.tls 应为 1（客户端），实际为其它值"
	}
	// 3. config_json.exposure_mode 必须为 "argo_tunnel"
	em, _ := node.ConfigJSON["exposure_mode"].(string)
	if em != "argo_tunnel" {
		return "config_json.exposure_mode 应为 argo_tunnel，实际为 " + em
	}
	// 4. cdn_address 非空（cloudflared ingress hostname 需要）
	cdnAddr, _ := node.ConfigJSON["cdn_address"].(string)
	if cdnAddr == "" {
		return "config_json.cdn_address 为空，cloudflared ingress 无法路由"
	}
	// 5. WS/HTTPUpgrade 传输 ALPN 必须为 ["http/1.1"]
	if node.TransportType == "ws" || node.TransportType == "httpupgrade" {
		alpn := extractALPNSlice(node.ConfigJSON["alpn"])
		if !isALPNHTTP11(alpn) {
			return "WS/HTTPUpgrade 节点 ALPN 应为 [http/1.1]，实际为 " + formatALPN(alpn)
		}
	}
	return ""
}

// extractALPNSlice 从 config_json.alpn 提取 ALPN 切片
// 兼容 []string（Go 内部）和 []interface{}（JSON 反序列化）两种类型
func extractALPNSlice(v interface{}) []string {
	if v == nil {
		return nil
	}
	// 优先 []string（Go 内部直接赋值的情况）
	if s, ok := v.([]string); ok {
		return s
	}
	// JSON 反序列化为 []interface{}
	if arr, ok := v.([]interface{}); ok {
		result := make([]string, 0, len(arr))
		for _, item := range arr {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

// isALPNHTTP11 检查 ALPN 是否仅包含 http/1.1
func isALPNHTTP11(alpn []string) bool {
	if len(alpn) != 1 {
		return false
	}
	return alpn[0] == "http/1.1"
}

// formatALPN 格式化 ALPN 用于日志输出
func formatALPN(alpn []string) string {
	if len(alpn) == 0 {
		return "[]"
	}
	result := "["
	for i, v := range alpn {
		if i > 0 {
			result += ","
		}
		result += v
	}
	return result + "]"
}

// countArgoTunnelNodes 统计 argo_tunnel 节点数量
func countArgoTunnelNodes(nodes []*model.Node) int {
	count := 0
	for _, n := range nodes {
		if isArgoTunnelExposureNode(n) {
			count++
		}
	}
	return count
}
