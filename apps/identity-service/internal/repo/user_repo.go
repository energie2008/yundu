package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserRepo struct {
	pool *pgxpool.Pool
}

func NewUserRepo(pool *pgxpool.Pool) *UserRepo {
	return &UserRepo{pool: pool}
}

const userColumns = `id, email, username, password_hash, password_algo, status, uuid,
	email_verified_at, telegram_chat_id, inviter_id,
	commission_balance, commission_total,
	notify_expiry, notify_traffic, notify_ticket_reply,
	registered_at,
	locale, timezone, last_login_at, last_login_ip::text, last_seen_at,
	notes, group_id, created_at, updated_at, deleted_at`

func scanUser(row pgx.Row, u *model.User) error {
	return row.Scan(
		&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.PasswordAlgo, &u.Status, &u.UUID,
		&u.EmailVerifiedAt, &u.TelegramChatID, &u.InviterID,
		&u.CommissionBalance, &u.CommissionTotal,
		&u.NotifyExpiry, &u.NotifyTraffic, &u.NotifyTicketReply,
		&u.RegisteredAt,
		&u.Locale, &u.Timezone, &u.LastLoginAt, &u.LastLoginIP, &u.LastSeenAt,
		&u.Notes, &u.GroupID, &u.CreatedAt, &u.UpdatedAt, &u.DeletedAt,
	)
}

func (r *UserRepo) Create(ctx context.Context, user *model.User) error {
	query := `
		INSERT INTO users (id, email, username, password_hash, password_algo, status, uuid, locale, timezone, inviter_id,
			notify_expiry, notify_traffic, notify_ticket_reply, registered_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING created_at, updated_at, uuid`
	return r.pool.QueryRow(ctx, query,
		user.ID,
		user.Email,
		user.Username,
		user.PasswordHash,
		user.PasswordAlgo,
		user.Status,
		uuid.New().String(),
		user.Locale,
		user.Timezone,
		user.InviterID,
		user.NotifyExpiry,
		user.NotifyTraffic,
		user.NotifyTicketReply,
		user.RegisteredAt,
	).Scan(&user.CreatedAt, &user.UpdatedAt, &user.UUID)
}

func (r *UserRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE id = $1 AND deleted_at IS NULL`
	u := &model.User{}
	err := scanUser(r.pool.QueryRow(ctx, query, id), u)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return u, nil
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE email = $1 AND deleted_at IS NULL`
	u := &model.User{}
	err := scanUser(r.pool.QueryRow(ctx, query, email), u)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return u, nil
}

func (r *UserRepo) Update(ctx context.Context, user *model.User) error {
	query := `
		UPDATE users SET
			email = $2, username = $3, password_hash = $4, status = $5,
			email_verified_at = $6, telegram_chat_id = $7, locale = $8, timezone = $9,
			last_login_at = $10, last_login_ip = $11, last_seen_at = $12,
			notes = $13, group_id = $14, updated_at = now()
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, query,
		user.ID, user.Email, user.Username, user.PasswordHash, user.Status,
		user.EmailVerifiedAt, user.TelegramChatID, user.Locale, user.Timezone,
		user.LastLoginAt, user.LastLoginIP, user.LastSeenAt, user.Notes, user.GroupID,
	)
	return err
}

// UpdateGroupID 更新用户的会员分组ID（购买套餐时自动赋值 plan.group_id）
// groupID 为 nil 时表示清除分组绑定（回到全量节点）
func (r *UserRepo) UpdateGroupID(ctx context.Context, userID uuid.UUID, groupID *uuid.UUID) error {
	query := `UPDATE users SET group_id = $2, updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, userID, groupID)
	return err
}

func (r *UserRepo) List(ctx context.Context, page, pageSize int, status, search string) ([]*model.User, int, error) {
	var where []string
	var args []interface{}
	argIdx := 1

	where = append(where, "deleted_at IS NULL")
	if status != "" {
		where = append(where, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, status)
		argIdx++
	}
	if search != "" {
		where = append(where, fmt.Sprintf("(email ILIKE $%d OR username ILIKE $%d)", argIdx, argIdx))
		args = append(args, "%"+search+"%")
		argIdx++
	}

	whereClause := strings.Join(where, " AND ")
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM users WHERE %s", whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`SELECT `+userColumns+` FROM users WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var users []*model.User
	for rows.Next() {
		u := &model.User{}
		if err := scanUser(rows, u); err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}

// ListInvitedByUser 分页查询被某用户邀请注册的用户列表（按注册/创建时间倒序）。
func (r *UserRepo) ListInvitedByUser(ctx context.Context, inviterID uuid.UUID, page, pageSize int) ([]*model.User, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	var total int
	countQuery := `SELECT COUNT(*) FROM users WHERE inviter_id = $1 AND deleted_at IS NULL`
	if err := r.pool.QueryRow(ctx, countQuery, inviterID).Scan(&total); err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []*model.User{}, 0, nil
	}
	offset := (page - 1) * pageSize
	query := `SELECT ` + userColumns + ` FROM users WHERE inviter_id = $1 AND deleted_at IS NULL ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, query, inviterID, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	users := make([]*model.User, 0)
	for rows.Next() {
		u := &model.User{}
		if err := scanUser(rows, u); err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}

func (r *UserRepo) UpdateLastLogin(ctx context.Context, id uuid.UUID, ip string) error {
	query := `UPDATE users SET last_login_at = now(), last_login_ip = $2, last_seen_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, ip)
	return err
}

func (r *UserRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status model.UserStatus) error {
	query := `UPDATE users SET status = $2, updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, status)
	return err
}

func (r *UserRepo) UpdateNotes(ctx context.Context, id uuid.UUID, notes *string) error {
	query := `UPDATE users SET notes = $2, updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, notes)
	return err
}

func (r *UserRepo) UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	query := `UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, passwordHash)
	return err
}

func (r *UserRepo) UpdateEmail(ctx context.Context, id uuid.UUID, email string) error {
	query := `UPDATE users SET email = $2, updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, email)
	return err
}

func (r *UserRepo) UpdateEmailVerified(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE users SET email_verified_at = now(), status = 'active', updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

func (r *UserRepo) SoftDelete(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE users SET deleted_at = now(), status = 'disabled', updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

func (r *UserRepo) FindBySubscriptionTokenHash(ctx context.Context, tokenHash string) (*model.User, error) {
	query := `
		SELECT u.id, u.email, u.username, u.password_hash, u.password_algo, u.status, u.uuid,
		       u.email_verified_at, u.telegram_chat_id, u.inviter_id,
		       u.commission_balance, u.commission_total,
		       u.notify_expiry, u.notify_traffic, u.notify_ticket_reply,
		       u.registered_at,
		       u.locale, u.timezone, u.last_login_at, u.last_login_ip::text, u.last_seen_at,
		       u.notes, u.group_id, u.created_at, u.updated_at, u.deleted_at
		FROM users u
		JOIN subscription_tokens st ON st.user_id = u.id
		WHERE st.token_hash = $1 AND st.status = 'active' AND u.deleted_at IS NULL`
	u := &model.User{}
	err := scanUser(r.pool.QueryRow(ctx, query, tokenHash), u)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return u, nil
}

func (r *UserRepo) GetSubscription(ctx context.Context, userID uuid.UUID) (*model.UserSubscription, error) {
	query := `
		SELECT ups.id, ups.user_id, ups.plan_id, p.name as plan_name, ups.status,
		       COALESCE(ups.started_at, ups.created_at) as started_at, ups.expires_at,
		       COALESCE(ups.traffic_quota_bytes, p.traffic_bytes) as traffic_quota_bytes,
		       COALESCE(ups.traffic_used_bytes, 0) as traffic_used_bytes,
		       COALESCE(ups.upload_bytes, 0) as upload_bytes,
		       COALESCE(ups.download_bytes, 0) as download_bytes,
		       COALESCE(ups.speed_limit_mbps, p.speed_limit_mbps, 0) as speed_limit_mbps,
		       COALESCE(ups.device_limit, p.device_limit, 0) as device_limit,
		       ups.reset_at
		FROM user_plan_subscriptions ups
		JOIN plans p ON p.id = ups.plan_id
		WHERE ups.user_id = $1 AND ups.status = 'active' AND ups.deleted_at IS NULL
		  AND (ups.expires_at IS NULL OR ups.expires_at > now())
		ORDER BY ups.created_at DESC LIMIT 1`
	sub := &model.UserSubscription{}
	var speedLimit, deviceLimit *int
	err := r.pool.QueryRow(ctx, query, userID).Scan(
		&sub.ID, &sub.UserID, &sub.PlanID, &sub.PlanName, &sub.Status,
		&sub.StartedAt, &sub.ExpiresAt,
		&sub.TrafficQuotaBytes, &sub.TrafficUsedBytes, &sub.UploadBytes, &sub.DownloadBytes,
		&speedLimit, &deviceLimit, &sub.ResetAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if speedLimit != nil {
		sub.SpeedLimitMbps = *speedLimit
	}
	if deviceLimit != nil {
		sub.DeviceLimit = *deviceLimit
	}
	return sub, nil
}

func (r *UserRepo) GetByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]*model.User, error) {
	if len(ids) == 0 {
		return make(map[uuid.UUID]*model.User), nil
	}
	query := `SELECT ` + userColumns + ` FROM users WHERE id = ANY($1) AND deleted_at IS NULL`
	rows, err := r.pool.Query(ctx, query, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[uuid.UUID]*model.User)
	for rows.Next() {
		u := &model.User{}
		if err := scanUser(rows, u); err != nil {
			return nil, err
		}
		result[u.ID] = u
	}
	return result, rows.Err()
}

func (r *UserRepo) BatchUpdateStatus(ctx context.Context, ids []uuid.UUID, status model.UserStatus) error {
	if len(ids) == 0 {
		return nil
	}
	query := `UPDATE users SET status = $2, updated_at = now() WHERE id = ANY($1)`
	_, err := r.pool.Exec(ctx, query, ids, status)
	return err
}

func (r *UserRepo) BatchSoftDelete(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	query := `UPDATE users SET deleted_at = now(), status = 'banned', updated_at = now() WHERE id = ANY($1) AND deleted_at IS NULL`
	_, err := r.pool.Exec(ctx, query, ids)
	return err
}

func (r *UserRepo) UpdateCommission(ctx context.Context, userID uuid.UUID, balance, total float64) error {
	query := `UPDATE users SET commission_balance = $2, commission_total = $3, updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, userID, balance, total)
	return err
}

func (r *UserRepo) GetNotificationPreferences(ctx context.Context, userID uuid.UUID) (*model.NotificationPreferences, error) {
	query := `SELECT notify_expiry, notify_traffic, notify_ticket_reply FROM users WHERE id = $1 AND deleted_at IS NULL`
	prefs := &model.NotificationPreferences{}
	err := r.pool.QueryRow(ctx, query, userID).Scan(&prefs.NotifyExpiry, &prefs.NotifyTraffic, &prefs.NotifyTicketReply)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return prefs, nil
}

func (r *UserRepo) UpdateNotificationPreferences(ctx context.Context, userID uuid.UUID, prefs *model.NotificationPreferences) error {
	query := `UPDATE users SET notify_expiry = $2, notify_traffic = $3, notify_ticket_reply = $4, updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, userID, prefs.NotifyExpiry, prefs.NotifyTraffic, prefs.NotifyTicketReply)
	return err
}

func (r *UserRepo) DeductCommissionBalance(ctx context.Context, userID uuid.UUID, amount float64) error {
	query := `UPDATE users SET commission_balance = commission_balance - $2, updated_at = now() WHERE id = $1 AND commission_balance >= $2`
	tag, err := r.pool.Exec(ctx, query, userID, amount)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("insufficient commission balance")
	}
	return nil
}

// RefundCommissionBalance adds amount back to the user's commission balance (used when a withdrawal is rejected).
func (r *UserRepo) RefundCommissionBalance(ctx context.Context, userID uuid.UUID, amount float64) error {
	query := `UPDATE users SET commission_balance = commission_balance + $2, updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, userID, amount)
	return err
}

func (r *UserRepo) CountInvitedByUser(ctx context.Context, userID uuid.UUID) (int, error) {
	query := `SELECT COUNT(*) FROM users WHERE inviter_id = $1 AND deleted_at IS NULL`
	var count int
	err := r.pool.QueryRow(ctx, query, userID).Scan(&count)
	return count, err
}

// GetUUIDByID 返回用户的代理协议 UUID（全节点共享）
func (r *UserRepo) GetUUIDByID(ctx context.Context, userID uuid.UUID) (string, error) {
	var u string
	err := r.pool.QueryRow(ctx, `SELECT uuid FROM users WHERE id = $1 AND deleted_at IS NULL`, userID).Scan(&u)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return u, nil
}
