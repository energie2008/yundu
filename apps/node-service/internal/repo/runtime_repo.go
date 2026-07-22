package repo

import (
	"context"

	"github.com/airport-panel/node-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RuntimeRepo struct {
	pool *pgxpool.Pool
}

func NewRuntimeRepo(pool *pgxpool.Pool) *RuntimeRepo {
	return &RuntimeRepo{pool: pool}
}

func (r *RuntimeRepo) Create(ctx context.Context, rt *model.Runtime) error {
	query := `
		INSERT INTO runtimes (id, server_id, runtime_type, runtime_version, provider_type, provider_ref,
			listen_host, api_port, status, capabilities, config_schema_version, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		rt.ID, rt.ServerID, rt.RuntimeType, rt.RuntimeVersion, rt.ProviderType, rt.ProviderRef,
		rt.ListenHost, rt.APIPort, rt.Status, rt.Capabilities, rt.ConfigSchemaVersion, rt.Metadata,
	).Scan(&rt.CreatedAt, &rt.UpdatedAt)
}

func (r *RuntimeRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.Runtime, error) {
	query := `
		SELECT id, server_id, runtime_type, runtime_version, provider_type, provider_ref,
			listen_host, api_port, status, capabilities, config_schema_version, metadata,
			last_heartbeat_at, created_at, updated_at
		FROM runtimes WHERE id = $1`
	rt := &model.Runtime{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&rt.ID, &rt.ServerID, &rt.RuntimeType, &rt.RuntimeVersion, &rt.ProviderType, &rt.ProviderRef,
		&rt.ListenHost, &rt.APIPort, &rt.Status, &rt.Capabilities, &rt.ConfigSchemaVersion, &rt.Metadata,
		&rt.LastHeartbeatAt, &rt.CreatedAt, &rt.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return rt, nil
}

func (r *RuntimeRepo) GetByServerAndProvider(ctx context.Context, serverID uuid.UUID, providerType model.RuntimeProviderType, providerRef *string) (*model.Runtime, error) {
	var query string
	var args []interface{}

	if providerRef != nil {
		query = `
			SELECT id, server_id, runtime_type, runtime_version, provider_type, provider_ref,
				listen_host, api_port, status, capabilities, config_schema_version, metadata,
				last_heartbeat_at, created_at, updated_at
			FROM runtimes WHERE server_id = $1 AND provider_type = $2 AND provider_ref = $3`
		args = []interface{}{serverID, string(providerType), providerRef}
	} else {
		query = `
			SELECT id, server_id, runtime_type, runtime_version, provider_type, provider_ref,
				listen_host, api_port, status, capabilities, config_schema_version, metadata,
				last_heartbeat_at, created_at, updated_at
			FROM runtimes WHERE server_id = $1 AND provider_type = $2 AND provider_ref IS NULL`
		args = []interface{}{serverID, string(providerType)}
	}

	rt := &model.Runtime{}
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&rt.ID, &rt.ServerID, &rt.RuntimeType, &rt.RuntimeVersion, &rt.ProviderType, &rt.ProviderRef,
		&rt.ListenHost, &rt.APIPort, &rt.Status, &rt.Capabilities, &rt.ConfigSchemaVersion, &rt.Metadata,
		&rt.LastHeartbeatAt, &rt.CreatedAt, &rt.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return rt, nil
}

func (r *RuntimeRepo) ListByServer(ctx context.Context, serverID uuid.UUID) ([]*model.Runtime, error) {
	query := `
		SELECT id, server_id, runtime_type, runtime_version, provider_type, provider_ref,
			listen_host, api_port, status, capabilities, config_schema_version, metadata,
			last_heartbeat_at, created_at, updated_at
		FROM runtimes WHERE server_id = $1 ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, query, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runtimes []*model.Runtime
	for rows.Next() {
		rt := &model.Runtime{}
		err := rows.Scan(
			&rt.ID, &rt.ServerID, &rt.RuntimeType, &rt.RuntimeVersion, &rt.ProviderType, &rt.ProviderRef,
			&rt.ListenHost, &rt.APIPort, &rt.Status, &rt.Capabilities, &rt.ConfigSchemaVersion, &rt.Metadata,
			&rt.LastHeartbeatAt, &rt.CreatedAt, &rt.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		runtimes = append(runtimes, rt)
	}
	return runtimes, rows.Err()
}

func (r *RuntimeRepo) UpdateHeartbeat(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE runtimes SET last_heartbeat_at = now(), status = $2, updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, model.RuntimeStatusActive)
	return err
}

func (r *RuntimeRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status model.RuntimeStatus) error {
	query := `UPDATE runtimes SET status = $2, updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, status)
	return err
}

func (r *RuntimeRepo) Update(ctx context.Context, rt *model.Runtime) error {
	query := `
		UPDATE runtimes SET
			runtime_type = $2,
			runtime_version = $3,
			provider_ref = $4,
			listen_host = $5,
			api_port = $6,
			status = $7,
			capabilities = $8,
			config_schema_version = $9,
			metadata = $10,
			updated_at = now()
		WHERE id = $1
		RETURNING updated_at`
	return r.pool.QueryRow(ctx, query,
		rt.ID, rt.RuntimeType, rt.RuntimeVersion, rt.ProviderRef,
		rt.ListenHost, rt.APIPort, rt.Status,
		rt.Capabilities, rt.ConfigSchemaVersion, rt.Metadata,
	).Scan(&rt.UpdatedAt)
}

// ActiveRuntimeServer 表示一个活跃的 runtime 及其所属 server 信息
type ActiveRuntimeServer struct {
	RuntimeID   uuid.UUID
	ServerID    uuid.UUID
	ServerCode  string
	RuntimeType string
}

// ListActiveRuntimeServers 获取所有有活跃心跳的 runtime-server 对（用于批量推送）
// 判断条件：runtime.last_heartbeat_at 在 5 分钟内
func (r *RuntimeRepo) ListActiveRuntimeServers(ctx context.Context) ([]*ActiveRuntimeServer, error) {
	query := `
		SELECT rt.id, s.id, s.code, rt.runtime_type
		FROM runtimes rt
		JOIN servers s ON s.id = rt.server_id AND s.deleted_at IS NULL
		WHERE rt.last_heartbeat_at > NOW() - INTERVAL '5 minutes'
		  AND rt.status = 'active'
		ORDER BY s.code ASC`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*ActiveRuntimeServer
	for rows.Next() {
		var (
			runtimeID, serverID uuid.UUID
			serverCode, rtType  string
		)
		if err := rows.Scan(&runtimeID, &serverID, &serverCode, &rtType); err != nil {
			continue
		}
		result = append(result, &ActiveRuntimeServer{
			RuntimeID:   runtimeID,
			ServerID:    serverID,
			ServerCode:  serverCode,
			RuntimeType: rtType,
		})
	}
	return result, rows.Err()
}
