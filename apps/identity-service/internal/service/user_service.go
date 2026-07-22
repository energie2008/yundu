package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/airport-panel/config/events"
	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/pkg"
	"github.com/airport-panel/identity-service/internal/repo"
	"github.com/google/uuid"
)

var (
	ErrInvalidVerifyToken   = errors.New("invalid or expired verify token")
	ErrInvalidResetToken    = errors.New("invalid or expired reset token")
	ErrUserBanned           = errors.New("user is banned")
	ErrUserPending          = errors.New("please verify your email first")
	ErrTokenNotFound        = errors.New("subscription token not found")
	ErrPlanNotExist         = errors.New("plan does not exist")
	ErrImpersonateDisabled  = errors.New("impersonation is disabled")
)

const (
	verifyEmailPrefix   = "verify_email:"
	resetPasswordPrefix = "reset_password:"
	impersonatePrefix   = "impersonate:"
	verifyEmailTTL      = 24 * time.Hour
	resetPasswordTTL    = 1 * time.Hour
	impersonateTTL      = 5 * time.Minute

	// 订阅 token raw 值的 Redis 缓存。
	// DB 只存 token_hash（SHA256），rawToken 只在创建时返回一次。
	// 为支持 EnsureSubscriptionToken 的"ensure"语义（有则返回旧 token，无则创建），
	// 将 rawToken 缓存到 Redis，避免每次 ensure 都生成新 token 导致订阅地址变化。
	subRawTokenPrefix = "sub:raw_token:"
	subRawTokenTTL    = 30 * 24 * time.Hour
)

// getCachedRawToken 从 Redis 获取缓存的 raw subscription token，并验证其在 DB 中仍有效。
// 返回 (token, rawToken, true) 表示命中且有效；返回 (nil, "", false) 表示未命中或已失效。
func (s *UserService) getCachedRawToken(ctx context.Context, userID uuid.UUID) (*model.SubscriptionToken, string, bool) {
	if s.redisClient == nil {
		return nil, "", false
	}
	key := subRawTokenPrefix + userID.String()
	raw, err := s.redisClient.Get(ctx, key).Result()
	if err != nil || raw == "" {
		return nil, "", false
	}
	// 验证 DB 中该 token 仍 active 且未过期
	hash := pkg.HashToken(raw)
	token, err := s.tokens.GetByTokenHash(ctx, hash)
	if err != nil || token == nil || token.Status != model.SubscriptionTokenStatusActive {
		return nil, "", false
	}
	if token.ExpiresAt != nil && token.ExpiresAt.Before(time.Now()) {
		return nil, "", false
	}
	return token, raw, true
}

// cacheRawToken 将 raw subscription token 缓存到 Redis。
func (s *UserService) cacheRawToken(ctx context.Context, userID uuid.UUID, raw string) {
	if s.redisClient == nil {
		return
	}
	key := subRawTokenPrefix + userID.String()
	if err := s.redisClient.Set(ctx, key, raw, subRawTokenTTL).Err(); err != nil {
		s.logger.Warn("failed to cache raw subscription token", "user", userID, "error", err)
	}
}

// evictCachedRawToken 清除 Redis 中缓存的 raw subscription token。
// 在重置/吊销/封禁/删除等场景调用，确保旧 token 不再被 EnsureSubscriptionToken 返回。
func (s *UserService) evictCachedRawToken(ctx context.Context, userID uuid.UUID) {
	if s.redisClient == nil {
		return
	}
	key := subRawTokenPrefix + userID.String()
	if err := s.redisClient.Del(ctx, key).Err(); err != nil {
		s.logger.Warn("failed to evict cached raw subscription token", "user", userID, "error", err)
	}
}

type SMTPConfig struct {
	Enabled  bool   `json:"enabled"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	BaseURL  string `json:"-"`
}

type UserService struct {
	users       *repo.UserRepo
	profiles    *repo.UserProfileRepo
	subs        *repo.SubscriptionRepo
	plans       *repo.PlanRepo
	tokens      *repo.SubscriptionTokenRepo
	orders      *repo.PaymentOrderRepo
	settings    *repo.SettingRepo
	inviteCodes *repo.InviteCodeRepo
	auditSvc    *AuditService
	mailSvc     *MailService
	redisClient *goredis.Client
	logger      *slog.Logger
	onEvent     func(ctx context.Context, topic string, payload interface{})
}

func NewUserService(
	users *repo.UserRepo,
	profiles *repo.UserProfileRepo,
	subs *repo.SubscriptionRepo,
	plans *repo.PlanRepo,
	tokens *repo.SubscriptionTokenRepo,
	orders *repo.PaymentOrderRepo,
	settings *repo.SettingRepo,
	inviteCodes *repo.InviteCodeRepo,
	auditSvc *AuditService,
	mailSvc *MailService,
	redisClient *goredis.Client,
	logger *slog.Logger,
) *UserService {
	return &UserService{
		users:       users,
		profiles:    profiles,
		subs:        subs,
		plans:       plans,
		tokens:      tokens,
		orders:      orders,
		settings:    settings,
		inviteCodes: inviteCodes,
		auditSvc:    auditSvc,
		mailSvc:     mailSvc,
		redisClient: redisClient,
		logger:      logger,
		onEvent:     func(ctx context.Context, topic string, payload interface{}) {},
	}
}

func (s *UserService) SetEventPublisher(fn func(ctx context.Context, topic string, payload interface{})) {
	if fn != nil {
		s.onEvent = fn
	}
}

type RegisterResult struct {
	User              *model.User
	SubscriptionToken string
}

func (s *UserService) Register(ctx context.Context, req *model.UserRegisterRequest, ip string) (*RegisterResult, error) {
	existing, err := s.users.GetByEmail(ctx, req.Email)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrUserAlreadyExists
	}

	var inviterID *uuid.UUID
	if strings.TrimSpace(req.InviteCode) != "" {
		invCode, err := s.inviteCodes.GetByCode(ctx, strings.TrimSpace(req.InviteCode))
		if err == nil && invCode != nil {
			uid := invCode.UserID
			inviterID = &uid
			_ = s.inviteCodes.IncrementPV(ctx, invCode.Code)
		}
	}

	passwordHash, err := pkg.HashPassword(req.Password, nil)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	user := &model.User{
		ID:                uuid.New(),
		Email:             strings.ToLower(strings.TrimSpace(req.Email)),
		PasswordAlgo:      "argon2id",
		Status:            model.UserStatusPending,
		Locale:            "zh-CN",
		Timezone:          "Asia/Shanghai",
		InviterID:         inviterID,
		RegisteredAt:      &now,
		NotifyExpiry:      true,
		NotifyTraffic:     true,
		NotifyTicketReply: true,
	}
	hashStr := passwordHash
	user.PasswordHash = &hashStr
	if req.Username != "" {
		username := strings.TrimSpace(req.Username)
		user.Username = &username
	}

	if err := s.users.Create(ctx, user); err != nil {
		return nil, err
	}

	profile := &model.UserProfile{
		UserID: user.ID,
	}
	_ = s.profiles.Create(ctx, profile)

	_ = s.createInviteCodesForUser(ctx, user.ID, 1)

	verifyToken := pkg.GenerateRandomString(64)
	if s.redisClient != nil {
		s.redisClient.Set(ctx, verifyEmailPrefix+verifyToken, user.ID.String(), verifyEmailTTL)
	}

	mailSent := false
	var rawSubToken string
	if s.mailSvc != nil && s.mailSvc.IsEnabled() {
		if err := s.mailSvc.SendVerifyEmail(ctx, user.Email, verifyToken); err == nil {
			mailSent = true
		} else {
			s.logger.Warn("failed to send verify email, auto-activating user", "email", user.Email, "error", err)
		}
	} else {
		s.logger.Warn("mail service not enabled, auto-activating user", "email", user.Email, "verify_token", verifyToken)
	}

	if !mailSent {
		s.logger.Warn("mail service unavailable or send failed, auto-activating user", "email", user.Email, "verify_token", verifyToken)
		now := time.Now()
		user.Status = model.UserStatusActive
		user.EmailVerifiedAt = &now
		if err := s.users.UpdateEmailVerified(ctx, user.ID); err != nil {
			s.logger.Error("failed to auto-activate user", "error", err)
		} else {
			freePlanID, err := s.getDefaultFreePlanID(ctx)
			if err == nil && freePlanID != uuid.Nil {
				plan, err := s.plans.GetByID(ctx, freePlanID)
				if err == nil && plan != nil {
					sub := &model.UserPlanSubscription{
						ID:                uuid.New(),
						UserID:            user.ID,
						PlanID:            freePlanID,
						Status:            model.SubscriptionStatusActive,
						StartedAt:         &now,
						RenewalMode:       model.RenewalModeManual,
						TrafficQuotaBytes: plan.TrafficBytes,
						TrafficUsedBytes:  0,
						SpeedLimitMbps:    plan.SpeedLimitMbps,
						DeviceLimit:       plan.DeviceLimit,
						IPLimit:           plan.IPLimit,
						Source:            "free_trial",
					}
					_ = s.subs.Create(ctx, sub)
				}
			}
			_, rawSubToken, _ = s.CreateSubscriptionToken(ctx, user.ID, "default", "")
		}
	}

	if rawSubToken == "" {
		_, rawSubToken, _, _ = s.EnsureSubscriptionToken(ctx, user.ID, ip)
	}

	s.writeAudit(ctx, model.ActorTypeUser, &user.ID, user.Email, "register", "user", &user.ID, ip, nil, user)

	return &RegisterResult{
		User:              user,
		SubscriptionToken: rawSubToken,
	}, nil
}

func (s *UserService) GetNotificationPreferences(ctx context.Context, userID uuid.UUID) (*model.NotificationPreferences, error) {
	return s.users.GetNotificationPreferences(ctx, userID)
}

func (s *UserService) UpdateNotificationPreferences(ctx context.Context, userID uuid.UUID, prefs *model.NotificationPreferences) error {
	return s.users.UpdateNotificationPreferences(ctx, userID, prefs)
}

// EnsureSubscriptionToken 确保用户有一个可用的订阅 token，并返回 raw token 值。
// 语义：如果用户已有 active token 且 Redis 中缓存了 rawToken，则返回现有 token（is_new=false）；
// 否则吊销所有旧 token 并创建新 token（is_new=true）。
// rawToken 只在创建时生成（DB 只存 SHA256 hash），因此用 Redis 缓存 rawToken 以支持幂等查询。
// 修复前：每次调用都 RevokeAll + Create，导致用户每次刷新页面订阅地址都变化。
func (s *UserService) EnsureSubscriptionToken(ctx context.Context, userID uuid.UUID, ip string) (*model.SubscriptionToken, string, bool, error) {
	// 1. 先查 Redis 缓存，命中且 DB 中仍 active 则直接返回（is_new=false）
	if token, raw, ok := s.getCachedRawToken(ctx, userID); ok {
		return token, raw, false, nil
	}
	// 2. 未命中或已失效 → 吊销所有旧 token + 创建新 token + 缓存
	if err := s.tokens.RevokeAllForUser(ctx, userID); err != nil {
		s.logger.Warn("failed to revoke existing tokens", "error", err)
	}
	token, raw, err := s.CreateSubscriptionToken(ctx, userID, "default", ip)
	if err != nil {
		return nil, "", false, err
	}
	s.cacheRawToken(ctx, userID, raw)
	return token, raw, true, nil
}

func (s *UserService) createInviteCodesForUser(ctx context.Context, userID uuid.UUID, count int) error {
	if s.inviteCodes == nil {
		return nil
	}
	for i := 0; i < count; i++ {
		code := s.generateInviteCode()
		ic := &model.InviteCode{
			ID:     uuid.New(),
			UserID: userID,
			Code:   code,
			PV:     0,
		}
		if err := s.inviteCodes.Create(ctx, ic); err != nil {
			s.logger.Warn("failed to create invite code", "user", userID, "error", err)
			return err
		}
	}
	return nil
}

func (s *UserService) generateInviteCode() string {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

func (s *UserService) VerifyEmail(ctx context.Context, token string) error {
	if s.redisClient == nil {
		return errors.New("redis not available")
	}

	key := verifyEmailPrefix + token
	userIDStr, err := s.redisClient.Get(ctx, key).Result()
	if err != nil {
		if err == goredis.Nil {
			return ErrInvalidVerifyToken
		}
		return err
	}

	s.redisClient.Del(ctx, key)

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return ErrInvalidVerifyToken
	}

	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrInvalidVerifyToken
	}

	if err := s.users.UpdateEmailVerified(ctx, userID); err != nil {
		return err
	}

	freePlanID, err := s.getDefaultFreePlanID(ctx)
	if err == nil && freePlanID != uuid.Nil {
		plan, err := s.plans.GetByID(ctx, freePlanID)
		if err == nil && plan != nil {
			now := time.Now()
			sub := &model.UserPlanSubscription{
				ID:                uuid.New(),
				UserID:            userID,
				PlanID:            freePlanID,
				Status:            model.SubscriptionStatusActive,
				StartedAt:         &now,
				RenewalMode:       model.RenewalModeManual,
				TrafficQuotaBytes: plan.TrafficBytes,
				TrafficUsedBytes:  0,
				SpeedLimitMbps:    plan.SpeedLimitMbps,
				DeviceLimit:       plan.DeviceLimit,
				IPLimit:           plan.IPLimit,
				Source:            "free_trial",
			}
			_ = s.subs.Create(ctx, sub)
		}
	}

	if _, raw, err := s.CreateSubscriptionToken(ctx, userID, "default", ""); err == nil {
		s.cacheRawToken(ctx, userID, raw)
	}

	s.writeAudit(ctx, model.ActorTypeUser, &userID, user.Email, "verify_email", "user", &userID, "", map[string]string{"status": "pending"}, map[string]string{"status": "active", "email_verified": "true"})

	return nil
}

func (s *UserService) ForgotPassword(ctx context.Context, email string) error {
	user, err := s.users.GetByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return err
	}
	if user == nil {
		return nil
	}

	resetToken := pkg.GenerateRandomString(64)
	if s.redisClient != nil {
		s.redisClient.Set(ctx, resetPasswordPrefix+resetToken, user.ID.String(), resetPasswordTTL)
	}

	if s.mailSvc != nil {
		_ = s.mailSvc.SendResetPassword(ctx, user.Email, resetToken)
	}

	s.writeAudit(ctx, model.ActorTypeUser, &user.ID, user.Email, "forgot_password", "user", &user.ID, "", nil, nil)
	return nil
}

func (s *UserService) ResetPassword(ctx context.Context, token, newPassword string) error {
	if s.redisClient == nil {
		return errors.New("redis not available")
	}

	key := resetPasswordPrefix + token
	userIDStr, err := s.redisClient.Get(ctx, key).Result()
	if err != nil {
		if err == goredis.Nil {
			return ErrInvalidResetToken
		}
		return err
	}

	s.redisClient.Del(ctx, key)

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return ErrInvalidResetToken
	}

	passwordHash, err := pkg.HashPassword(newPassword, nil)
	if err != nil {
		return err
	}

	if err := s.users.UpdatePassword(ctx, userID, passwordHash); err != nil {
		return err
	}

	s.writeAudit(ctx, model.ActorTypeUser, &userID, "", "reset_password", "user", &userID, "", nil, nil)
	return nil
}

func (s *UserService) GetUserDetail(ctx context.Context, userID uuid.UUID) (*model.User, *model.UserProfile, *model.UserSubscription, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, nil, nil, err
	}
	if user == nil {
		return nil, nil, nil, ErrUserNotFound
	}

	profile, _ := s.profiles.GetByUserID(ctx, userID)
	sub, _ := s.users.GetSubscription(ctx, userID)

	return user, profile, sub, nil
}

func (s *UserService) UpdateProfile(ctx context.Context, userID uuid.UUID, req *model.UpdateProfileRequest, ip string) error {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}

	if req.Username != nil {
		user.Username = req.Username
	}
	if err := s.users.Update(ctx, user); err != nil {
		return err
	}

	profile, err := s.profiles.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}
	if profile == nil {
		profile = &model.UserProfile{UserID: userID}
		_ = s.profiles.Create(ctx, profile)
	}

	if req.ContactEmail != nil {
		profile.ContactEmail = req.ContactEmail
	}
	if req.Phone != nil {
		profile.Phone = req.Phone
	}
	if req.CountryCode != nil {
		profile.CountryCode = req.CountryCode
	}
	if req.AvatarURL != nil {
		profile.AvatarURL = req.AvatarURL
	}
	if err := s.profiles.Update(ctx, profile); err != nil {
		return err
	}

	s.writeAudit(ctx, model.ActorTypeUser, &userID, user.Email, "update_profile", "user", &userID, ip, nil, req)
	return nil
}

func (s *UserService) GetSubscription(ctx context.Context, userID uuid.UUID) (*model.UserSubscription, error) {
	return s.users.GetSubscription(ctx, userID)
}

func (s *UserService) ListSubscriptionTokens(ctx context.Context, userID uuid.UUID) ([]*model.SubscriptionToken, error) {
	return s.tokens.ListByUser(ctx, userID)
}

func (s *UserService) CreateSubscriptionToken(ctx context.Context, userID uuid.UUID, clientHint string, ip string) (*model.SubscriptionToken, string, error) {
	rawToken, tokenHash := pkg.GenerateSubscriptionToken()
	preview := rawToken[:16]

	var hint *string
	if clientHint != "" {
		hint = &clientHint
	}

	var boundIP *string
	if ip != "" {
		boundIP = &ip
	}

	token := &model.SubscriptionToken{
		ID:           uuid.New(),
		UserID:       userID,
		TokenHash:    tokenHash,
		TokenPreview: preview,
		Status:       model.SubscriptionTokenStatusActive,
		ClientHint:   hint,
		BoundIP:      boundIP,
		AllowIPBind:  true,
	}

	if err := s.tokens.Create(ctx, token); err != nil {
		return nil, "", err
	}

	s.writeAudit(ctx, model.ActorTypeUser, &userID, "", "create_token", "subscription_token", &token.ID, ip, nil, token)
	return token, rawToken, nil
}

func (s *UserService) RevokeSubscriptionToken(ctx context.Context, userID, tokenID uuid.UUID, ip string) error {
	token, err := s.tokens.GetByID(ctx, tokenID)
	if err != nil {
		return err
	}
	if token == nil || token.UserID != userID {
		return ErrTokenNotFound
	}
	if err := s.tokens.Revoke(ctx, tokenID); err != nil {
		return err
	}
	s.writeAudit(ctx, model.ActorTypeUser, &userID, "", "revoke_token", "subscription_token", &tokenID, ip, nil, nil)
	return nil
}

func (s *UserService) ResetSubscriptionToken(ctx context.Context, userID, tokenID uuid.UUID, ip string) (*model.SubscriptionToken, string, error) {
	token, err := s.tokens.GetByID(ctx, tokenID)
	if err != nil {
		return nil, "", err
	}
	if token == nil || token.UserID != userID {
		return nil, "", ErrTokenNotFound
	}
	if err := s.tokens.Revoke(ctx, tokenID); err != nil {
		return nil, "", err
	}
	s.evictCachedRawToken(ctx, userID)
	newToken, raw, err := s.CreateSubscriptionToken(ctx, userID, "", ip)
	if err != nil {
		return nil, "", err
	}
	s.cacheRawToken(ctx, userID, raw)
	return newToken, raw, nil
}

func (s *UserService) ResetAllSubscriptionTokens(ctx context.Context, userID uuid.UUID, ip string) (*model.SubscriptionToken, string, error) {
	if err := s.tokens.RevokeAllForUser(ctx, userID); err != nil {
		return nil, "", err
	}
	s.evictCachedRawToken(ctx, userID)
	token, raw, err := s.CreateSubscriptionToken(ctx, userID, "reset", ip)
	if err != nil {
		return nil, "", err
	}
	s.cacheRawToken(ctx, userID, raw)
	s.writeAudit(ctx, model.ActorTypeUser, &userID, "", "reset_all_tokens", "subscription_token", nil, ip, nil, nil)
	return token, raw, nil
}

func (s *UserService) UpdateTokenAccess(ctx context.Context, tokenHash string, ip net.IP) error {
	token, err := s.tokens.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		return err
	}
	if token == nil {
		return ErrTokenNotFound
	}
	return s.tokens.UpdateAccess(ctx, token.ID, ip)
}

func (s *UserService) GetUserByTokenHash(ctx context.Context, tokenHash string) (*model.User, error) {
	return s.users.FindBySubscriptionTokenHash(ctx, tokenHash)
}

func (s *UserService) ListActivePlans(ctx context.Context) ([]*model.Plan, error) {
	items, err := s.plans.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range items {
		prices, err := s.plans.GetPrices(ctx, p.ID)
		if err != nil {
			s.logger.Warn("failed to get plan prices", "error", err)
		} else {
			p.Prices = prices
		}
		nc, err := s.plans.CountNodesForPlan(ctx, p.ID)
		if err != nil {
			s.logger.Warn("failed to count plan nodes", "plan_id", p.ID, "error", err)
		} else {
			p.NodeCount = nc
		}
	}
	return items, nil
}

func (s *UserService) GetPlan(ctx context.Context, id uuid.UUID) (*model.Plan, error) {
	plan, err := s.plans.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, ErrPlanNotExist
	}
	prices, err := s.plans.GetPrices(ctx, id)
	if err != nil {
		s.logger.Warn("failed to get plan prices", "error", err)
	} else {
		plan.Prices = prices
	}
	nc, err := s.plans.CountNodesForPlan(ctx, id)
	if err == nil {
		plan.NodeCount = nc
	}
	return plan, nil
}

func (s *UserService) ListPlanNodes(ctx context.Context, planID uuid.UUID) ([]*model.PlanNodeInfo, error) {
	p, err := s.plans.GetByID(ctx, planID)
	if err != nil {
		return nil, err
	}
	if p == nil || p.DeletedAt != nil {
		return nil, ErrPlanNotExist
	}
	return s.plans.ListNodesForPlan(ctx, planID)
}

// ListUserNodes 获取用户当前可见的节点列表（基于活跃订阅套餐）
func (s *UserService) ListUserNodes(ctx context.Context, userID uuid.UUID) ([]*model.PlanNodeInfo, error) {
	sub, err := s.users.GetSubscription(ctx, userID)
	if err != nil {
		return nil, err
	}
	if sub == nil {
		return []*model.PlanNodeInfo{}, nil
	}
	return s.plans.ListNodesForPlan(ctx, sub.PlanID)
}

func (s *UserService) GetUserTrafficLogs(ctx context.Context, userID uuid.UUID, days int) ([]*model.TrafficLog, error) {
	return s.plans.GetUserTrafficLogs(ctx, userID, days)
}

func (s *UserService) AdminCreateUser(ctx context.Context, adminID uuid.UUID, adminEmail string, req *model.AdminCreateUserRequest, ip string) (*model.User, error) {
	existing, err := s.users.GetByEmail(ctx, req.Email)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrUserAlreadyExists
	}

	passwordHash, err := pkg.HashPassword(req.Password, nil)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	user := &model.User{
		ID:                uuid.New(),
		Email:             strings.ToLower(strings.TrimSpace(req.Email)),
		PasswordAlgo:      "argon2id",
		Status:            model.UserStatusActive,
		Locale:            "zh-CN",
		Timezone:          "Asia/Shanghai",
		RegisteredAt:      &now,
		EmailVerifiedAt:   &now,
		NotifyExpiry:      true,
		NotifyTraffic:     true,
		NotifyTicketReply: true,
	}
	hashStr := passwordHash
	user.PasswordHash = &hashStr
	if req.Remarks != "" {
		remarks := req.Remarks
		user.Notes = &remarks
	}

	if err := s.users.Create(ctx, user); err != nil {
		return nil, err
	}

	profile := &model.UserProfile{UserID: user.ID}
	_ = s.profiles.Create(ctx, profile)

	if req.PlanID != nil && *req.PlanID != uuid.Nil {
		plan, err := s.plans.GetByID(ctx, *req.PlanID)
		if err != nil {
			s.logger.Warn("failed to get plan for admin-created user", "plan_id", *req.PlanID, "error", err)
		} else if plan != nil {
			sub := &model.UserPlanSubscription{
				ID:                uuid.New(),
				UserID:            user.ID,
				PlanID:            *req.PlanID,
				Status:            model.SubscriptionStatusActive,
				StartedAt:         &now,
				RenewalMode:       model.RenewalModeManual,
				TrafficQuotaBytes: plan.TrafficBytes,
				TrafficUsedBytes:  0,
				SpeedLimitMbps:    plan.SpeedLimitMbps,
				DeviceLimit:       plan.DeviceLimit,
				IPLimit:           plan.IPLimit,
				Source:            "admin_create",
			}
			if req.TransferEnableGB != nil && *req.TransferEnableGB > 0 {
				sub.TrafficQuotaBytes = int64(*req.TransferEnableGB) * 1024 * 1024 * 1024
			}
			if req.DurationDays != nil && *req.DurationDays > 0 {
				expiresAt := now.AddDate(0, 0, *req.DurationDays)
				sub.ExpiresAt = &expiresAt
			}
			if err := s.subs.Create(ctx, sub); err != nil {
				s.logger.Error("failed to create subscription for admin-created user", "user", user.ID, "error", err)
			}
		}
	}

	if _, raw, err := s.CreateSubscriptionToken(ctx, user.ID, "default", ip); err == nil {
		s.cacheRawToken(ctx, user.ID, raw)
	}

	actorType := model.ActorTypeAdmin
	s.writeAudit(ctx, actorType, &adminID, adminEmail, "create_user", "user", &user.ID, ip, nil, req)

	type userEvt struct {
		UserID   string `json:"user_id"`
		Operator string `json:"operator,omitempty"`
	}
	s.onEvent(ctx, "user:created", userEvt{UserID: user.ID.String(), Operator: adminEmail})
	return user, nil
}

func (s *UserService) AdminListUsers(ctx context.Context, page, pageSize int, status, search string) ([]model.UserDetailResponse, int, error) {
	users, total, err := s.users.List(ctx, page, pageSize, status, search)
	if err != nil {
		return nil, 0, err
	}

	var result []model.UserDetailResponse
	for _, u := range users {
		profile, _ := s.profiles.GetByUserID(ctx, u.ID)
		sub, _ := s.users.GetSubscription(ctx, u.ID)
		resp := model.NewUserDetailResponse(u, profile, sub)
		if sub != nil && sub.PlanID != uuid.Nil {
			if plan, err := s.plans.GetByID(ctx, sub.PlanID); err == nil && plan != nil {
				p := model.NewPlanResponse(plan)
				resp.Plan = &p
			}
		}
		resp.IsAdmin = false
		result = append(result, resp)
	}
	return result, total, nil
}

func (s *UserService) AdminGetUser(ctx context.Context, userID uuid.UUID) (*model.User, *model.UserProfile, *model.UserSubscription, error) {
	return s.GetUserDetail(ctx, userID)
}

func (s *UserService) AdminUpdateUser(ctx context.Context, adminID uuid.UUID, adminEmail string, userID uuid.UUID, req *model.AdminUpdateUserRequest, ip string) error {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}

	oldStatus := user.Status
	statusChanged := false
	planChanged := false

	if req.Email != nil {
		newEmail := strings.ToLower(strings.TrimSpace(*req.Email))
		if newEmail != "" && newEmail != user.Email {
			existing, err := s.users.GetByEmail(ctx, newEmail)
			if err != nil {
				return err
			}
			if existing != nil && existing.ID != userID {
				return ErrUserAlreadyExists
			}
			if err := s.users.UpdateEmail(ctx, userID, newEmail); err != nil {
				return err
			}
		}
	}
	if req.Password != nil && *req.Password != "" {
		passwordHash, err := pkg.HashPassword(*req.Password, nil)
		if err != nil {
			return err
		}
		if err := s.users.UpdatePassword(ctx, userID, passwordHash); err != nil {
			return err
		}
		if err := s.tokens.RevokeAllForUser(ctx, userID); err != nil {
			s.logger.Warn("failed to revoke tokens on password reset", "user", userID, "error", err)
		}
		s.evictCachedRawToken(ctx, userID)
	}
	if req.Notes != nil {
		if err := s.users.UpdateNotes(ctx, userID, req.Notes); err != nil {
			return err
		}
	}
	if req.Status != nil {
		newStatus := model.UserStatus(*req.Status)
		if newStatus != oldStatus {
			if err := s.users.UpdateStatus(ctx, userID, newStatus); err != nil {
				return err
			}
			statusChanged = true
			if newStatus == model.UserStatusBanned {
				s.evictCachedRawToken(ctx, userID)
				_ = s.tokens.RevokeAllForUser(ctx, userID)
			}
		}
	}
	if req.PlanID != nil && *req.PlanID != uuid.Nil {
		if err := s.AdminChangePlan(ctx, adminID, adminEmail, userID, *req.PlanID, true, ip); err != nil {
			return err
		}
		planChanged = true
	}
	if req.TransferEnableBytes != nil {
		if err := s.subs.UpdateQuotaBytes(ctx, userID, *req.TransferEnableBytes); err != nil {
			return err
		}
		planChanged = true
	}
	if req.ExpiresAt != nil {
		if err := s.subs.UpdateExpiresAt(ctx, userID, *req.ExpiresAt); err != nil {
			return err
		}
		planChanged = true
	}

	profile, err := s.profiles.GetByUserID(ctx, userID)
	if err == nil && profile != nil && req.Tags != nil {
		profile.Tags = req.Tags
		_ = s.profiles.Update(ctx, profile)
	}

	actorType := model.ActorTypeAdmin
	s.writeAudit(ctx, actorType, &adminID, adminEmail, "update_user", "user", &userID, ip, map[string]interface{}{"status": oldStatus}, req)

	if statusChanged {
		newStatus := model.UserStatus(*req.Status)
		if newStatus == model.UserStatusBanned {
			s.onEvent(ctx, events.TopicUserBanned, events.UserEvent{
				UserID:   userID.String(),
				Reason:   "admin_ban",
				Operator: adminEmail,
			})
		} else if oldStatus == model.UserStatusBanned && newStatus == model.UserStatusActive {
			s.onEvent(ctx, events.TopicUserUnbanned, events.UserEvent{
				UserID:   userID.String(),
				Operator: adminEmail,
			})
		}
	}
	if planChanged {
		s.onEvent(ctx, events.TopicPlanChanged, struct {
			UserID   string `json:"user_id"`
			Operator string `json:"operator,omitempty"`
		}{UserID: userID.String(), Operator: adminEmail})
	}
	return nil
}

func (s *UserService) AdminBanUser(ctx context.Context, adminID uuid.UUID, adminEmail string, userID uuid.UUID, reason string, ip string) error {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}

	oldStatus := user.Status
	if err := s.users.UpdateStatus(ctx, userID, model.UserStatusBanned); err != nil {
		return err
	}
	if err := s.tokens.RevokeAllForUser(ctx, userID); err != nil {
		s.logger.Error("failed to revoke tokens on ban", "user", userID, "error", err)
	}
	s.evictCachedRawToken(ctx, userID)

	actorType := model.ActorTypeAdmin
	s.writeAudit(ctx, actorType, &adminID, adminEmail, "ban_user", "user", &userID, ip, map[string]string{"status": string(oldStatus)}, map[string]string{"status": "banned", "reason": reason})

	type userEvt struct {
		UserID   string `json:"user_id"`
		Reason   string `json:"reason,omitempty"`
		Operator string `json:"operator,omitempty"`
	}
	s.onEvent(ctx, "user:banned", userEvt{UserID: userID.String(), Reason: reason, Operator: adminEmail})
	return nil
}

func (s *UserService) AdminUnbanUser(ctx context.Context, adminID uuid.UUID, adminEmail string, userID uuid.UUID, ip string) error {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}

	oldStatus := user.Status
	newStatus := model.UserStatusActive
	if user.EmailVerifiedAt == nil {
		newStatus = model.UserStatusPending
	}
	if err := s.users.UpdateStatus(ctx, userID, newStatus); err != nil {
		return err
	}

	actorType := model.ActorTypeAdmin
	s.writeAudit(ctx, actorType, &adminID, adminEmail, "unban_user", "user", &userID, ip, map[string]string{"status": string(oldStatus)}, map[string]string{"status": string(newStatus)})

	type userEvt struct {
		UserID   string `json:"user_id"`
		Operator string `json:"operator,omitempty"`
	}
	s.onEvent(ctx, "user:unbanned", userEvt{UserID: userID.String(), Operator: adminEmail})
	return nil
}

func (s *UserService) AdminResetPassword(ctx context.Context, adminID uuid.UUID, adminEmail string, userID uuid.UUID, ip string) (string, error) {
	newPassword := pkg.GenerateRandomString(12)
	passwordHash, err := pkg.HashPassword(newPassword, nil)
	if err != nil {
		return "", err
	}
	if err := s.users.UpdatePassword(ctx, userID, passwordHash); err != nil {
		return "", err
	}

	actorType := model.ActorTypeAdmin
	s.writeAudit(ctx, actorType, &adminID, adminEmail, "reset_password", "user", &userID, ip, nil, nil)
	return newPassword, nil
}

func (s *UserService) AdminResetTraffic(ctx context.Context, adminID uuid.UUID, adminEmail string, userID uuid.UUID, ip string) error {
	if err := s.subs.ResetTraffic(ctx, userID); err != nil {
		return err
	}
	actorType := model.ActorTypeAdmin
	s.writeAudit(ctx, actorType, &adminID, adminEmail, "reset_traffic", "subscription", &userID, ip, nil, nil)

	type trafficEvt struct {
		UserID   string `json:"user_id"`
		Operator string `json:"operator,omitempty"`
	}
	s.onEvent(ctx, "user:traffic_reset", trafficEvt{UserID: userID.String(), Operator: adminEmail})
	return nil
}

func (s *UserService) AdminAddTraffic(ctx context.Context, adminID uuid.UUID, adminEmail string, userID uuid.UUID, bytes int64, ip string) error {
	if err := s.subs.AddQuotaBytes(ctx, userID, bytes); err != nil {
		return err
	}
	actorType := model.ActorTypeAdmin
	s.writeAudit(ctx, actorType, &adminID, adminEmail, "add_traffic", "subscription", &userID, ip, nil, map[string]int64{"bytes": bytes})

	s.onEvent(ctx, events.TopicPlanChanged, struct {
		UserID   string `json:"user_id"`
		Bytes    int64  `json:"bytes"`
		Operator string `json:"operator,omitempty"`
	}{UserID: userID.String(), Bytes: bytes, Operator: adminEmail})
	return nil
}

func (s *UserService) AdminExtendSubscription(ctx context.Context, adminID uuid.UUID, adminEmail string, userID uuid.UUID, days int, ip string) error {
	sub, err := s.subs.GetActiveByUserID(ctx, userID)
	if err != nil {
		return err
	}
	if sub == nil {
		return errors.New("no active subscription")
	}
	if err := s.subs.ExtendByDays(ctx, sub.ID, days); err != nil {
		return err
	}
	actorType := model.ActorTypeAdmin
	s.writeAudit(ctx, actorType, &adminID, adminEmail, "extend_subscription", "subscription", &sub.ID, ip, nil, map[string]int{"days": days})

	s.onEvent(ctx, events.TopicPlanChanged, struct {
		UserID   string `json:"user_id"`
		Operator string `json:"operator,omitempty"`
	}{UserID: userID.String(), Operator: adminEmail})
	return nil
}

func (s *UserService) AdminChangePlan(ctx context.Context, adminID uuid.UUID, adminEmail string, userID uuid.UUID, planID uuid.UUID, immediate bool, ip string) error {
	plan, err := s.plans.GetByID(ctx, planID)
	if err != nil {
		return err
	}
	if plan == nil {
		return ErrPlanNotExist
	}

	existingSub, _ := s.subs.GetActiveByUserID(ctx, userID)

	if existingSub != nil {
		if immediate {
			if err := s.subs.MarkReplaced(ctx, existingSub.ID); err != nil {
				return err
			}
		} else {
			return errors.New("non-immediate change not supported yet")
		}
	}

	now := time.Now()
	newSub := &model.UserPlanSubscription{
		ID:                uuid.New(),
		UserID:            userID,
		PlanID:            planID,
		Status:            model.SubscriptionStatusActive,
		StartedAt:         &now,
		RenewalMode:       model.RenewalModeManual,
		TrafficQuotaBytes: plan.TrafficBytes,
		TrafficUsedBytes:  0,
		SpeedLimitMbps:    plan.SpeedLimitMbps,
		DeviceLimit:       plan.DeviceLimit,
		IPLimit:           plan.IPLimit,
		Source:            "admin_change",
	}
	if err := s.subs.Create(ctx, newSub); err != nil {
		return err
	}

	actorType := model.ActorTypeAdmin
	s.writeAudit(ctx, actorType, &adminID, adminEmail, "change_plan", "subscription", &newSub.ID, ip, nil, map[string]interface{}{"plan_id": planID, "immediate": immediate})

	type planEvt struct {
		UserID   string `json:"user_id"`
		PlanID   string `json:"plan_id"`
		Operator string `json:"operator,omitempty"`
	}
	s.onEvent(ctx, "user:plan_changed", planEvt{UserID: userID.String(), PlanID: planID.String(), Operator: adminEmail})
	return nil
}

func (s *UserService) AdminResetSubscriptionTokens(ctx context.Context, adminID uuid.UUID, adminEmail string, userID uuid.UUID, ip string) error {
	if err := s.tokens.RevokeAllForUser(ctx, userID); err != nil {
		return err
	}
	s.evictCachedRawToken(ctx, userID)
	if _, raw, err := s.CreateSubscriptionToken(ctx, userID, "reset", ip); err == nil {
		s.cacheRawToken(ctx, userID, raw)
	}
	actorType := model.ActorTypeAdmin
	s.writeAudit(ctx, actorType, &adminID, adminEmail, "reset_subscription_tokens", "user", &userID, ip, nil, nil)

	type tokenEvt struct {
		UserID   string `json:"user_id"`
		Reason   string `json:"reason,omitempty"`
		Operator string `json:"operator,omitempty"`
	}
	s.onEvent(ctx, "token:revoked", tokenEvt{UserID: userID.String(), Reason: "reset", Operator: adminEmail})
	return nil
}

func (s *UserService) AdminSoftDeleteUser(ctx context.Context, adminID uuid.UUID, adminEmail string, userID uuid.UUID, ip string) error {
	if err := s.users.SoftDelete(ctx, userID); err != nil {
		return err
	}
	if err := s.tokens.RevokeAllForUser(ctx, userID); err != nil {
		s.logger.Error("failed to revoke tokens on delete", "user", userID, "error", err)
	}
	s.evictCachedRawToken(ctx, userID)
	actorType := model.ActorTypeAdmin
	s.writeAudit(ctx, actorType, &adminID, adminEmail, "delete_user", "user", &userID, ip, nil, nil)
	return nil
}

func (s *UserService) AdminBatchBan(ctx context.Context, adminID uuid.UUID, adminEmail string, userIDs []uuid.UUID, reason string, ip string) error {
	if err := s.users.BatchUpdateStatus(ctx, userIDs, model.UserStatusBanned); err != nil {
		return err
	}
	for _, uid := range userIDs {
		_ = s.tokens.RevokeAllForUser(ctx, uid)
		s.evictCachedRawToken(ctx, uid)
	}
	actorType := model.ActorTypeAdmin
	s.writeAudit(ctx, actorType, &adminID, adminEmail, "batch_ban", "user", nil, ip, nil, map[string]interface{}{"user_ids": userIDs, "reason": reason})

	type userEvt struct {
		UserID   string `json:"user_id"`
		Reason   string `json:"reason,omitempty"`
		Operator string `json:"operator,omitempty"`
	}
	for _, uid := range userIDs {
		s.onEvent(ctx, "user:banned", userEvt{UserID: uid.String(), Reason: reason, Operator: adminEmail})
	}
	return nil
}

func (s *UserService) AdminBatchUnban(ctx context.Context, adminID uuid.UUID, adminEmail string, userIDs []uuid.UUID, ip string) error {
	if err := s.users.BatchUpdateStatus(ctx, userIDs, model.UserStatusActive); err != nil {
		return err
	}
	actorType := model.ActorTypeAdmin
	s.writeAudit(ctx, actorType, &adminID, adminEmail, "batch_unban", "user", nil, ip, nil, map[string]interface{}{"user_ids": userIDs})

	type userEvt struct {
		UserID   string `json:"user_id"`
		Operator string `json:"operator,omitempty"`
	}
	for _, uid := range userIDs {
		s.onEvent(ctx, "user:unbanned", userEvt{UserID: uid.String(), Operator: adminEmail})
	}
	return nil
}

func (s *UserService) AdminBatchResetTraffic(ctx context.Context, adminID uuid.UUID, adminEmail string, userIDs []uuid.UUID, ip string) error {
	for _, uid := range userIDs {
		_ = s.subs.ResetTraffic(ctx, uid)
	}
	actorType := model.ActorTypeAdmin
	s.writeAudit(ctx, actorType, &adminID, adminEmail, "batch_reset_traffic", "subscription", nil, ip, nil, map[string]interface{}{"user_ids": userIDs})

	type trafficEvt struct {
		UserID   string `json:"user_id"`
		Operator string `json:"operator,omitempty"`
	}
	for _, uid := range userIDs {
		s.onEvent(ctx, "user:traffic_reset", trafficEvt{UserID: uid.String(), Operator: adminEmail})
	}
	return nil
}

func (s *UserService) AdminBatchDelete(ctx context.Context, adminID uuid.UUID, adminEmail string, userIDs []uuid.UUID, ip string) error {
	if err := s.users.BatchSoftDelete(ctx, userIDs); err != nil {
		return err
	}
	for _, uid := range userIDs {
		_ = s.tokens.RevokeAllForUser(ctx, uid)
		s.evictCachedRawToken(ctx, uid)
	}
	actorType := model.ActorTypeAdmin
	s.writeAudit(ctx, actorType, &adminID, adminEmail, "batch_delete_user", "user", nil, ip, nil, map[string]interface{}{"user_ids": userIDs})
	return nil
}

func (s *UserService) AdminCreateImpersonateToken(ctx context.Context, adminID uuid.UUID, adminEmail string, userID uuid.UUID, ip string) (string, error) {
	settings, err := s.settings.GetByGroupKey(ctx, "security", "allow_impersonate")
	impersonateEnabled := true
	if err == nil && settings != nil {
		var enabled bool
		if json.Unmarshal(settings.ValueJSON, &enabled) == nil {
			impersonateEnabled = enabled
		}
	}
	if !impersonateEnabled {
		return "", ErrImpersonateDisabled
	}

	token := pkg.GenerateRandomString(64)
	if s.redisClient != nil {
		s.redisClient.Set(ctx, impersonatePrefix+token, userID.String(), impersonateTTL)
	}

	actorType := model.ActorTypeAdmin
	s.writeAudit(ctx, actorType, &adminID, adminEmail, "impersonate", "user", &userID, ip, nil, nil)
	return token, nil
}

func (s *UserService) getDefaultFreePlanID(ctx context.Context) (uuid.UUID, error) {
	setting, err := s.settings.GetByGroupKey(ctx, "billing", "default_free_plan_id")
	if err != nil || setting == nil {
		return uuid.Nil, nil
	}
	var idStr string
	if err := json.Unmarshal(setting.ValueJSON, &idStr); err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(idStr)
}

func (s *UserService) writeAudit(ctx context.Context, actorType model.ActorType, actorID *uuid.UUID, actorDisplay, action, resourceType string, resourceID *uuid.UUID, ip string, before, after interface{}) {
	if s.auditSvc == nil {
		return
	}
	display := actorDisplay
	_ = s.auditSvc.Write(ctx, actorType, actorID, &display, action, resourceType, resourceID, ip, "", "", before, after, nil)
}

func (s *UserService) GetSMTPConfig(ctx context.Context) (*SMTPConfig, error) {
	setting, err := s.settings.GetByGroupKey(ctx, "smtp", "config")
	if err != nil || setting == nil {
		return nil, nil
	}
	return ParseSMTPConfigFromJSON(setting.ValueJSON)
}

func ParseSMTPConfigFromJSON(data []byte) (*SMTPConfig, error) {
	var cfg SMTPConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	return &cfg, nil
}

func (s *UserService) RefreshMailConfig(ctx context.Context) {
	cfg, err := s.GetSMTPConfig(ctx)
	if err != nil {
		s.logger.Error("failed to load smtp config", "error", err)
		return
	}
	if s.mailSvc != nil {
		baseURL := "http://localhost:3000"
		siteName := "YunDu"
		urlSetting, err := s.settings.GetByGroupKey(ctx, "general", "frontend_url")
		if err == nil && urlSetting != nil {
			var url string
			if json.Unmarshal(urlSetting.ValueJSON, &url) == nil && url != "" {
				baseURL = url
			}
		}
		nameSetting, err := s.settings.GetByGroupKey(ctx, "general", "app_name")
		if err == nil && nameSetting != nil {
			var name string
			if json.Unmarshal(nameSetting.ValueJSON, &name) == nil && name != "" {
				siteName = name
			}
		}
		if cfg != nil {
			cfg.BaseURL = baseURL
		}
		s.mailSvc.UpdateConfig(cfg)
		s.mailSvc.UpdateSiteInfo(siteName, baseURL)
		// 重新加载邮件模板缓存
		if err := s.mailSvc.ReloadCache(ctx); err != nil {
			s.logger.Warn("failed to reload mail template cache", "error", err)
		}
	}
}

var ErrUserNotFound = fmt.Errorf("user not found")

var (
	ErrPlanNotFound     = fmt.Errorf("plan not found")
	ErrPlanNotActive    = fmt.Errorf("plan is not active")
	ErrInvalidPeriodCode = fmt.Errorf("invalid period code")
	ErrOrderNotFound    = fmt.Errorf("order not found")
	ErrOrderNotPending  = fmt.Errorf("order is not pending")
	ErrTRC20Disabled    = fmt.Errorf("USDT-TRC20 payment not enabled")
	ErrWechatDisabled   = fmt.Errorf("wechat payment not enabled")
	ErrAlipayDisabled   = fmt.Errorf("alipay payment not enabled")
	ErrCouponNotFound   = fmt.Errorf("coupon not found")
	ErrCouponExpired    = fmt.Errorf("coupon has expired")
	ErrCouponUsedUp     = fmt.Errorf("coupon has been used up")
	ErrCouponMinAmount  = fmt.Errorf("order amount does not meet coupon minimum")
	ErrCouponInvalid    = fmt.Errorf("coupon is invalid")
	ErrCouponPlanLimit  = fmt.Errorf("coupon not valid for this plan")
	ErrCouponNewUserOnly = fmt.Errorf("coupon is for new users only")
	ErrCouponCodeExists = fmt.Errorf("coupon code already exists")
)
