package channelhealth

import (
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// 领域模型
// ============================================================================

// ChannelHealthSnapshot 对应 channel_health_snapshots 表（每次心跳一条）
type ChannelHealthSnapshot struct {
	ID            int64     `json:"id"`
	ServerID      uuid.UUID `json:"server_id"`
	RuntimeID     *uuid.UUID `json:"runtime_id,omitempty"`
	ActiveChannel string    `json:"active_channel"`     // grpc / ws / http
	ChannelState  string    `json:"channel_state"`      // healthy / degraded / unhealthy / unknown
	RTTMs         *int      `json:"rtt_ms,omitempty"`
	FailCount1h  int       `json:"fail_count_1h"`
	OnlineUsers  int       `json:"online_users"`
	LastError    *string   `json:"last_error,omitempty"`
	ReportedAt   time.Time `json:"reported_at"`
}

// ChannelHealthCurrent 对应 channel_health_current 表（每服务器一条）
type ChannelHealthCurrent struct {
	ServerID           uuid.UUID  `json:"server_id"`
	RuntimeID          *uuid.UUID `json:"runtime_id,omitempty"`
	ActiveChannel      string     `json:"active_channel"`
	ChannelState       string     `json:"channel_state"`
	RTTMs              *int       `json:"rtt_ms,omitempty"`
	FailCount1h        int        `json:"fail_count_1h"`
	OnlineUsers        int        `json:"online_users"`
	FailoverCount1h    int        `json:"failover_count_1h"`
	FailoverCount24h   int        `json:"failover_count_24h"`
	LastError          *string    `json:"last_error,omitempty"`
	LastFailoverAt     *time.Time `json:"last_failover_at,omitempty"`
	LastFailoverFrom   *string    `json:"last_failover_from,omitempty"`
	LastFailoverTo     *string    `json:"last_failover_to,omitempty"`
	LastFailoverReason *string    `json:"last_failover_reason,omitempty"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// FailoverEvent 对应 channel_failover_events 表
type FailoverEvent struct {
	ID         int64     `json:"id"`
	ServerID   uuid.UUID `json:"server_id"`
	RuntimeID  *uuid.UUID `json:"runtime_id,omitempty"`
	FromChannel string   `json:"from_channel"`
	ToChannel   string   `json:"to_channel"`
	Reason      string   `json:"reason"`
	Detail      string   `json:"detail,omitempty"`
	OccurredAt  time.Time `json:"occurred_at"`
}

// ============================================================================
// 心跳上报 DTO（来自 AgentHeartbeatRequest.ChannelHealth）
// ============================================================================

// HeartbeatChannelHealth 是 Agent 心跳上报的通道健康数据
type HeartbeatChannelHealth struct {
	ActiveChannel string  `json:"active_channel"`     // grpc / ws / http
	ChannelState  string  `json:"channel_state"`      // healthy / degraded / unhealthy / unknown
	RTTMs         *int    `json:"rtt_ms,omitempty"`
	FailCount1h   int     `json:"fail_count_1h,omitempty"`
	OnlineUsers   int     `json:"online_users,omitempty"`
	LastError     *string `json:"last_error,omitempty"`
	// 当发生降级时填充（非空表示本次心跳伴随一次 failover）
	Failover *FailoverDetail `json:"failover,omitempty"`
}

// FailoverDetail 心跳中携带的降级事件
type FailoverDetail struct {
	FromChannel string `json:"from_channel"`
	ToChannel   string `json:"to_channel"`
	Reason      string `json:"reason"`            // heartbeat_timeout / connection_error / auto_recovery / initial_connect
}

// ============================================================================
// 查询/响应 DTO
// ============================================================================

type ChannelHealthListQuery struct {
	Page          int       `form:"page"`
	PageSize      int       `form:"page_size"`
	ServerID      *uuid.UUID `form:"server_id,omitempty"`
	ChannelState  string    `form:"channel_state,omitempty"`
}

type ChannelHealthListItem struct {
	ServerID         uuid.UUID  `json:"server_id"`
	ServerCode       string     `json:"server_code"`
	ServerName       string     `json:"server_name"`
	ActiveChannel    string     `json:"active_channel"`
	ChannelState     string     `json:"channel_state"`
	RTTMs            *int       `json:"rtt_ms,omitempty"`
	FailCount1h      int        `json:"fail_count_1h"`
	OnlineUsers      int        `json:"online_users"`
	FailoverCount1h  int        `json:"failover_count_1h"`
	FailoverCount24h int        `json:"failover_count_24h"`
	LastFailoverAt   *time.Time `json:"last_failover_at,omitempty"`
	LastFailoverFrom *string    `json:"last_failover_from,omitempty"`
	LastFailoverTo   *string    `json:"last_failover_to,omitempty"`
	LastFailoverReason *string  `json:"last_failover_reason,omitempty"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type FailoverEventListQuery struct {
	Page     int        `form:"page"`
	PageSize int        `form:"page_size"`
	ServerID *uuid.UUID `form:"server_id,omitempty"`
	Reason   string     `form:"reason,omitempty"`
	StartAt  *time.Time `form:"start_at,omitempty"`
	EndAt    *time.Time `form:"end_at,omitempty"`
}

// ManualSwitchRequest 手动切换通道（管理员排障用）
type ManualSwitchRequest struct {
	ServerID      uuid.UUID `json:"server_id" binding:"required"`
	TargetChannel string    `json:"target_channel" binding:"required,oneof=grpc ws http"`
	Reason        string    `json:"reason,omitempty"`
}

type ManualSwitchResponse struct {
	ServerID      uuid.UUID `json:"server_id"`
	TargetChannel string    `json:"target_channel"`
	Status        string    `json:"status"` // queued / unsupported / failed
	Message       string    `json:"message,omitempty"`
}
