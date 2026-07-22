package service

import (
	"context"
	"errors"
	"sync"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/repo"
	"github.com/google/uuid"
)

var (
	ErrPermissionDenied = errors.New("permission denied")
)

type RBACService struct {
	roleRepo  *repo.RoleRepo
	permCache sync.Map
}

func NewRBACService(roleRepo *repo.RoleRepo) *RBACService {
	return &RBACService{
		roleRepo: roleRepo,
	}
}

func (s *RBACService) HasPermission(ctx context.Context, adminID uuid.UUID, isSuperAdmin bool, permissionCode string) (bool, error) {
	if isSuperAdmin {
		return true, nil
	}

	if cached, ok := s.permCache.Load(adminID); ok {
		if perms, ok := cached.(map[string]bool); ok {
			return perms[permissionCode], nil
		}
	}

	perms, err := s.roleRepo.GetPermissionsForAdmin(ctx, adminID, isSuperAdmin)
	if err != nil {
		return false, err
	}

	permMap := make(map[string]bool)
	for _, p := range perms {
		permMap[p.Code] = true
	}
	s.permCache.Store(adminID, permMap)
	return permMap[permissionCode], nil
}

func (s *RBACService) RequirePermission(ctx context.Context, adminID uuid.UUID, isSuperAdmin bool, permissionCode string) error {
	has, err := s.HasPermission(ctx, adminID, isSuperAdmin, permissionCode)
	if err != nil {
		return err
	}
	if !has {
		return ErrPermissionDenied
	}
	return nil
}

func (s *RBACService) InvalidateCache(adminID uuid.UUID) {
	s.permCache.Delete(adminID)
}

func (s *RBACService) GetAdminPermissions(ctx context.Context, adminID uuid.UUID, isSuperAdmin bool) ([]*model.Permission, error) {
	return s.roleRepo.GetPermissionsForAdmin(ctx, adminID, isSuperAdmin)
}
