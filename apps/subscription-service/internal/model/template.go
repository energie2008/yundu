package model

import (
	"time"

	"github.com/google/uuid"
)

type ClientType string

const (
	ClientTypeClash         ClientType = "clash"
	ClientTypeClashMeta     ClientType = "clash-meta"
	ClientTypeClashForAndroid ClientType = "clash-for-android"
	ClientTypeClashVerge    ClientType = "clash-verge"
	ClientTypeClashX        ClientType = "clashx"
	ClientTypeClashXPro     ClientType = "clashx-pro"
	ClientTypeCFW           ClientType = "clash-for-windows"
	ClientTypeMihomo        ClientType = "mihomo"
	ClientTypeMihomoParty   ClientType = "mihomo-party"
	ClientTypeVergeRev      ClientType = "verge-rev"
	ClientTypeNyanpasu      ClientType = "clash-nyanpasu"
	ClientTypeSingBox       ClientType = "sing-box"
	ClientTypeSFA           ClientType = "sing-box-for-android"
	ClientTypeSFI           ClientType = "sing-box-for-apple"
	ClientTypeSFM           ClientType = "sing-box-for-macos"
	ClientTypeURI           ClientType = "uri"
	ClientTypeSurge         ClientType = "surge"
	ClientTypeSurgeMac      ClientType = "surge-mac"
	ClientTypeSurgeiOS      ClientType = "surge-ios"
	ClientTypeQuantumult    ClientType = "quanx"
	ClientTypeQuantumultX   ClientType = "quantumult-x"
	ClientTypeShadowrocket  ClientType = "shadowrocket"
	ClientTypeV2RayN        ClientType = "v2rayn"
	ClientTypeV2RayNG       ClientType = "v2rayng"
	ClientTypeNekoBox       ClientType = "nekobox"
	ClientTypeNekoRay       ClientType = "nekoray"
	ClientTypeV2Box         ClientType = "v2box"
	ClientTypeStash         ClientType = "stash"
	ClientTypeLoon          ClientType = "loon"
	ClientTypeLoonLite      ClientType = "loon-lite"
	ClientTypeSurfboard     ClientType = "surfboard"
	ClientTypeHiddify       ClientType = "hiddify"
	ClientTypeHiddifyNext   ClientType = "hiddify-next"
	ClientTypeStreisand     ClientType = "streisand"
	ClientTypeMomo          ClientType = "momo"
	ClientTypeKaring        ClientType = "karing"
	ClientTypeFlClash       ClientType = "flclash"
	ClientTypeBox           ClientType = "box"
	ClientTypeXBrowser      ClientType = "xbrowser"
	ClientTypeSubStore      ClientType = "sub-store"
)

type TemplateStatus string

const (
	TemplateStatusActive   TemplateStatus = "active"
	TemplateStatusInactive TemplateStatus = "inactive"
)

type SubscriptionTemplate struct {
	ID             uuid.UUID      `json:"id" db:"id"`
	Code           string         `json:"code" db:"code"`
	Name           string         `json:"name" db:"name"`
	TargetClient   ClientType     `json:"target_client" db:"target_client"`
	TemplateType   string         `json:"template_type" db:"template_type"`
	Content        string         `json:"content" db:"content"`
	Status         TemplateStatus `json:"status" db:"status"`
	IsDefault      bool           `json:"is_default" db:"is_default"`
	SchemaVersion  string         `json:"schema_version" db:"schema_version"`
	CreatedByAdmin *uuid.UUID     `json:"created_by_admin_id,omitempty" db:"created_by_admin_id"`
	CreatedAt      time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at" db:"updated_at"`
}
