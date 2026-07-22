package model

import (
	"time"

	"github.com/google/uuid"
)

type AdminStatus string

const (
	AdminStatusActive  AdminStatus = "active"
	AdminStatusDisabled AdminStatus = "disabled"
)

type Admin struct {
	ID           uuid.UUID   `json:"id" db:"id"`
	UserID       uuid.UUID   `json:"user_id" db:"user_id"`
	DisplayName  string      `json:"display_name" db:"display_name"`
	Status       AdminStatus `json:"status" db:"status"`
	IsSuperAdmin bool        `json:"is_super_admin" db:"is_super_admin"`
	LastLoginAt  *time.Time  `json:"last_login_at,omitempty" db:"last_login_at"`
	LastLoginIP  *string     `json:"last_login_ip,omitempty" db:"last_login_ip"`
	CreatedAt    time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at" db:"updated_at"`
	DeletedAt    *time.Time  `json:"deleted_at,omitempty" db:"deleted_at"`
}

type AdminRole struct {
	AdminID   uuid.UUID `json:"admin_id" db:"admin_id"`
	RoleID    uuid.UUID `json:"role_id" db:"role_id"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}
