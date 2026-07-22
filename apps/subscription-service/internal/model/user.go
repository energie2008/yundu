package model

import (
	"time"

	"github.com/google/uuid"
)

type UserStatus string

const (
	UserStatusActive   UserStatus = "active"
	UserStatusBanned   UserStatus = "banned"
	UserStatusDisabled UserStatus = "disabled"
	UserStatusExpired  UserStatus = "expired"
)

type User struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	Email          string     `json:"email,omitempty" db:"email"`
	Username       string     `json:"username,omitempty" db:"username"`
	Status         UserStatus `json:"status" db:"status"`
	// UUID 是用户代理协议凭证（对齐 XBoard），全节点共享
	UUID           string     `json:"uuid" db:"uuid"`
	TrafficUsed    int64      `json:"traffic_used" db:"traffic_used_bytes"`
	TrafficTotal   int64      `json:"traffic_total" db:"traffic_total_bytes"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty" db:"expires_at"`
	// GroupID 会员分组ID，用于按分组过滤可见节点（nil=可见全部节点）
	GroupID        *uuid.UUID `json:"group_id,omitempty" db:"group_id"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}
