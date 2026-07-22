package repo

import (
	"context"
	"time"

	"github.com/airport-panel/traffic-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	OnlineUserKeyPrefix = "online:"
	OnlineTTL           = 5 * time.Minute
)

type SessionRepo struct {
	pool *pgxpool.Pool
}

func NewSessionRepo(pool *pgxpool.Pool) *SessionRepo {
	return &SessionRepo{pool: pool}
}

func (r *SessionRepo) StartSession(ctx context.Context, userID uuid.UUID, nodeID, runtimeID, credentialID *uuid.UUID, clientIP, clientType *string) (*model.OnlineSession, error) {
	session := &model.OnlineSession{
		ID:           uuid.New(),
		UserID:       userID,
		CredentialID: credentialID,
		NodeID:       nodeID,
		RuntimeID:    runtimeID,
		ClientIP:     clientIP,
		ClientType:   clientType,
		ConnectedAt:  time.Now(),
		LastSeenAt:   time.Now(),
		Status:       model.OnlineSessionStatusOnline,
	}

	query := `
		INSERT INTO online_sessions (id, user_id, credential_id, node_id, runtime_id, client_ip, client_type, connected_at, last_seen_at, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING connected_at, last_seen_at`
	err := r.pool.QueryRow(ctx, query,
		session.ID, session.UserID, session.CredentialID, session.NodeID, session.RuntimeID,
		session.ClientIP, session.ClientType, session.ConnectedAt, session.LastSeenAt, session.Status,
	).Scan(&session.ConnectedAt, &session.LastSeenAt)
	if err != nil {
		return nil, err
	}
	return session, nil
}

func (r *SessionRepo) EndSession(ctx context.Context, sessionID uuid.UUID) error {
	now := time.Now()
	query := `UPDATE online_sessions SET status = $2, disconnected_at = $3, last_seen_at = $3 WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, sessionID, model.OnlineSessionStatusOffline, now)
	return err
}

func (r *SessionRepo) EndStaleSessions(ctx context.Context, staleTimeout time.Duration) error {
	cutoff := time.Now().Add(-staleTimeout)
	query := `UPDATE online_sessions SET status = $2, disconnected_at = now() WHERE status = $3 AND last_seen_at < $4`
	_, err := r.pool.Exec(ctx, query, model.OnlineSessionStatusOffline, model.OnlineSessionStatusOnline, cutoff)
	return err
}

func (r *SessionRepo) GetActiveSessions(ctx context.Context) ([]*model.OnlineSession, error) {
	query := `
		SELECT id, user_id, credential_id, node_id, runtime_id, client_ip::text, client_type, connected_at, last_seen_at, disconnected_at, status
		FROM online_sessions
		WHERE status = 'online' AND (disconnected_at IS NULL)
		ORDER BY last_seen_at DESC`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*model.OnlineSession
	for rows.Next() {
		s := &model.OnlineSession{}
		err := rows.Scan(
			&s.ID, &s.UserID, &s.CredentialID, &s.NodeID, &s.RuntimeID,
			&s.ClientIP, &s.ClientType, &s.ConnectedAt, &s.LastSeenAt, &s.DisconnectedAt, &s.Status,
		)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

func (r *SessionRepo) GetActiveSessionsByUser(ctx context.Context, userID uuid.UUID) ([]*model.OnlineSession, error) {
	query := `
		SELECT id, user_id, credential_id, node_id, runtime_id, client_ip::text, client_type, connected_at, last_seen_at, disconnected_at, status
		FROM online_sessions
		WHERE user_id = $1 AND status = 'online' AND (disconnected_at IS NULL)
		ORDER BY last_seen_at DESC`
	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*model.OnlineSession
	for rows.Next() {
		s := &model.OnlineSession{}
		err := rows.Scan(
			&s.ID, &s.UserID, &s.CredentialID, &s.NodeID, &s.RuntimeID,
			&s.ClientIP, &s.ClientType, &s.ConnectedAt, &s.LastSeenAt, &s.DisconnectedAt, &s.Status,
		)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

func (r *SessionRepo) UpdateHeartbeat(ctx context.Context, sessionID uuid.UUID) error {
	query := `UPDATE online_sessions SET last_seen_at = now() WHERE id = $1 AND status = 'online'`
	_, err := r.pool.Exec(ctx, query, sessionID)
	return err
}

func (r *SessionRepo) GetSessionByID(ctx context.Context, id uuid.UUID) (*model.OnlineSession, error) {
	query := `
		SELECT id, user_id, credential_id, node_id, runtime_id, client_ip::text, client_type, connected_at, last_seen_at, disconnected_at, status
		FROM online_sessions WHERE id = $1`
	s := &model.OnlineSession{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&s.ID, &s.UserID, &s.CredentialID, &s.NodeID, &s.RuntimeID,
		&s.ClientIP, &s.ClientType, &s.ConnectedAt, &s.LastSeenAt, &s.DisconnectedAt, &s.Status,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return s, nil
}
