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
