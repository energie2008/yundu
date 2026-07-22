package repo

import (
	"context"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AdminRepo struct {
	pool *pgxpool.Pool
}

func NewAdminRepo(pool *pgxpool.Pool) *AdminRepo {
	return &AdminRepo{pool: pool}
}

func (r *AdminRepo) Create(ctx context.Context, admin *model.Admin) error {
	query := `
		INSERT INTO admins (id, user_id, display_name, status, is_super_admin)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		admin.ID, admin.UserID, admin.DisplayName, admin.Status, admin.IsSuperAdmin,
	).Scan(&admin.CreatedAt, &admin.UpdatedAt)
}

func (r *AdminRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.Admin, error) {
	query := `
		SELECT id, user_id, display_name, status, is_super_admin,
		       last_login_at, last_login_ip::text, created_at, updated_at, deleted_at
		FROM admins WHERE id = $1 AND deleted_at IS NULL`
	a := &model.Admin{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&a.ID, &a.UserID, &a.DisplayName, &a.Status, &a.IsSuperAdmin,
		&a.LastLoginAt, &a.LastLoginIP, &a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return a, nil
}

func (r *AdminRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*model.Admin, error) {
	query := `
		SELECT id, user_id, display_name, status, is_super_admin,
		       last_login_at, last_login_ip::text, created_at, updated_at, deleted_at
		FROM admins WHERE user_id = $1 AND deleted_at IS NULL`
	a := &model.Admin{}
	err := r.pool.QueryRow(ctx, query, userID).Scan(
		&a.ID, &a.UserID, &a.DisplayName, &a.Status, &a.IsSuperAdmin,
		&a.LastLoginAt, &a.LastLoginIP, &a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return a, nil
}

func (r *AdminRepo) GetByEmail(ctx context.Context, email string) (*model.Admin, *model.User, error) {
	query := `
		SELECT a.id, a.user_id, a.display_name, a.status, a.is_super_admin,
		       a.last_login_at, a.last_login_ip::text, a.created_at, a.updated_at, a.deleted_at,
		       u.id, u.email, u.username, u.password_hash, u.password_algo, u.status,
		       u.email_verified_at, u.locale, u.timezone, u.created_at, u.updated_at
		FROM admins a
		JOIN users u ON u.id = a.user_id AND u.deleted_at IS NULL
		WHERE u.email = $1 AND a.deleted_at IS NULL`
	a := &model.Admin{}
	u := &model.User{}
	err := r.pool.QueryRow(ctx, query, email).Scan(
		&a.ID, &a.UserID, &a.DisplayName, &a.Status, &a.IsSuperAdmin,
		&a.LastLoginAt, &a.LastLoginIP, &a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
		&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.PasswordAlgo, &u.Status,
		&u.EmailVerifiedAt, &u.Locale, &u.Timezone, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	return a, u, nil
}

func (r *AdminRepo) List(ctx context.Context, page, pageSize int) ([]*model.Admin, []*model.User, int, error) {
	countQuery := `SELECT COUNT(*) FROM admins WHERE deleted_at IS NULL`
	var total int
	if err := r.pool.QueryRow(ctx, countQuery).Scan(&total); err != nil {
		return nil, nil, 0, err
	}

	query := `
		SELECT a.id, a.user_id, a.display_name, a.status, a.is_super_admin,
		       a.last_login_at, a.last_login_ip::text, a.created_at, a.updated_at,
		       u.id, u.email, u.username, u.status, u.created_at
		FROM admins a
		JOIN users u ON u.id = a.user_id AND u.deleted_at IS NULL
		WHERE a.deleted_at IS NULL
		ORDER BY a.created_at DESC
		LIMIT $1 OFFSET $2`
	rows, err := r.pool.Query(ctx, query, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, nil, 0, err
	}
	defer rows.Close()

	var admins []*model.Admin
	var users []*model.User
	for rows.Next() {
		a := &model.Admin{}
		u := &model.User{}
		err := rows.Scan(
			&a.ID, &a.UserID, &a.DisplayName, &a.Status, &a.IsSuperAdmin,
			&a.LastLoginAt, &a.LastLoginIP, &a.CreatedAt, &a.UpdatedAt,
			&u.ID, &u.Email, &u.Username, &u.Status, &u.CreatedAt,
		)
		if err != nil {
			return nil, nil, 0, err
		}
		admins = append(admins, a)
		users = append(users, u)
	}
	return admins, users, total, rows.Err()
}

func (r *AdminRepo) UpdateLastLogin(ctx context.Context, id uuid.UUID, ip string) error {
	query := `UPDATE admins SET last_login_at = now(), last_login_ip = $2 WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, ip)
	return err
}

func (r *AdminRepo) AssignRole(ctx context.Context, adminID, roleID uuid.UUID) error {
	query := `INSERT INTO admin_roles (admin_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
	_, err := r.pool.Exec(ctx, query, adminID, roleID)
	return err
}

func (r *AdminRepo) GetRoleIDsForAdmin(ctx context.Context, adminID uuid.UUID) ([]uuid.UUID, error) {
	query := `SELECT role_id FROM admin_roles WHERE admin_id = $1`
	rows, err := r.pool.Query(ctx, query, adminID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roleIDs []uuid.UUID
	for rows.Next() {
		var rid uuid.UUID
		if err := rows.Scan(&rid); err != nil {
			return nil, err
		}
		roleIDs = append(roleIDs, rid)
	}
	return roleIDs, rows.Err()
}

// GetPermissionsForAdmin 返回某个 admin 通过 admin_roles → role_permissions → permissions
// 关联得到的所有权限 code（去重）。调用方负责处理 super_admin 的 ["*"] 兜底。
func (r *AdminRepo) GetPermissionsForAdmin(ctx context.Context, adminID uuid.UUID) ([]string, error) {
	query := `
		SELECT DISTINCT p.code
		FROM admin_roles ar
		JOIN role_permissions rp ON rp.role_id = ar.role_id
		JOIN permissions p ON p.id = rp.permission_id
		WHERE ar.admin_id = $1`
	rows, err := r.pool.Query(ctx, query, adminID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var perms []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		perms = append(perms, code)
	}
	return perms, rows.Err()
}
