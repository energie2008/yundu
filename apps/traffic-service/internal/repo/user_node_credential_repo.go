package repo

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UserNodeCredential 用户凭证（对齐 XBoard 模型）
// 每用户一个 UUID，全节点共享。用于流量统计时按 UUID 反查用户。
type UserNodeCredential struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	NodeID          uuid.UUID
	CredentialType  string
	CredentialValue string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type UserNodeCredentialRepo struct {
	pool *pgxpool.Pool
}

func NewUserNodeCredentialRepo(pool *pgxpool.Pool) *UserNodeCredentialRepo {
	return &UserNodeCredentialRepo{pool: pool}
}

// GetByCredentialValue 按 UUID、email 或 user ID 反查用户。
// node-agent 上报流量时携带凭证（user.uuid 或 email 或 user.id 字符串），通过此方法关联到具体用户。
// 查询顺序：uuid → email → id::text（兜底：xray config 中 email 字段存的是 user.id 字符串）。
// 未匹配时返回 (nil, nil)。
func (r *UserNodeCredentialRepo) GetByCredentialValue(ctx context.Context, credValue string) (*UserNodeCredential, error) {
	if credValue == "" {
		return nil, nil
	}
	// 直接从 users 表按 uuid、email 或 id::text 查询
	// id::text 兜底：xray config 的 email 字段实际存的是 user.id 字符串（非真实邮箱），
	// 当 extractEmailUUIDMap 未能提取 UUID 时，credential 会退化为 email 字段值（即 user.id）
	query := `SELECT id, uuid, created_at, updated_at
              FROM users WHERE (uuid = $1 OR email = $1 OR id::text = $1) AND deleted_at IS NULL LIMIT 1`
	row := r.pool.QueryRow(ctx, query, credValue)
	var (
		userID    uuid.UUID
		uuidValue string
		createdAt time.Time
		updatedAt time.Time
	)
	err := row.Scan(&userID, &uuidValue, &createdAt, &updatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &UserNodeCredential{
		ID:              userID,
		UserID:          userID,
		NodeID:          uuid.Nil, // 不再使用 node_id（全节点共享）
		CredentialType:  "uuid",
		CredentialValue: uuidValue,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
	}, nil
}

// GetByNodeID 获取所有活跃用户的凭证（对齐 XBoard：全节点共享同一组用户 UUID）
// nodeID 参数保留用于接口兼容，实际查询不依赖它
func (r *UserNodeCredentialRepo) GetByNodeID(ctx context.Context, nodeID uuid.UUID) ([]*UserNodeCredential, error) {
	query := `SELECT id, uuid, created_at, updated_at
              FROM users WHERE deleted_at IS NULL AND status = 'active'
                AND uuid IS NOT NULL AND uuid != ''
              ORDER BY created_at ASC`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var creds []*UserNodeCredential
	for rows.Next() {
		var (
			userID    uuid.UUID
			uuidValue string
			createdAt time.Time
			updatedAt time.Time
		)
		if err := rows.Scan(&userID, &uuidValue, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		creds = append(creds, &UserNodeCredential{
			ID:              userID,
			UserID:          userID,
			NodeID:          nodeID,
			CredentialType:  "uuid",
			CredentialValue: uuidValue,
			CreatedAt:       createdAt,
			UpdatedAt:       updatedAt,
		})
	}
	return creds, rows.Err()
}
