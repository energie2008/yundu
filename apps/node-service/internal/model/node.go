package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type NodeType string

const (
	NodeTypeStandard   NodeType = "standard"
	NodeTypePremium    NodeType = "premium"
	NodeTypeReserved   NodeType = "reserved"
	NodeTypeBackup     NodeType = "backup"
)

type NodeStatus string

const (
	NodeStatusUnknown   NodeStatus = "unknown"
	NodeStatusHealthy   NodeStatus = "healthy"
	NodeStatusDegraded  NodeStatus = "degraded"
	NodeStatusOffline   NodeStatus = "offline"
	NodeStatusDisabled  NodeStatus = "disabled"
)

type HealthSeverity string

const (
	HealthSeverityInfo     HealthSeverity = "info"
	HealthSeverityWarning  HealthSeverity = "warning"
	HealthSeverityCritical HealthSeverity = "critical"
)

type Node struct {
	ID                     uuid.UUID              `json:"id"`
	Code                   string                 `json:"code"`
	Name                   string                 `json:"name"`
	RuntimeID              uuid.UUID              `json:"runtime_id"`
	RegionID               *uuid.UUID             `json:"region_id,omitempty"`
	GroupID                *uuid.UUID             `json:"group_id,omitempty"`
	NodeType               NodeType               `json:"node_type"`
	ProtocolType           string                 `json:"protocol_type"`
	TransportType          string                 `json:"transport_type"`
	SecurityType           *string                `json:"security_type,omitempty"`
	Address                string                 `json:"address"`
	Port                   int                    `json:"port"`
	ServerPort             *int                   `json:"server_port,omitempty"`
	RealityServerName      *string                `json:"reality_server_name,omitempty"`
	SNI                    *string                `json:"sni,omitempty"`
	ALPN                   []string               `json:"alpn"`
	Path                   *string                `json:"path,omitempty"`
	HostHeader             *string                `json:"host_header,omitempty"`
	Flow                   *string                `json:"flow,omitempty"`
	IsEnabled              bool                   `json:"is_enabled"`
	IsVisible              bool                   `json:"is_visible"`
	AllowUDP               bool                   `json:"allow_udp"`
	SpeedLimitMbps         *int                   `json:"speed_limit_mbps,omitempty"`
	DeviceLimit            *int                   `json:"device_limit,omitempty"`
	PaddingScheme          *string                `json:"padding_scheme,omitempty"`
	RateTimeEnable         *bool                  `json:"rate_time_enable,omitempty"`
	RateTimeRanges         json.RawMessage        `json:"rate_time_ranges,omitempty"`
	TransferEnableBytes    *int64                 `json:"transfer_enable_bytes,omitempty"`
	TrafficRate            float64                `json:"traffic_rate"`
	Priority               int                    `json:"priority"`
	CapacityScore          int                    `json:"capacity_score"`
	ProtocolSchemaVersion  string                 `json:"protocol_schema_version"`
	ExposureMode           *string                `json:"exposure_mode,omitempty"`
	DownstreamExposureMode *string                `json:"downstream_exposure_mode,omitempty"`
	IsSplitMode            bool                   `json:"is_split_mode"`
	ConfigJSON             map[string]interface{} `json:"config_json"`
	Tags                   []string               `json:"tags"`
	Metadata               map[string]interface{} `json:"metadata"`
	LastPublishedVersion   int64                  `json:"last_published_version"`
	LastSeenAt            *time.Time             `json:"last_seen_at,omitempty"`
	CreatedAt              time.Time              `json:"created_at"`
	UpdatedAt              time.Time              `json:"updated_at"`
	DeletedAt              *time.Time             `json:"deleted_at,omitempty"`
}

type HealthProfile struct {
	ID                    uuid.UUID              `json:"id"`
	Code                  string                 `json:"code"`
	Name                  string                 `json:"name"`
	ProbeIntervalSeconds  int                    `json:"probe_interval_seconds"`
	TimeoutSeconds        int                    `json:"timeout_seconds"`
	FailureThreshold      int                    `json:"failure_threshold"`
	RecoveryThreshold     int                    `json:"recovery_threshold"`
	ProbeTargets          []interface{}          `json:"probe_targets"`
	Metadata              map[string]interface{} `json:"metadata"`
	CreatedAt             time.Time              `json:"created_at"`
	UpdatedAt             time.Time              `json:"updated_at"`
}

type NodeHealthStatus struct {
	NodeID             uuid.UUID   `json:"node_id"`
	OverallStatus      string      `json:"overall_status"`
	HeartbeatStatus    string      `json:"heartbeat_status"`
	ProbeStatus        string      `json:"probe_status"`
	AvailabilityScore  int         `json:"availability_score"`
	LatencyScore       int         `json:"latency_score"`
	LossScore          int         `json:"loss_score"`
	HandshakeScore     int         `json:"handshake_score"`
	ChainScore         int         `json:"chain_score"`
	StabilityScore     int         `json:"stability_score"`
	CurrentRTTMs       *int        `json:"current_rtt_ms,omitempty"`
	CurrentLossRatio   *float64    `json:"current_loss_ratio,omitempty"`
	CurrentOnlineUsers int         `json:"current_online_users"`
	CurrentCPUPercent  *float64    `json:"current_cpu_percent,omitempty"`
	CurrentMemPercent  *float64    `json:"current_mem_percent,omitempty"`
	CurrentDiskPercent *float64    `json:"current_disk_percent,omitempty"`
	LastHeartbeatAt    *time.Time  `json:"last_heartbeat_at,omitempty"`
	LastProbeAt        *time.Time  `json:"last_probe_at,omitempty"`
	LastStateChangedAt *time.Time  `json:"last_state_changed_at,omitempty"`
	LastErrorCode      *string     `json:"last_error_code,omitempty"`
	LastErrorMessage   *string     `json:"last_error_message,omitempty"`
	UpdatedAt          time.Time   `json:"updated_at"`
}

type NodeHealthEvent struct {
	ID         uuid.UUID              `json:"id"`
	NodeID     uuid.UUID              `json:"node_id"`
	EventType  string                 `json:"event_type"`
	Severity   HealthSeverity         `json:"severity"`
	FromStatus *string                `json:"from_status,omitempty"`
	ToStatus   *string                `json:"to_status,omitempty"`
	Metrics    map[string]interface{} `json:"metrics"`
	Message    *string                `json:"message,omitempty"`
	OccurredAt time.Time              `json:"occurred_at"`
}
