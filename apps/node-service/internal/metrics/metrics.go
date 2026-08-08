// Package metrics 定义 node-service 的业务领域 Prometheus 指标。
// 通用 HTTP 指标（http_requests_total 等）由共享包 packages/config/observability 提供，
// 此处补充 node-service 独有的领域指标：gRPC 连接、doctor 检查、通道健康、AI 诊断、边缘暴露。
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// GRPCAgentConnections 当前活跃的 gRPC agent 连接数（Gauge）
	GRPCAgentConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nodeservice_grpc_agent_connections",
		Help: "Current number of active gRPC agent connections.",
	})

	// GRPCMessagesReceived 从 agent 收到的消息总数（按消息类型分类）
	GRPCMessagesReceived = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nodeservice_grpc_messages_received_total",
		Help: "Total gRPC messages received from agents, by message type.",
	}, []string{"message_type"})

	// GRPCMessagesPushed 推送到 agent 的消息总数（按消息类型分类）
	GRPCMessagesPushed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nodeservice_grpc_messages_pushed_total",
		Help: "Total gRPC messages pushed to agents, by message type.",
	}, []string{"message_type"})

	// DoctorChecksTotal doctor 体检执行次数（按结果分类）
	DoctorChecksTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nodeservice_doctor_checks_total",
		Help: "Total doctor checks executed, by result status (pass/warn/fail/error).",
	}, []string{"result"})

	// DoctorAutofixDispatched doctor autofix 派发次数（按动作和结果分类）
	DoctorAutofixDispatched = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nodeservice_doctor_autofix_dispatched_total",
		Help: "Total doctor autofix actions dispatched, by action type and dispatch status.",
	}, []string{"action", "status"})

	// ChannelHealthState 通道健康状态（按服务器维度，1=healthy, 0=degraded, -1=unhealthy）
	ChannelHealthState = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nodeservice_channel_health_state",
		Help: "Channel health state per server (1=healthy, 0=degraded, -1=unhealthy).",
	}, []string{"server_id", "active_channel", "state"})

	// DiagnosisSessionsTotal AI 诊断会话创建次数（按类别分类）
	DiagnosisSessionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nodeservice_diagnosis_sessions_total",
		Help: "Total AI diagnosis sessions created, by category.",
	}, []string{"category"})

	// DiagnosisAutofixTotal AI 诊断 autofix 执行次数（按动作和结果分类）
	DiagnosisAutofixTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nodeservice_diagnosis_autofix_total",
		Help: "Total AI diagnosis autofix actions, by action type and status.",
	}, []string{"action", "status"})

	// ExposureAppliesTotal 边缘暴露配置 apply 次数（按结果状态分类）
	ExposureAppliesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nodeservice_exposure_applies_total",
		Help: "Total exposure config applies, by final status (applied/failed).",
	}, []string{"status"})

	// ConfigRenderHashChurnTotal 渲染 hash 漂移计数：相同输入渲染产生不同 hash 时 +1。
	// 用于版本号死循环复发检测（排序契约破坏时触发告警）。
	ConfigRenderHashChurnTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nodeservice_config_render_hash_churn_total",
		Help: "Total config render hash churn events (input unchanged but hash changed).",
	}, []string{"runtime_id"})

	// DualKernelStandaloneXraySkippedTotal 双内核架构下被拦截的辅内核 xray 独立推送次数。
	// 事故复发检测：稳定态应为 0，>0 说明仍有路径试图把独立 xray 配置推给 sing-box agent
	// （曾致 VPS206 "unknown field api" 校验失败事故）。
	DualKernelStandaloneXraySkippedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nodeservice_dual_kernel_xray_standalone_skipped_total",
		Help: "Standalone xray config pushes skipped for auxiliary xray runtime on dual-kernel servers (steady state 0).",
	}, []string{"server_code", "reason"})

	// DualKernelCrossRuntimePayloadRejectedTotal 跨 runtime 拉取 payload 被拒绝次数
	// （FetchPayload 按 X-Runtime-Ref 做版本归属校验，防辅内核 xray 配置泄漏给 sing-box agent）。
	DualKernelCrossRuntimePayloadRejectedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nodeservice_dual_kernel_cross_runtime_payload_rejected_total",
		Help: "Payload fetch rejected due to version not owned by requesting agent runtime (cross-runtime leak guard).",
	}, []string{"server_code"})

	// DualKernelXrayArchivedTotal 辅内核 xray 独立配置版本仅存档（不推送）次数。
	DualKernelXrayArchivedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nodeservice_dual_kernel_xray_archived_total",
		Help: "Auxiliary xray runtime config versions archived without standalone push (delivered embedded in sing-box config).",
	}, []string{"runtime_id"})
)

// ChannelStateValue 将通道状态字符串映射为数值，用于 Gauge 指标
func ChannelStateValue(state string) float64 {
	switch state {
	case "healthy":
		return 1
	case "degraded":
		return 0
	case "unhealthy":
		return -1
	default:
		return -1
	}
}
