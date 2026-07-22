package repo

import (
	"context"
	"encoding/json"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SettingRepo struct {
	pool *pgxpool.Pool
}

func NewSettingRepo(pool *pgxpool.Pool) *SettingRepo {
	return &SettingRepo{pool: pool}
}

func (r *SettingRepo) GetByGroupKey(ctx context.Context, group, key string) (*model.SystemSetting, error) {
	query := `
		SELECT id, setting_group, setting_key, value_json, is_secret, description, updated_by_admin_id, updated_at
		FROM system_settings WHERE setting_group = $1 AND setting_key = $2`
	s := &model.SystemSetting{}
	err := r.pool.QueryRow(ctx, query, group, key).Scan(
		&s.ID, &s.SettingGroup, &s.SettingKey, &s.ValueJSON, &s.IsSecret,
		&s.Description, &s.UpdatedByAdminID, &s.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return s, nil
}

func (r *SettingRepo) SetByGroupKey(ctx context.Context, group, key string, value interface{}, isSecret bool, description *string, updatedBy *uuid.UUID) (*model.SystemSetting, error) {
	valueJSON, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	query := `
		INSERT INTO system_settings (id, setting_group, setting_key, value_json, is_secret, description, updated_by_admin_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (setting_group, setting_key)
		DO UPDATE SET value_json = EXCLUDED.value_json, is_secret = EXCLUDED.is_secret,
		              description = EXCLUDED.description, updated_by_admin_id = EXCLUDED.updated_by_admin_id,
		              updated_at = now()
		RETURNING id, setting_group, setting_key, value_json, is_secret, description, updated_by_admin_id, updated_at`
	s := &model.SystemSetting{}
	id := uuid.New()
	err = r.pool.QueryRow(ctx, query, id, group, key, valueJSON, isSecret, description, updatedBy).Scan(
		&s.ID, &s.SettingGroup, &s.SettingKey, &s.ValueJSON, &s.IsSecret,
		&s.Description, &s.UpdatedByAdminID, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *SettingRepo) ListByGroup(ctx context.Context, group string) ([]*model.SystemSetting, error) {
	query := `
		SELECT id, setting_group, setting_key, value_json, is_secret, description, updated_by_admin_id, updated_at
		FROM system_settings WHERE setting_group = $1 ORDER BY setting_key`
	rows, err := r.pool.Query(ctx, query, group)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var settings []*model.SystemSetting
	for rows.Next() {
		s := &model.SystemSetting{}
		if err := rows.Scan(
			&s.ID, &s.SettingGroup, &s.SettingKey, &s.ValueJSON, &s.IsSecret,
			&s.Description, &s.UpdatedByAdminID, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		settings = append(settings, s)
	}
	return settings, rows.Err()
}

func (r *SettingRepo) ListAll(ctx context.Context) ([]*model.SystemSetting, error) {
	query := `
		SELECT id, setting_group, setting_key, value_json, is_secret, description, updated_by_admin_id, updated_at
		FROM system_settings ORDER BY setting_group, setting_key`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var settings []*model.SystemSetting
	for rows.Next() {
		s := &model.SystemSetting{}
		if err := rows.Scan(
			&s.ID, &s.SettingGroup, &s.SettingKey, &s.ValueJSON, &s.IsSecret,
			&s.Description, &s.UpdatedByAdminID, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		settings = append(settings, s)
	}
	return settings, rows.Err()
}

func (r *SettingRepo) GetJSON(ctx context.Context, group, key string) ([]byte, error) {
	query := `SELECT value_json FROM system_settings WHERE setting_group = $1 AND setting_key = $2`
	var data []byte
	err := r.pool.QueryRow(ctx, query, group, key).Scan(&data)
	if err != nil {
		return nil, err
	}
	return data, nil
}
