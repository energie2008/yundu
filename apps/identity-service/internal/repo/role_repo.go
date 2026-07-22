package repo

import (
	"context"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RoleRepo struct {
	pool *pgxpool.Pool
}

func NewRoleRepo(pool *pgxpool.Pool) *RoleRepo {
	return &RoleRepo{pool: pool}
}

func (r *RoleRepo) GetRoleByID(ctx context.Context, id uuid.UUID) (*model.Role, error) {
	query := `SELECT id, code, name, description, created_at, updated_at FROM roles WHERE id = $1`
	role := &model.Role{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&role.ID, &role.Code, &role.Name, &role.Description, &role.CreatedAt, &role.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return role, nil
}

func (r *RoleRepo) ListRoles(ctx context.Context) ([]*model.Role, error) {
	query := `SELECT id, code, name, description, created_at, updated_at FROM roles ORDER BY code`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roles []*model.Role
	for rows.Next() {
		role := &model.Role{}
		if err := rows.Scan(&role.ID, &role.Code, &role.Name, &role.Description, &role.CreatedAt, &role.UpdatedAt); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (r *RoleRepo) GetPermissionsForAdmin(ctx context.Context, adminID uuid.UUID, isSuperAdmin bool) ([]*model.Permission, error) {
	if isSuperAdmin {
		query := `SELECT id, code, name, resource, action, created_at FROM permissions`
		rows, err := r.pool.Query(ctx, query)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var perms []*model.Permission
		for rows.Next() {
			p := &model.Permission{}
			if err := rows.Scan(&p.ID, &p.Code, &p.Name, &p.Resource, &p.Action, &p.CreatedAt); err != nil {
				return nil, err
			}
			perms = append(perms, p)
		}
		return perms, rows.Err()
	}

	query := `
		SELECT DISTINCT p.id, p.code, p.name, p.resource, p.action, p.created_at
		FROM permissions p
		JOIN role_permissions rp ON rp.permission_id = p.id
		JOIN admin_roles ar ON ar.role_id = rp.role_id
		WHERE ar.admin_id = $1
		ORDER BY p.code`
	rows, err := r.pool.Query(ctx, query, adminID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var perms []*model.Permission
	for rows.Next() {
		p := &model.Permission{}
		if err := rows.Scan(&p.ID, &p.Code, &p.Name, &p.Resource, &p.Action, &p.CreatedAt); err != nil {
			return nil, err
		}
		perms = append(perms, p)
	}
	return perms, rows.Err()
}
