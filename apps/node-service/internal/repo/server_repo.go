package repo

import (
	"context"
	"fmt"
	"strings"

	"github.com/airport-panel/node-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ServerRepo struct {
	pool *pgxpool.Pool
}

func NewServerRepo(pool *pgxpool.Pool) *ServerRepo {
	return &ServerRepo{pool: pool}
}

func (r *ServerRepo) Create(ctx context.Context, s *model.Server) error {
	query := `
		INSERT INTO servers (id, code, name, region_id, provider, host, ipv4, ipv6, ssh_port, os_name, os_version, arch, status, role, labels, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		RETURNING created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		s.ID, s.Code, s.Name, s.RegionID, s.Provider, s.Host, s.IPv4, s.IPv6,
		s.SSHPort, s.OSName, s.OSVersion, s.Arch, s.Status, s.Role, s.Labels, s.Metadata,
	).Scan(&s.CreatedAt, &s.UpdatedAt)
}

func (r *ServerRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.Server, error) {
	query := `
		SELECT id, code, name, region_id, provider, host, ipv4::text, ipv6::text, ssh_port, os_name, os_version, arch,
		       status, role, labels, metadata, last_heartbeat_at, created_at, updated_at, deleted_at
		FROM servers WHERE id = $1 AND deleted_at IS NULL`
	s := &model.Server{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&s.ID, &s.Code, &s.Name, &s.RegionID, &s.Provider, &s.Host, &s.IPv4, &s.IPv6,
		&s.SSHPort, &s.OSName, &s.OSVersion, &s.Arch, &s.Status, &s.Role, &s.Labels, &s.Metadata,
		&s.LastHeartbeatAt, &s.CreatedAt, &s.UpdatedAt, &s.DeletedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return s, nil
}

func (r *ServerRepo) GetByCode(ctx context.Context, code string) (*model.Server, error) {
	query := `
		SELECT id, code, name, region_id, provider, host, ipv4::text, ipv6::text, ssh_port, os_name, os_version, arch,
		       status, role, labels, metadata, last_heartbeat_at, created_at, updated_at, deleted_at
		FROM servers WHERE code = $1 AND deleted_at IS NULL`
	s := &model.Server{}
	err := r.pool.QueryRow(ctx, query, code).Scan(
		&s.ID, &s.Code, &s.Name, &s.RegionID, &s.Provider, &s.Host, &s.IPv4, &s.IPv6,
		&s.SSHPort, &s.OSName, &s.OSVersion, &s.Arch, &s.Status, &s.Role, &s.Labels, &s.Metadata,
		&s.LastHeartbeatAt, &s.CreatedAt, &s.UpdatedAt, &s.DeletedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return s, nil
}

func (r *ServerRepo) UpdateHeartbeat(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE servers SET last_heartbeat_at = now(), status = $2, updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, model.ServerStatusActive)
	return err
}

func (r *ServerRepo) Update(ctx context.Context, s *model.Server) error {
	query := `
		UPDATE servers SET
			name = $2, region_id = $3, provider = $4, host = $5, ipv4 = $6, ipv6 = $7,
			ssh_port = $8, os_name = $9, os_version = $10, arch = $11, status = $12, role = $13,
			labels = $14, metadata = $15, updated_at = now()
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, query,
		s.ID, s.Name, s.RegionID, s.Provider, s.Host, s.IPv4, s.IPv6,
		s.SSHPort, s.OSName, s.OSVersion, s.Arch, s.Status, s.Role, s.Labels, s.Metadata,
	)
	return err
}

func (r *ServerRepo) List(ctx context.Context, page, pageSize int, status model.ServerStatus, search string) ([]*model.Server, int, error) {
	var where []string
	var args []interface{}
	argIdx := 1

	where = append(where, "deleted_at IS NULL")
	if status != "" {
		where = append(where, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, string(status))
		argIdx++
	}
	if search != "" {
		where = append(where, fmt.Sprintf("(code ILIKE $%d OR name ILIKE $%d OR host ILIKE $%d)", argIdx, argIdx, argIdx))
		args = append(args, "%"+search+"%")
		argIdx++
	}

	whereClause := strings.Join(where, " AND ")
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM servers WHERE %s", whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`
		SELECT id, code, name, region_id, provider, host, ipv4::text, ipv6::text, ssh_port, os_name, os_version, arch,
		       status, role, labels, metadata, last_heartbeat_at, created_at, updated_at, deleted_at
		FROM servers WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var servers []*model.Server
	for rows.Next() {
		s := &model.Server{}
		err := rows.Scan(
			&s.ID, &s.Code, &s.Name, &s.RegionID, &s.Provider, &s.Host, &s.IPv4, &s.IPv6,
			&s.SSHPort, &s.OSName, &s.OSVersion, &s.Arch, &s.Status, &s.Role, &s.Labels, &s.Metadata,
			&s.LastHeartbeatAt, &s.CreatedAt, &s.UpdatedAt, &s.DeletedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		servers = append(servers, s)
	}
	return servers, total, rows.Err()
}
