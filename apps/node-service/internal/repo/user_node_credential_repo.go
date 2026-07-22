package repo

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UserNodeCredential 用户凭证（对齐 XBoard 模型）
// 每用户一个 UUID，全节点共享。NodeID 字段保留用于兼容下游接口，但实际查询不依赖它
type UserNodeCredential struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	NodeID          uuid.UUID
	Email           string
	CredentialType  string
	CredentialValue string
	SpeedLimitMbps  int // 从 subscription/plan JOIN 获取（per-user 限速，0=不限）
	DeviceLimit     int // 从 subscription/plan JOIN 获取（per-user 设备限制，0=不限）
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// UserNodeCredentialRepo 处理用户凭证数据访问
// 改造后从 users 表查询 UUID（全节点共享），不再使用 user_node_credentials 关联表
type UserNodeCredentialRepo struct {
	pool *pgxpool.Pool
}

func NewUserNodeCredentialRepo(pool *pgxpool.Pool) *UserNodeCredentialRepo {
	return &UserNodeCredentialRepo{pool: pool}
}

// GetByNodeID 获取所有活跃且有有效订阅的用户凭证（对齐 XBoard：全节点共享同一组用户 UUID）
// nodeID 参数保留用于接口兼容，实际查询不依赖它（所有节点返回相同的用户列表）
// 凭证类型统一为 "uuid"（XBoard 模型：所有协议都用 user.uuid，SS2022 派生在订阅渲染层处理）
// 过滤条件：
//   - 用户 status='active' 且未删除
//   - 存在有效的 user_plan_subscriptions（status='active'，未过期）
//   - 流量未超额（traffic_quota_bytes=0 表示不限，或 traffic_used_bytes < traffic_quota_bytes）
func (r *UserNodeCredentialRepo) GetByNodeID(ctx context.Context, nodeID uuid.UUID) ([]*UserNodeCredential, error) {
	query := `
		SELECT u.id, u.email, u.uuid, u.created_at, u.updated_at,
			COALESCE(ups.speed_limit_mbps, p.speed_limit_mbps, 0) AS speed_limit_mbps,
			COALESCE(ups.device_limit, p.device_limit, 0) AS device_limit
		FROM users u
		INNER JOIN user_plan_subscriptions ups ON ups.user_id = u.id
			AND ups.status = 'active'
			AND ups.deleted_at IS NULL
			AND (ups.expires_at IS NULL OR ups.expires_at > now())
			AND (ups.traffic_quota_bytes = 0 OR ups.traffic_used_bytes < ups.traffic_quota_bytes)
		LEFT JOIN plans p ON p.id = ups.plan_id AND p.deleted_at IS NULL
		WHERE u.deleted_at IS NULL
		  AND u.status = 'active'
		  AND u.uuid IS NOT NULL AND u.uuid != ''
		ORDER BY u.created_at ASC`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var creds []*UserNodeCredential
	for rows.Next() {
		var (
			userID         uuid.UUID
			email          string
			uuidValue      string
			createdAt      time.Time
			updatedAt      time.Time
			speedLimitMbps int
			deviceLimit    int
		)
		if err := rows.Scan(&userID, &email, &uuidValue, &createdAt, &updatedAt, &speedLimitMbps, &deviceLimit); err != nil {
			continue
		}
		creds = append(creds, &UserNodeCredential{
			ID:              userID,
			UserID:          userID,
			NodeID:          nodeID,
			Email:           email,
			CredentialType:  "uuid",
			CredentialValue: uuidValue,
			SpeedLimitMbps:  speedLimitMbps,
			DeviceLimit:     deviceLimit,
			CreatedAt:       createdAt,
			UpdatedAt:       updatedAt,
		})
	}
	return creds, rows.Err()
}

// GetByUserID 获取单个用户的凭证信息（用于事件处理时的增量推送）
// 如果用户不存在、已删除、已封禁、无有效订阅、订阅已过期或流量已超额，返回 (nil, nil)
func (r *UserNodeCredentialRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*UserNodeCredential, error) {
	query := `
		SELECT u.id, u.email, u.uuid, u.created_at, u.updated_at,
			COALESCE(ups.speed_limit_mbps, p.speed_limit_mbps, 0) AS speed_limit_mbps,
			COALESCE(ups.device_limit, p.device_limit, 0) AS device_limit
		FROM users u
		INNER JOIN user_plan_subscriptions ups ON ups.user_id = u.id
			AND ups.status = 'active'
			AND ups.deleted_at IS NULL
			AND (ups.expires_at IS NULL OR ups.expires_at > now())
			AND (ups.traffic_quota_bytes = 0 OR ups.traffic_used_bytes < ups.traffic_quota_bytes)
		LEFT JOIN plans p ON p.id = ups.plan_id AND p.deleted_at IS NULL
		WHERE u.id = $1 AND u.deleted_at IS NULL AND u.status = 'active'
		  AND u.uuid IS NOT NULL AND u.uuid != ''`
	var (
		email          string
		uuidValue      string
		createdAt      time.Time
		updatedAt      time.Time
		speedLimitMbps int
		deviceLimit    int
	)
	err := r.pool.QueryRow(ctx, query, userID).Scan(&userID, &email, &uuidValue, &createdAt, &updatedAt, &speedLimitMbps, &deviceLimit)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &UserNodeCredential{
		ID:              userID,
		UserID:          userID,
		Email:           email,
		CredentialType:  "uuid",
		CredentialValue: uuidValue,
		SpeedLimitMbps:  speedLimitMbps,
		DeviceLimit:     deviceLimit,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
	}, nil
}
