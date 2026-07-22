package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type SystemSetting struct {
	ID               uuid.UUID       `json:"id" db:"id"`
	SettingGroup     string          `json:"setting_group" db:"setting_group"`
	SettingKey       string          `json:"setting_key" db:"setting_key"`
	ValueJSON        json.RawMessage `json:"value_json" db:"value_json"`
	IsSecret         bool            `json:"is_secret" db:"is_secret"`
	Description      *string         `json:"description,omitempty" db:"description"`
	UpdatedByAdminID *uuid.UUID      `json:"updated_by_admin_id,omitempty" db:"updated_by_admin_id"`
	UpdatedAt        time.Time       `json:"updated_at" db:"updated_at"`
}
