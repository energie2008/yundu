package service

import (
	"context"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/pkg"
	"github.com/airport-panel/identity-service/internal/repo"
	"github.com/google/uuid"
)

type AdminService struct {
	userRepo  *repo.UserRepo
	adminRepo *repo.AdminRepo
}

func NewAdminService(userRepo *repo.UserRepo, adminRepo *repo.AdminRepo) *AdminService {
	return &AdminService{
		userRepo:  userRepo,
		adminRepo: adminRepo,
	}
}

func (s *AdminService) CreateAdmin(ctx context.Context, req *model.CreateAdminRequest) (*model.Admin, *model.User, error) {
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

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, nil, err
	}

	admin := &model.Admin{
		ID:           uuid.New(),
		UserID:       user.ID,
		DisplayName:  req.DisplayName,
		Status:       model.AdminStatusActive,
		IsSuperAdmin: req.IsSuperAdmin,
	}
	if err := s.adminRepo.Create(ctx, admin); err != nil {
		return nil, nil, err
	}

	return admin, user, nil
}

func (s *AdminService) ListAdmins(ctx context.Context, page, pageSize int) ([]model.AdminResponse, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	admins, users, total, err := s.adminRepo.List(ctx, page, pageSize)
	if err != nil {
		return nil, 0, err
	}

	userMap := make(map[uuid.UUID]*model.User)
	for _, u := range users {
		userMap[u.ID] = u
	}

	var result []model.AdminResponse
	for _, a := range admins {
		email := ""
		if u, ok := userMap[a.UserID]; ok {
			email = u.Email
		}
		result = append(result, model.NewAdminResponse(a, email))
	}
	return result, total, nil
}

func (s *AdminService) ListUsers(ctx context.Context, page, pageSize int, status, search string) ([]model.UserResponse, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	users, total, err := s.userRepo.List(ctx, page, pageSize, status, search)
	if err != nil {
		return nil, 0, err
	}

	var result []model.UserResponse
	for _, u := range users {
		result = append(result, model.NewUserResponse(u))
	}
	return result, total, nil
}
