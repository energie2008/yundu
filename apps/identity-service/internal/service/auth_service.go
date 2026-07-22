package service

import (
	"context"
	"errors"
	"net"

	"github.com/airport-panel/config"
	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/pkg"
	"github.com/airport-panel/identity-service/internal/repo"
	"github.com/google/uuid"
)

var (
	ErrUserAlreadyExists  = errors.New("user already exists")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserDisabled       = errors.New("user is disabled")
	ErrSessionNotFound    = errors.New("session not found")
	ErrAdminNotFound      = errors.New("admin not found")
	ErrAdminDisabled      = errors.New("admin is disabled")
)

type AuthService struct {
	userRepo   *repo.UserRepo
	adminRepo  *repo.AdminRepo
	authRepo   *repo.AuthRepo
	jwtManager *pkg.JWTManager
}

func NewAuthService(userRepo *repo.UserRepo, adminRepo *repo.AdminRepo, authRepo *repo.AuthRepo, jwtManager *pkg.JWTManager) *AuthService {
	return &AuthService{
		userRepo:   userRepo,
		adminRepo:  adminRepo,
		authRepo:   authRepo,
		jwtManager: jwtManager,
	}
}

func (s *AuthService) Register(ctx context.Context, req *model.RegisterRequest, ip string) (*model.TokenResponse, *model.User, error) {
	existing, err := s.userRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		return nil, nil, err
	}
	if existing != nil {
		return nil, nil, ErrUserAlreadyExists
	}

	passwordHash, err := pkg.HashPassword(req.Password, nil)
	if err != nil {
		return nil, nil, err
	}

	user := &model.User{
		ID:           uuid.New(),
		Email:        req.Email,
		PasswordAlgo: "argon2id",
		Status:       model.UserStatusActive,
		Locale:       "zh-CN",
		Timezone:     "Asia/Shanghai",
	}
	hashStr := passwordHash
	user.PasswordHash = &hashStr
	if req.Username != "" {
		user.Username = &req.Username
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, nil, err
	}

	ipStr := extractIP(ip)
	if err := s.userRepo.UpdateLastLogin(ctx, user.ID, ipStr); err != nil {
		return nil, nil, err
	}

	tokenResp, err := s.createSession(ctx, user.ID, uuid.Nil, false, nil, "", ipStr)
	if err != nil {
		return nil, nil, err
	}

	return tokenResp, user, nil
}

func (s *AuthService) Login(ctx context.Context, req *model.LoginRequest, userAgent, ip string) (*model.TokenResponse, *model.User, error) {
	user, err := s.userRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		return nil, nil, err
	}
	if user == nil {
		return nil, nil, ErrInvalidCredentials
	}
	if user.Status == model.UserStatusDisabled || user.Status == model.UserStatusBanned {
		return nil, nil, ErrUserDisabled
	}
	if user.Status == model.UserStatusPending {
		return nil, nil, ErrUserPending
	}
	if user.PasswordHash == nil {
		return nil, nil, ErrInvalidCredentials
	}

	valid, err := pkg.VerifyPassword(req.Password, *user.PasswordHash)
	if err != nil || !valid {
		return nil, nil, ErrInvalidCredentials
	}

	ipStr := extractIP(ip)
	if err := s.userRepo.UpdateLastLogin(ctx, user.ID, ipStr); err != nil {
		return nil, nil, err
	}

	tokenResp, err := s.createSession(ctx, user.ID, uuid.Nil, false, nil, userAgent, ipStr)
	if err != nil {
		return nil, nil, err
	}

	return tokenResp, user, nil
}

func (s *AuthService) AdminLogin(ctx context.Context, req *model.AdminLoginRequest, userAgent, ip string) (*model.TokenResponse, *model.Admin, *model.User, error) {
	admin, user, err := s.adminRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		return nil, nil, nil, err
	}
	if admin == nil {
		return nil, nil, nil, ErrInvalidCredentials
	}
	if admin.Status == model.AdminStatusDisabled {
		return nil, nil, nil, ErrAdminDisabled
	}
	if user.PasswordHash == nil {
		return nil, nil, nil, ErrInvalidCredentials
	}

	valid, err := pkg.VerifyPassword(req.Password, *user.PasswordHash)
	if err != nil || !valid {
		return nil, nil, nil, ErrInvalidCredentials
	}

	ipStr := extractIP(ip)
	if err := s.adminRepo.UpdateLastLogin(ctx, admin.ID, ipStr); err != nil {
		return nil, nil, nil, err
	}

	// 查询 admin 的权限列表：super_admin 直接 ["*"]，否则从 admin_roles 关联查询
	perms, err := s.resolveAdminPermissions(ctx, admin)
	if err != nil {
		return nil, nil, nil, err
	}

	tokenResp, err := s.createSession(ctx, user.ID, admin.ID, true, perms, userAgent, ipStr)
	if err != nil {
		return nil, nil, nil, err
	}

	return tokenResp, admin, user, nil
}

// resolveAdminPermissions 返回 admin 的权限 code 列表：
//   - super_admin：返回 ["*"]（拥有全部权限，由 RequirePermission 中间件短路放行）
//   - 普通 admin：通过 admin_roles → role_permissions → permissions 关联查询去重后的 code 列表
func (s *AuthService) resolveAdminPermissions(ctx context.Context, admin *model.Admin) ([]string, error) {
	if admin.IsSuperAdmin {
		return []string{"*"}, nil
	}
	perms, err := s.adminRepo.GetPermissionsForAdmin(ctx, admin.ID)
	if err != nil {
		return nil, err
	}
	if perms == nil {
		// 避免下游 c.Set(permissions, nil) 把空 slice 当成"未设置"
		perms = []string{}
	}
	return perms, nil
}

func (s *AuthService) Logout(ctx context.Context, sessionID uuid.UUID) error {
	return s.authRepo.RevokeSession(ctx, sessionID)
}

func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*model.TokenResponse, error) {
	claims, err := s.jwtManager.ParseToken(refreshToken)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	if claims.TokenType != pkg.TokenTypeRefresh {
		return nil, ErrInvalidCredentials
	}

	refreshTokenID, err := uuid.Parse(claims.ID)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	session, err := s.authRepo.GetSessionByRefreshToken(ctx, refreshTokenID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, ErrSessionNotFound
	}

	if err := s.authRepo.RevokeSession(ctx, session.ID); err != nil {
		return nil, err
	}

	// 刷新时重新解析 admin 权限：若为 admin，从 DB 查询最新权限列表（避免角色变更后旧 token 残留）
	var adminID uuid.UUID
	var perms []string
	if claims.IsAdmin {
		admin, err := s.adminRepo.GetByUserID(ctx, claims.UserID)
		if err != nil {
			return nil, err
		}
		if admin == nil {
			return nil, ErrAdminNotFound
		}
		adminID = admin.ID
		perms, err = s.resolveAdminPermissions(ctx, admin)
		if err != nil {
			return nil, err
		}
	}

	tokenResp, err := s.createSessionFromClaims(ctx, claims.UserID, adminID, claims.IsAdmin, perms, session.UserAgent, session.IPAddress)
	if err != nil {
		return nil, err
	}

	return tokenResp, nil
}

func (s *AuthService) GetMe(ctx context.Context, userID uuid.UUID) (*model.User, error) {
	return s.userRepo.GetByID(ctx, userID)
}

func (s *AuthService) GetAdminMe(ctx context.Context, userID uuid.UUID) (*model.Admin, *model.User, error) {
	admin, err := s.adminRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	if admin == nil {
		return nil, nil, ErrAdminNotFound
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	return admin, user, nil
}

func (s *AuthService) ValidateSession(ctx context.Context, sessionID uuid.UUID) (*model.AuthSession, error) {
	return s.authRepo.GetSessionByID(ctx, sessionID)
}

func (s *AuthService) createSession(ctx context.Context, userID, adminID uuid.UUID, isAdmin bool, permissions []string, userAgent, ip string) (*model.TokenResponse, error) {
	sessionID := uuid.New()
	accessToken, accessExpiresAt, err := s.jwtManager.GenerateAccessToken(userID, sessionID, adminID, isAdmin, permissions)
	if err != nil {
		return nil, err
	}

	refreshToken, refreshTokenIDStr, refreshExpiresAt, err := s.jwtManager.GenerateRefreshToken(userID, sessionID, adminID, isAdmin, permissions)
	if err != nil {
		return nil, err
	}

	refreshTokenID, err := uuid.Parse(refreshTokenIDStr)
	if err != nil {
		return nil, err
	}

	sessionType := model.SessionTypeWeb
	session := &model.AuthSession{
		ID:             sessionID,
		UserID:         userID,
		SessionType:    sessionType,
		TokenID:        uuid.New(),
		RefreshTokenID: &refreshTokenID,
		ExpiresAt:      refreshExpiresAt,
	}
	if userAgent != "" {
		session.UserAgent = &userAgent
	}
	ipStr := extractIP(ip)
	if ipStr != "" {
		session.IPAddress = &ipStr
	}

	if err := s.authRepo.CreateSession(ctx, session); err != nil {
		return nil, err
	}

	return &model.TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(accessExpiresAt.Sub(accessExpiresAt.Add(-s.jwtManager.AccessTTL())).Seconds()),
	}, nil
}

func (s *AuthService) createSessionFromClaims(ctx context.Context, userID, adminID uuid.UUID, isAdmin bool, permissions []string, userAgent *string, ipAddr *string) (*model.TokenResponse, error) {
	sessionID := uuid.New()
	accessToken, accessExpiresAt, err := s.jwtManager.GenerateAccessToken(userID, sessionID, adminID, isAdmin, permissions)
	if err != nil {
		return nil, err
	}

	refreshToken, refreshTokenIDStr, refreshExpiresAt, err := s.jwtManager.GenerateRefreshToken(userID, sessionID, adminID, isAdmin, permissions)
	if err != nil {
		return nil, err
	}

	refreshTokenID, err := uuid.Parse(refreshTokenIDStr)
	if err != nil {
		return nil, err
	}

	session := &model.AuthSession{
		ID:             sessionID,
		UserID:         userID,
		SessionType:    model.SessionTypeWeb,
		TokenID:        uuid.New(),
		RefreshTokenID: &refreshTokenID,
		ExpiresAt:      refreshExpiresAt,
		UserAgent:      userAgent,
		IPAddress:      ipAddr,
	}

	if err := s.authRepo.CreateSession(ctx, session); err != nil {
		return nil, err
	}

	ttl := s.jwtManager.AccessTTL()
	_ = accessExpiresAt
	return &model.TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(ttl.Seconds()),
	}, nil
}

func MapErrorToCode(err error) (config.ErrorCode, string) {
	switch {
	case errors.Is(err, ErrUserAlreadyExists):
		return config.CodeConflict, err.Error()
	case errors.Is(err, ErrInvalidCredentials):
		return config.CodeUnauthorized, "invalid email or password"
	case errors.Is(err, ErrUserDisabled):
		return config.CodeForbidden, err.Error()
	case errors.Is(err, ErrAdminNotFound):
		return config.CodeUnauthorized, "unauthorized"
	case errors.Is(err, ErrAdminDisabled):
		return config.CodeForbidden, err.Error()
	case errors.Is(err, ErrSessionNotFound):
		return config.CodeUnauthorized, "session expired or invalid"
	case errors.Is(err, ErrInvalidVerifyToken):
		return config.CodeBadRequest, err.Error()
	case errors.Is(err, ErrInvalidResetToken):
		return config.CodeBadRequest, err.Error()
	case errors.Is(err, ErrUserBanned):
		return config.CodeForbidden, err.Error()
	case errors.Is(err, ErrUserPending):
		return config.CodeForbidden, err.Error()
	case errors.Is(err, ErrUserNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrTokenNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrPlanNotExist):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrOrderNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrImpersonateDisabled):
		return config.CodeForbidden, err.Error()
	case errors.Is(err, ErrPlanNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrPlanNotActive):
		return config.CodeForbidden, err.Error()
	case errors.Is(err, ErrInvalidPeriodCode):
		return config.CodeBadRequest, err.Error()
	case errors.Is(err, ErrOrderNotPending):
		return config.CodeConflict, err.Error()
	case errors.Is(err, ErrTRC20Disabled):
		return config.CodeForbidden, err.Error()
	case errors.Is(err, ErrCouponNotFound):
		return config.CodeBadRequest, err.Error()
	case errors.Is(err, ErrCouponExpired):
		return config.CodeBadRequest, err.Error()
	case errors.Is(err, ErrCouponUsedUp):
		return config.CodeBadRequest, err.Error()
	case errors.Is(err, ErrCouponNewUserOnly):
		return config.CodeBadRequest, err.Error()
	case errors.Is(err, ErrWithdrawDisabled):
		return config.CodeForbidden, err.Error()
	case errors.Is(err, ErrWithdrawMinAmount):
		return config.CodeBadRequest, err.Error()
	case errors.Is(err, ErrInsufficientBalance):
		return config.CodeBadRequest, err.Error()
	case errors.Is(err, ErrTicketClosed):
		return config.CodeBadRequest, err.Error()
	case errors.Is(err, ErrTicketNotFound):
		return config.CodeNotFound, err.Error()
	default:
		return config.CodeInternalError, ""
	}
}

func extractIP(ipStr string) string {
	if ipStr == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(ipStr)
	if err != nil {
		return ipStr
	}
	return host
}
