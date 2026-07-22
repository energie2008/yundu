package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type UserStatus string

const (
	UserStatusPending  UserStatus = "pending"
	UserStatusActive   UserStatus = "active"
	UserStatusDisabled UserStatus = "disabled"
	UserStatusBanned   UserStatus = "banned"
	UserStatusExpired  UserStatus = "expired"
)

type UserSubscription struct {
	ID               uuid.UUID  `json:"id"`
	UserID           uuid.UUID  `json:"user_id"`
	PlanID           uuid.UUID  `json:"plan_id"`
	PlanName         string     `json:"plan_name"`
	Status           string     `json:"status"`
	StartedAt        time.Time  `json:"started_at"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	TrafficQuotaBytes int64     `json:"traffic_quota_bytes"`
	TrafficUsedBytes  int64     `json:"traffic_used_bytes"`
	UploadBytes      int64      `json:"upload_bytes"`
	DownloadBytes    int64      `json:"download_bytes"`
	SpeedLimitMbps   int        `json:"speed_limit_mbps"`
	DeviceLimit      int        `json:"device_limit"`
	ResetAt          *time.Time `json:"reset_at,omitempty"`
}

type PaymentOrder struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	OrderNo        string     `json:"order_no" db:"order_no"`
	UserID         uuid.UUID  `json:"user_id" db:"user_id"`
	PlanID         uuid.UUID  `json:"plan_id" db:"plan_id"`
	PlanName       string     `json:"plan_name,omitempty" db:"plan_name"`
	PeriodCode     string     `json:"period_code" db:"period_code"`
	AmountUSDT     float64    `json:"amount_usdt" db:"amount_usdt"`
	AmountCNY      float64    `json:"amount_cny" db:"amount_cny"`
	ExchangeRate   float64    `json:"exchange_rate" db:"exchange_rate"`
	DiscountAmount float64    `json:"discount_amount" db:"discount_amount"`
	FinalAmount    float64    `json:"final_amount" db:"final_amount"`
	CouponCode     string     `json:"coupon_code,omitempty" db:"coupon_code"`
	PayAddress     string     `json:"pay_address" db:"pay_address"`
	PayCurrency    string     `json:"pay_currency" db:"pay_currency"`
	PaymentMethod  string     `json:"payment_method" db:"payment_method"`
	PaymentURI     string     `json:"payment_uri,omitempty" db:"-"`
	Status         string     `json:"status" db:"status"`
	TxHash         *string    `json:"tx_hash,omitempty" db:"tx_hash"`
	PaidAmount     *float64   `json:"paid_amount,omitempty" db:"paid_amount"`
	PaidAt         *time.Time `json:"paid_at,omitempty" db:"paid_at"`
	BlockNumber    *int64     `json:"block_number,omitempty" db:"block_number"`
	ExpiresAt      time.Time  `json:"expires_at" db:"expires_at"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}

type User struct {
	ID               uuid.UUID  `json:"id" db:"id"`
	Email            string     `json:"email" db:"email"`
	Username         *string    `json:"username,omitempty" db:"username"`
	PasswordHash     *string    `json:"-" db:"password_hash"`
	PasswordAlgo     string     `json:"-" db:"password_algo"`
	Status           UserStatus `json:"status" db:"status"`
	// UUID 是用户代理协议凭证（对齐 XBoard 模型）：每用户唯一，全节点共享
	// VLESS/VMess/TUIC 直接使用；Trojan/SS/Hysteria2/AnyTLS 直接使用；
	// SS2022 通过 serverKey:userKey 派生（派生算法在订阅渲染层实现）
	UUID             string     `json:"uuid" db:"uuid"`
	EmailVerifiedAt  *time.Time `json:"email_verified_at,omitempty" db:"email_verified_at"`
	TelegramChatID   *string    `json:"telegram_chat_id,omitempty" db:"telegram_chat_id"`
	InviterID        *uuid.UUID `json:"inviter_id,omitempty" db:"inviter_id"`
	CommissionBalance float64   `json:"commission_balance" db:"commission_balance"`
	CommissionTotal  float64    `json:"commission_total" db:"commission_total"`
	NotifyExpiry     bool       `db:"notify_expiry" json:"-"`
	NotifyTraffic    bool       `db:"notify_traffic" json:"-"`
	NotifyTicketReply bool      `db:"notify_ticket_reply" json:"-"`
	RegisteredAt     *time.Time `json:"registered_at,omitempty" db:"registered_at"`
	Locale           string     `json:"locale" db:"locale"`
	Timezone         string     `json:"timezone" db:"timezone"`
	LastLoginAt      *time.Time `json:"last_login_at,omitempty" db:"last_login_at"`
	LastLoginIP      *string    `json:"last_login_ip,omitempty" db:"last_login_ip"`
	LastSeenAt       *time.Time `json:"last_seen_at,omitempty" db:"last_seen_at"`
	Notes            *string    `json:"notes,omitempty" db:"notes"`
	// GroupID 关联 node_groups 表，决定用户可见的节点分组（购买套餐时自动赋值 plan.group_id）
	GroupID          *uuid.UUID `json:"group_id,omitempty" db:"group_id"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt        *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
}

type UserProfile struct {
	UserID       uuid.UUID       `json:"user_id" db:"user_id"`
	AvatarURL    *string         `json:"avatar_url,omitempty" db:"avatar_url"`
	ContactEmail *string         `json:"contact_email,omitempty" db:"contact_email"`
	Phone        *string         `json:"phone,omitempty" db:"phone"`
	CountryCode  *string         `json:"country_code,omitempty" db:"country_code"`
	Tags         []string        `json:"tags" db:"tags"`
	Metadata     json.RawMessage `json:"metadata" db:"metadata"`
	UpdatedAt    time.Time       `json:"updated_at" db:"updated_at"`
}

type CredentialType string

const (
	CredentialTypeAPIKey CredentialType = "api_key"
	CredentialTypeUUID   CredentialType = "uuid"
)

type CredentialStatus string

const (
	CredentialStatusActive  CredentialStatus = "active"
	CredentialStatusRevoked CredentialStatus = "revoked"
)

type UserCredential struct {
	ID             uuid.UUID        `json:"id" db:"id"`
	UserID         uuid.UUID        `json:"user_id" db:"user_id"`
	CredentialType CredentialType   `json:"credential_type" db:"credential_type"`
	UUIDValue      *uuid.UUID       `json:"uuid_value,omitempty" db:"uuid_value"`
	TokenValue     *string          `json:"-" db:"token_value"`
	Label          *string          `json:"label,omitempty" db:"label"`
	Status         CredentialStatus `json:"status" db:"status"`
	RotatedFromID  *uuid.UUID       `json:"rotated_from_id,omitempty" db:"rotated_from_id"`
	ExpiresAt      *time.Time       `json:"expires_at,omitempty" db:"expires_at"`
	LastUsedAt     *time.Time       `json:"last_used_at,omitempty" db:"last_used_at"`
	CreatedAt      time.Time        `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at" db:"updated_at"`
}
