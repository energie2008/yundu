package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8,max=128"`
	Username string `json:"username"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type AdminLoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

type UserResponse struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Username  *string   `json:"username,omitempty"`
	Status    string    `json:"status"`
	Locale    string    `json:"locale"`
	Timezone  string    `json:"timezone"`
	CreatedAt string    `json:"created_at"`
}

type AdminResponse struct {
	ID           uuid.UUID `json:"id"`
	UserID       uuid.UUID `json:"user_id"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"display_name"`
	IsSuperAdmin bool      `json:"is_super_admin"`
	Status       string    `json:"status"`
	CreatedAt    string    `json:"created_at"`
}

type CreateAdminRequest struct {
	Email        string `json:"email" binding:"required,email"`
	Password     string `json:"password" binding:"required,min=8,max=128"`
	DisplayName  string `json:"display_name" binding:"required"`
	IsSuperAdmin bool   `json:"is_super_admin"`
}

type UserListQuery struct {
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
	Status   string `form:"status"`
	Search   string `form:"search"`
}

type AdminListQuery struct {
	Page     int `form:"page"`
	PageSize int `form:"page_size"`
}

type AuditLogListQuery struct {
	Page         int    `form:"page"`
	PageSize     int    `form:"page_size"`
	ActorType    string `form:"actor_type"`
	ActorID      string `form:"actor_id"`
	ResourceType string `form:"resource_type"`
	ResourceID   string `form:"resource_id"`
	Action       string `form:"action"`
}

type SystemSettingResponse struct {
	SettingGroup string          `json:"setting_group"`
	SettingKey   string          `json:"setting_key"`
	Value        interface{}     `json:"value"`
	IsSecret     bool            `json:"is_secret"`
	Description  *string         `json:"description,omitempty"`
	UpdatedAt    string          `json:"updated_at"`
}

type UpdateSettingRequest struct {
	Value interface{} `json:"value" binding:"required"`
}

type PaginationResponse struct {
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
	Total    int         `json:"total"`
	Items    interface{} `json:"items"`
}

func NewUserResponse(u *User) UserResponse {
	var username *string
	if u.Username != nil && *u.Username != "" {
		username = u.Username
	}
	return UserResponse{
		ID:        u.ID,
		Email:     u.Email,
		Username:  username,
		Status:    string(u.Status),
		Locale:    u.Locale,
		Timezone:  u.Timezone,
		CreatedAt: u.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func NewAdminResponse(a *Admin, email string) AdminResponse {
	return AdminResponse{
		ID:           a.ID,
		UserID:       a.UserID,
		Email:        email,
		DisplayName:  a.DisplayName,
		IsSuperAdmin: a.IsSuperAdmin,
		Status:       string(a.Status),
		CreatedAt:    a.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// =====================================================
// Phase 6: 工单 / 公告 / 通知 DTO
// =====================================================

// ===== 工单 DTO =====

type CreateTicketRequest struct {
	Subject             string `json:"subject" binding:"required,min=1,max=200"`
	Description         string `json:"description"`
	Message             string `json:"message"`
	Category            string `json:"category"`
	Priority            string `json:"priority"`
	RelatedResourceType string `json:"related_resource_type"`
	RelatedResourceID   string `json:"related_resource_id"`
}

type UpdateTicketRequest struct {
	Status          *string `json:"status,omitempty"`
	Priority        *string `json:"priority,omitempty"`
	AssignedAdminID *string `json:"assigned_admin_id,omitempty"`
	Category        *string `json:"category,omitempty"`
}

type CreateTicketReplyRequest struct {
	Content    string `json:"content" binding:"required"`
	IsInternal bool   `json:"is_internal"`
}

type TicketListQuery struct {
	Page       int    `form:"page"`
	PageSize   int    `form:"page_size"`
	Status     string `form:"status"`
	Category   string `form:"category"`
	Priority   string `form:"priority"`
	UserID     string `form:"user_id"`
	Email      string `form:"email"`
	AssignedTo string `form:"assigned_to"`
	Search     string `form:"search"`
}

// TicketUserSummary 工单列表中嵌入的用户摘要（参考 XBoard ticket.user）
type TicketUserSummary struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
}

type TicketResponse struct {
	ID              uuid.UUID          `json:"id"`
	UserID          uuid.UUID          `json:"user_id"`
	Subject         string             `json:"subject"`
	Description     string             `json:"description"`
	Message         string             `json:"message"`
	Category        string             `json:"category"`
	Priority        string             `json:"priority"`
	Status          string             `json:"status"`
	AssignedAdminID *uuid.UUID         `json:"assigned_admin_id,omitempty"`
	ReplyCount      int                `json:"reply_count"`
	LastReplyAt     *time.Time         `json:"last_reply_at,omitempty"`
	ClosedAt        *time.Time         `json:"closed_at,omitempty"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
	User            *TicketUserSummary `json:"user,omitempty"`
}

type TicketReplyResponse struct {
	ID         uuid.UUID  `json:"id"`
	TicketID   uuid.UUID  `json:"ticket_id"`
	AuthorID   uuid.UUID  `json:"author_id"`
	AuthorType string     `json:"author_type"`
	UserID     uuid.UUID  `json:"user_id"`
	AdminID    *uuid.UUID `json:"admin_id,omitempty"`
	IsAdmin    bool       `json:"is_admin"`
	Content    string     `json:"content"`
	IsInternal bool       `json:"is_internal"`
	CreatedAt  time.Time  `json:"created_at"`
}

func NewTicketResponse(t *Ticket) TicketResponse {
	return TicketResponse{
		ID:              t.ID,
		UserID:          t.UserID,
		Subject:         t.Subject,
		Description:     t.Description,
		Message:         t.Description,
		Category:        string(t.Category),
		Priority:        string(t.Priority),
		Status:          string(t.Status),
		AssignedAdminID: t.AssignedAdminID,
		ReplyCount:      t.ReplyCount,
		LastReplyAt:     t.LastReplyAt,
		ClosedAt:        t.ClosedAt,
		CreatedAt:       t.CreatedAt,
		UpdatedAt:       t.UpdatedAt,
	}
}

func NewTicketReplyResponse(r *TicketReply) TicketReplyResponse {
	resp := TicketReplyResponse{
		ID:         r.ID,
		TicketID:   r.TicketID,
		AuthorID:   r.AuthorID,
		AuthorType: string(r.AuthorType),
		Content:    r.Content,
		IsInternal: r.IsInternal,
		CreatedAt:  r.CreatedAt,
	}
	if r.AuthorType == AuthorTypeAdmin {
		resp.IsAdmin = true
		adminID := r.AuthorID
		resp.AdminID = &adminID
	} else {
		resp.UserID = r.AuthorID
	}
	return resp
}

// ===== 公告 DTO =====

type CreateAnnouncementRequest struct {
	Title          string `json:"title" binding:"required,min=1,max=200"`
	Content        string `json:"content" binding:"required"`
	Summary        string `json:"summary"`
	Type           string `json:"type"`
	TargetAudience string `json:"target_audience"`
	Pinned         bool   `json:"pinned"`
	EffectiveAt    *time.Time `json:"effective_at,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
}

type UpdateAnnouncementRequest struct {
	Title   *string `json:"title,omitempty"`
	Content *string `json:"content,omitempty"`
	Summary *string `json:"summary,omitempty"`
	Type    *string `json:"type,omitempty"`
	Status  *string `json:"status,omitempty"`
	Pinned  *bool   `json:"pinned,omitempty"`
}

type AnnouncementListQuery struct {
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
	Status   string `form:"status"`
	Type     string `form:"type"`
	Search   string `form:"search"`
	Pinned   *bool  `form:"pinned"`
}

type AnnouncementResponse struct {
	ID             uuid.UUID  `json:"id"`
	Title          string     `json:"title"`
	Content        string     `json:"content"`
	Summary        *string    `json:"summary,omitempty"`
	Type           string     `json:"type"`
	Status         string     `json:"status"`
	TargetAudience string     `json:"target_audience"`
	Pinned         bool       `json:"pinned"`
	ViewCount      int        `json:"view_count"`
	ReadCount      int        `json:"read_count"`
	EffectiveAt    *time.Time `json:"effective_at,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	PublishedAt    *time.Time `json:"published_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func NewAnnouncementResponse(a *Announcement) AnnouncementResponse {
	return AnnouncementResponse{
		ID:             a.ID,
		Title:          a.Title,
		Content:        a.Content,
		Summary:        a.Summary,
		Type:           string(a.Type),
		Status:         string(a.Status),
		TargetAudience: string(a.TargetAudience),
		Pinned:         a.Pinned,
		ViewCount:      a.ViewCount,
		ReadCount:      a.ReadCount,
		EffectiveAt:    a.EffectiveAt,
		ExpiresAt:      a.ExpiresAt,
		PublishedAt:    a.PublishedAt,
		CreatedAt:      a.CreatedAt,
		UpdatedAt:      a.UpdatedAt,
	}
}

// ===== 通知 DTO =====

type CreateNotificationRequest struct {
	UserID   string `json:"user_id" binding:"required"`
	Category string `json:"category"`
	Title    string `json:"title" binding:"required"`
	Content  string `json:"content" binding:"required"`
	Channel  string `json:"channel"`
	Priority string `json:"priority"`
}

type NotificationListQuery struct {
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
	UserID   string `form:"user_id"`
	Category string `form:"category"`
	Status   string `form:"status"`
	Channel  string `form:"channel"`
	// ExcludeArchived 为 true 时在 SQL 中追加 archived_at IS NULL 过滤，
	// 避免在内存中过滤（用户端列表使用）。
	ExcludeArchived bool `form:"-"`
}

type NotificationResponse struct {
	ID        uuid.UUID  `json:"id"`
	UserID    uuid.UUID  `json:"user_id"`
	Category  string     `json:"category"`
	Title    string     `json:"title"`
	Content  string     `json:"content"`
	Channel  string     `json:"channel"`
	Status   string     `json:"status"`
	Priority string     `json:"priority"`
	ReadAt   *time.Time `json:"read_at,omitempty"`
	SentAt   *time.Time `json:"sent_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func NewNotificationResponse(n *Notification) NotificationResponse {
	return NotificationResponse{
		ID:        n.ID,
		UserID:    n.UserID,
		Category:  string(n.Category),
		Title:     n.Title,
		Content:   n.Content,
		Channel:   string(n.Channel),
		Status:    string(n.Status),
		Priority:  string(n.Priority),
		ReadAt:    n.ReadAt,
		SentAt:    n.SentAt,
		CreatedAt: n.CreatedAt,
	}
}

// NotificationTemplateResponse
type NotificationTemplateResponse struct {
	ID            uuid.UUID `json:"id"`
	Code          string    `json:"code"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Category      string    `json:"category"`
	Channel       string    `json:"channel"`
	TitleTemplate string    `json:"title_template"`
	BodyTemplate  string    `json:"body_template"`
	Enabled       bool      `json:"enabled"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func NewNotificationTemplateResponse(t *NotificationTemplate) NotificationTemplateResponse {
	return NotificationTemplateResponse{
		ID:            t.ID,
		Code:          t.Code,
		Name:          t.Name,
		Description:   t.Description,
		Category:      string(t.Category),
		Channel:       string(t.Channel),
		TitleTemplate: t.TitleTemplate,
		BodyTemplate:  t.BodyTemplate,
		Enabled:       t.Enabled,
		UpdatedAt:     t.UpdatedAt,
	}
}

// =====================================================
// 佣金明细 / 邀请明细 DTO
// =====================================================

// CommissionDetailResponse 单条佣金记录（用户视角）
type CommissionDetailResponse struct {
	ID                uuid.UUID  `json:"id"`
	InviteeID         uuid.UUID  `json:"invitee_id"`
	OrderID           *uuid.UUID `json:"order_id,omitempty"`
	TradeNo           *string    `json:"trade_no,omitempty"`
	OrderAmount       float64    `json:"order_amount"`
	GetAmount         float64    `json:"get_amount"`
	CommissionBalance float64    `json:"commission_balance"`
	// 0=待结算 1=已结算 2=已取消
	Status     int       `json:"status"`
	StatusText string    `json:"status_text"`
	CreatedAt  time.Time `json:"created_at"`
}

func NewCommissionDetailResponse(c *CommissionLog) CommissionDetailResponse {
	statusText := "pending"
	switch c.Status {
	case 1:
		statusText = "settled"
	case 2:
		statusText = "canceled"
	}
	return CommissionDetailResponse{
		ID:                c.ID,
		InviteeID:         c.InviteeID,
		OrderID:           c.OrderID,
		TradeNo:           c.TradeNo,
		OrderAmount:       c.OrderAmount,
		GetAmount:         c.GetAmount,
		CommissionBalance: c.CommissionBalance,
		Status:            c.Status,
		StatusText:        statusText,
		CreatedAt:         c.CreatedAt,
	}
}

// InvitationResponse 被邀请用户列表项（脱敏，不含敏感字段）
type InvitationResponse struct {
	ID            uuid.UUID  `json:"id"`
	Email         string     `json:"email"`
	Username      *string    `json:"username,omitempty"`
	Status        string     `json:"status"`
	EmailVerified bool       `json:"email_verified"`
	RegisteredAt  *time.Time `json:"registered_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

func NewInvitationResponse(u *User) InvitationResponse {
	return InvitationResponse{
		ID:            u.ID,
		Email:         u.Email,
		Username:      u.Username,
		Status:        string(u.Status),
		EmailVerified: u.EmailVerifiedAt != nil,
		RegisteredAt:  u.RegisteredAt,
		CreatedAt:     u.CreatedAt,
	}
}

// =====================================================
// 用户注册/验证/密码重置 DTO
// =====================================================

type UserRegisterRequest struct {
	Email      string `json:"email" binding:"required,email"`
	Password   string `json:"password" binding:"required,min=8,max=128"`
	Username   string `json:"username"`
	InviteCode string `json:"invite_code,omitempty"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type ResetPasswordRequest struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=8,max=128"`
}

type UpdateProfileRequest struct {
	Username     *string `json:"username,omitempty"`
	ContactEmail *string `json:"contact_email,omitempty" binding:"omitempty,email"`
	Phone        *string `json:"phone,omitempty"`
	CountryCode  *string `json:"country_code,omitempty"`
	AvatarURL    *string `json:"avatar_url,omitempty"`
}

type UserDetailResponse struct {
	ID               uuid.UUID              `json:"id"`
	Email            string                 `json:"email"`
	Username         *string                `json:"username,omitempty"`
	Status           string                 `json:"status"`
	IsBanned         bool                   `json:"is_banned"`
	IsAdmin          bool                   `json:"is_admin"`
	UUID             string                 `json:"uuid"`
	EmailVerified    bool                   `json:"email_verified"`
	NotifyExpiry     bool                   `json:"notify_expiry"`
	NotifyTraffic    bool                   `json:"notify_traffic"`
	NotifyTicketReply bool                  `json:"notify_ticket_reply"`
	CommissionBalance float64               `json:"commission_balance"`
	CommissionTotal  float64                `json:"commission_total"`
	Locale           string                 `json:"locale"`
	Timezone         string                 `json:"timezone"`
	Profile          *UserProfile           `json:"profile,omitempty"`
	Subscription     *UserSubscription      `json:"subscription,omitempty"`
	Plan             *PlanResponse          `json:"plan,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
	LastLoginAt      *time.Time             `json:"last_login_at,omitempty"`
}

type UserRegisterResponse struct {
	UserID             uuid.UUID `json:"user_id"`
	RequiresVerification bool    `json:"requires_verification"`
	SubscriptionToken  string    `json:"subscription_token,omitempty"`
}

// =====================================================
// 订阅Token DTO
// =====================================================

type CreateTokenRequest struct {
	ClientHint string `json:"client_hint"`
}

type SubscriptionTokenResponse struct {
	ID           uuid.UUID  `json:"id"`
	Token        string     `json:"token,omitempty"`
	TokenPreview string     `json:"token_preview"`
	ClientHint   *string    `json:"client_hint,omitempty"`
	Status       string     `json:"status"`
	BoundIP      *string    `json:"bound_ip,omitempty"`
	LastAccessAt *time.Time `json:"last_access_at,omitempty"`
	LastAccessIP *string    `json:"last_access_ip,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// =====================================================
// 管理员用户管理 DTO
// =====================================================

type AdminUserListQuery struct {
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
	Status   string `form:"status"`
	Search   string `form:"search"`
}

type AdminCreateUserRequest struct {
	Email            string     `json:"email" binding:"required,email"`
	Password         string     `json:"password" binding:"required,min=6,max=128"`
	PlanID           *uuid.UUID `json:"plan_id,omitempty"`
	TransferEnableGB *int       `json:"transfer_enable_gb,omitempty"`
	DurationDays     *int       `json:"duration_days,omitempty"`
	Remarks          string     `json:"remarks,omitempty"`
}

type AdminUpdateUserRequest struct {
	Email               *string     `json:"email,omitempty" binding:"omitempty,email"`
	Password            *string     `json:"password,omitempty" binding:"omitempty,min=6,max=128"`
	PlanID              *uuid.UUID  `json:"plan_id,omitempty"`
	TransferEnableBytes *int64      `json:"transfer_enable_bytes,omitempty"`
	ExpiresAt           *time.Time  `json:"expires_at,omitempty"`
	Notes               *string     `json:"notes,omitempty"`
	Tags                []string    `json:"tags,omitempty"`
	Status              *string     `json:"status,omitempty" binding:"omitempty,oneof=pending active disabled banned expired"`
}

type BanUserRequest struct {
	Reason string `json:"reason" binding:"required"`
}

type AddTrafficRequest struct {
	Bytes int64 `json:"bytes" binding:"required"`
}

type ExtendSubscriptionRequest struct {
	Days int `json:"days" binding:"required,min=1"`
}

type ChangePlanRequest struct {
	PlanID    uuid.UUID `json:"plan_id" binding:"required"`
	Immediate bool      `json:"immediate"`
}

type BatchUserRequest struct {
	UserIDs []uuid.UUID `json:"user_ids" binding:"required,min=1"`
}

type ResetPasswordResponse struct {
	NewPassword string `json:"new_password"`
}

// =====================================================
// Plan DTO
// =====================================================

type PlanResponse struct {
	ID             uuid.UUID   `json:"id"`
	Code           string      `json:"code"`
	Name           string      `json:"name"`
	Description    string      `json:"description,omitempty"`
	Content        string      `json:"content,omitempty"`
	Status         string      `json:"status"`
	BillingType    string      `json:"billing_type"`
	TrafficBytes   int64       `json:"traffic_bytes"`
	SpeedLimitMbps *int        `json:"speed_limit_mbps,omitempty"`
	DeviceLimit    *int        `json:"device_limit,omitempty"`
	IPLimit        *int        `json:"ip_limit,omitempty"`
	ResetCycle     *string     `json:"reset_cycle,omitempty"`
	DurationDays   *int        `json:"duration_days,omitempty"`
	CanRenew       bool        `json:"can_renew"`
	SortOrder      int         `json:"sort_order"`
	GroupID        *uuid.UUID  `json:"group_id,omitempty"`
	NodeCount      int         `json:"node_count"`
	Tags           []string    `json:"tags"`
	Features       []string    `json:"features,omitempty"`
	FeatureFlags   map[string]interface{} `json:"feature_flags,omitempty"`
	Prices         []PlanPrice `json:"prices,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

type PlanNodeInfo struct {
	ID          uuid.UUID `json:"id"`
	NumericID   int       `json:"numeric_id,omitempty"`
	Name        string    `json:"name"`
	CountryCode string    `json:"country_code,omitempty"`
	CountryFlag string    `json:"country_flag,omitempty"`
	RegionCode  string    `json:"region_code,omitempty"`
	Protocol    string    `json:"protocol,omitempty"`
	Rate        float64   `json:"rate,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	IsOnline    bool      `json:"is_online"`
}

type TrafficLog struct {
	Date     string `json:"date"`
	Upload   int64  `json:"upload"`
	Download int64  `json:"download"`
	Total    int64  `json:"total"`
}

type PlanPrice struct {
	PeriodCode string  `json:"period_code"`
	PriceUSDT  float64 `json:"price_usdt"`
	PriceCNY   float64 `json:"price_cny"`
}

func NewPlanResponse(p *Plan) PlanResponse {
	features := p.Features
	if features == nil {
		features = []string{}
	}
	tags := p.Tags
	if tags == nil {
		tags = []string{}
	}
	var featureFlags map[string]interface{}
	if len(p.FeatureFlags) > 0 {
		_ = json.Unmarshal(p.FeatureFlags, &featureFlags)
	}
	// 向后兼容：如果 Description 为空但 feature_flags.description 存在，使用它
	description := p.Description
	if description == "" && featureFlags != nil {
		if desc, ok := featureFlags["description"].(string); ok && desc != "" {
			description = desc
		}
	}
	// 从 feature_flags 提取 features 列表（如果数据库中 features 为空）
	if len(features) == 0 && featureFlags != nil {
		if ff, ok := featureFlags["features"].([]interface{}); ok {
			for _, f := range ff {
				if s, ok := f.(string); ok {
					features = append(features, s)
				}
			}
		}
	}
	return PlanResponse{
		ID:             p.ID,
		Code:           p.Code,
		Name:           p.Name,
		Description:    description,
		Content:        p.Content,
		Status:         string(p.Status),
		BillingType:    string(p.BillingType),
		TrafficBytes:   p.TrafficBytes,
		SpeedLimitMbps: p.SpeedLimitMbps,
		DeviceLimit:    p.DeviceLimit,
		IPLimit:        p.IPLimit,
		ResetCycle:     p.ResetCycle,
		DurationDays:   p.DurationDays,
		CanRenew:       p.CanRenew,
		SortOrder:      p.SortOrder,
		GroupID:        p.GroupID,
		NodeCount:      p.NodeCount,
		Tags:           tags,
		Features:       features,
		FeatureFlags:   featureFlags,
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}
}

func NewUserDetailResponse(u *User, profile *UserProfile, sub *UserSubscription) UserDetailResponse {
	isBanned := u.Status == UserStatusBanned
	return UserDetailResponse{
		ID:                u.ID,
		Email:             u.Email,
		Username:          u.Username,
		Status:            string(u.Status),
		IsBanned:          isBanned,
		IsAdmin:           false,
		UUID:              u.UUID,
		EmailVerified:     u.EmailVerifiedAt != nil,
		NotifyExpiry:      u.NotifyExpiry,
		NotifyTraffic:     u.NotifyTraffic,
		NotifyTicketReply: u.NotifyTicketReply,
		CommissionBalance: u.CommissionBalance,
		CommissionTotal:   u.CommissionTotal,
		Locale:            u.Locale,
		Timezone:          u.Timezone,
		Profile:           profile,
		Subscription:      sub,
		CreatedAt:         u.CreatedAt,
		LastLoginAt:       u.LastLoginAt,
	}
}

func NewSubscriptionTokenResponse(t *SubscriptionToken) SubscriptionTokenResponse {
	return SubscriptionTokenResponse{
		ID:           t.ID,
		TokenPreview: t.TokenPreview,
		ClientHint:   t.ClientHint,
		Status:       string(t.Status),
		BoundIP:      t.BoundIP,
		LastAccessAt: t.LastAccessAt,
		LastAccessIP: t.LastAccessIP,
		ExpiresAt:    t.ExpiresAt,
		CreatedAt:    t.CreatedAt,
	}
}

