package model

import (
	"time"

	"github.com/google/uuid"
)

type ServerStatus string

const (
	ServerStatusProvisioning ServerStatus = "provisioning"
	ServerStatusActive       ServerStatus = "active"
	ServerStatusMaintenance  ServerStatus = "maintenance"
	ServerStatusOffline      ServerStatus = "offline"
	ServerStatusRetired      ServerStatus = "retired"
)

type ServerRole string

const (
	ServerRoleNode      ServerRole = "node"
	ServerRoleEdge      ServerRole = "edge"
	ServerRoleRelay     ServerRole = "relay"
	ServerRoleBalancer  ServerRole = "balancer"
)

type RuntimeStatus string

const (
	RuntimeStatusInactive RuntimeStatus = "inactive"
	RuntimeStatusActive   RuntimeStatus = "active"
	RuntimeStatusError    RuntimeStatus = "error"
)

type RuntimeProviderType string

const (
	RuntimeProviderNodeAgent RuntimeProviderType = "node-agent"
	RuntimeProviderThreeXUI  RuntimeProviderType = "3x-ui"
	RuntimeProviderCustom    RuntimeProviderType = "custom"
)

type Region struct {
	ID          uuid.UUID  `json:"id"`
	Code        string     `json:"code"`
	Name        string     `json:"name"`
	CountryCode *string    `json:"country_code,omitempty"`
	CityCode    *string    `json:"city_code,omitempty"`
	ISPCode     *string    `json:"isp_code,omitempty"`
	Tags        []string   `json:"tags"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type Server struct {
	ID              uuid.UUID     `json:"id"`
	Code            string        `json:"code"`
	Name            string        `json:"name"`
	RegionID        *uuid.UUID    `json:"region_id,omitempty"`
	Provider        *string       `json:"provider,omitempty"`
	Host            string        `json:"host"`
	IPv4            *string       `json:"ipv4,omitempty"`
	IPv6            *string       `json:"ipv6,omitempty"`
	SSHPort         *int          `json:"ssh_port,omitempty"`
	OSName          *string       `json:"os_name,omitempty"`
	OSVersion       *string       `json:"os_version,omitempty"`
	Arch            *string       `json:"arch,omitempty"`
	Status          ServerStatus  `json:"status"`
	Role            ServerRole    `json:"role"`
	Labels          map[string]string `json:"labels"`
	Metadata        map[string]interface{} `json:"metadata"`
	LastHeartbeatAt *time.Time    `json:"last_heartbeat_at,omitempty"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
	DeletedAt       *time.Time    `json:"deleted_at,omitempty"`
}

type Runtime struct {
	ID                  uuid.UUID            `json:"id"`
	ServerID            uuid.UUID            `json:"server_id"`
	RuntimeType         string               `json:"runtime_type"`
	RuntimeVersion      *string              `json:"runtime_version,omitempty"`
	ProviderType        RuntimeProviderType  `json:"provider_type"`
	ProviderRef         *string              `json:"provider_ref,omitempty"`
	ListenHost          *string              `json:"listen_host,omitempty"`
	APIPort             *int                 `json:"api_port,omitempty"`
	Status              RuntimeStatus        `json:"status"`
	Capabilities        map[string]interface{} `json:"capabilities"`
	ConfigSchemaVersion string               `json:"config_schema_version"`
	Metadata            map[string]interface{} `json:"metadata"`
	LastHeartbeatAt     *time.Time           `json:"last_heartbeat_at,omitempty"`
	CreatedAt           time.Time            `json:"created_at"`
	UpdatedAt           time.Time            `json:"updated_at"`
}

type NodeGroup struct {
	ID          uuid.UUID `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	Visibility  string    `json:"visibility"`
	SortOrder   int       `json:"sort_order"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
