package model

import (
	"time"

	"github.com/google/uuid"
)

type ChainStatus string

const (
	ChainStatusActive   ChainStatus = "active"
	ChainStatusInactive ChainStatus = "inactive"
	ChainStatusError    ChainStatus = "error"
)

type ChainMode string

const (
	ChainModeSingle ChainMode = "single"
	ChainModeMulti  ChainMode = "multi"
	ChainModeBackup ChainMode = "backup"
)

type ChainStrategy string

const (
	ChainStrategyOrdered  ChainStrategy = "ordered"
	ChainStrategyRandom   ChainStrategy = "random"
	ChainStrategyLeastLoad ChainStrategy = "least_load"
)

type HopType string

const (
	HopTypeNode    HopType = "node"
	HopTypeRelay   HopType = "relay"
	HopTypeOutbound HopType = "outbound"
)

type BindMode string

const (
	BindModeDefault BindMode = "default"
	BindModeBackup  BindMode = "backup"
	BindModeOnly    BindMode = "only"
)

type ProxyChain struct {
	ID             uuid.UUID              `json:"id"`
	Code           string                 `json:"code"`
	Name           string                 `json:"name"`
	Status         ChainStatus            `json:"status"`
	ChainMode      ChainMode              `json:"chain_mode"`
	Strategy       ChainStrategy          `json:"strategy"`
	MaxHops        int                    `json:"max_hops"`
	HealthPolicyID *uuid.UUID             `json:"health_policy_id,omitempty"`
	Metadata       map[string]interface{} `json:"metadata"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

type ProxyChainHop struct {
	ID                 uuid.UUID              `json:"id"`
	ChainID            uuid.UUID              `json:"chain_id"`
	HopIndex           int                    `json:"hop_index"`
	HopType            HopType                `json:"hop_type"`
	UpstreamNodeID     *uuid.UUID             `json:"upstream_node_id,omitempty"`
	UpstreamRuntimeID  *uuid.UUID             `json:"upstream_runtime_id,omitempty"`
	OutboundProtocolType *string              `json:"outbound_protocol_type,omitempty"`
	OutboundConfigJSON map[string]interface{} `json:"outbound_config_json"`
	CreatedAt          time.Time              `json:"created_at"`
}

type NodeChainBinding struct {
	NodeID    uuid.UUID `json:"node_id"`
	ChainID   uuid.UUID `json:"chain_id"`
	BindMode  BindMode  `json:"bind_mode"`
	Priority  int       `json:"priority"`
	CreatedAt time.Time `json:"created_at"`
}
