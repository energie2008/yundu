package model

import (
	"time"

	"github.com/google/uuid"
)

type NodeInfo struct {
	ID            uuid.UUID              `json:"id"`
	Code          string                 `json:"code"`
	Name          string                 `json:"name"`
	ProtocolType  string                 `json:"protocol_type"`
	TransportType string                 `json:"transport_type"`
	SecurityType  string                 `json:"security_type"`
	Address       string                 `json:"address"`
	Port          int                    `json:"port"`
	SNI           string                 `json:"sni"`
	ALPN          []string               `json:"alpn"`
	Path          string                 `json:"path"`
	HostHeader    string                 `json:"host_header"`
	Flow          string                 `json:"flow"`
	ConfigJSON    map[string]interface{} `json:"config_json"`
	Region        string                 `json:"region"`
	CountryCode   string                 `json:"country_code"`
	GroupName     string                 `json:"group_name"`
	GroupID       uuid.UUID              `json:"group_id"`
	Score         float64                `json:"score"`
	LatencyMs     int                    `json:"latency_ms"`
	IsEnabled     bool                   `json:"is_enabled"`
	IsVisible     bool                   `json:"is_visible"`
	Status        string                 `json:"status"`
	Multiplier    float64                `json:"multiplier"`
	Priority      int                    `json:"priority"`
	HealthScore   int                    `json:"health_score"`
	// CreatedAt 节点创建时间，用于 SS2022 密钥派生（serverKey = base64(substr(md5(created_at), N))）
	CreatedAt     time.Time              `json:"created_at"`
}

type RenderContext struct {
	Upload   int64
	Download int64
	Total    int64
	Expire   int64
	Token    string
}
