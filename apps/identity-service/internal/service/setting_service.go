package service

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/repo"
	"github.com/google/uuid"
)

var (
	ErrSettingNotFound = errors.New("setting not found")
)

type SettingService struct {
	settingRepo *repo.SettingRepo
}

func NewSettingService(settingRepo *repo.SettingRepo) *SettingService {
	return &SettingService{settingRepo: settingRepo}
}

func (s *SettingService) GetSettings(ctx context.Context, group string) ([]*model.SystemSettingResponse, error) {
	var settings []*model.SystemSetting
	var err error
	if group != "" {
		settings, err = s.settingRepo.ListByGroup(ctx, group)
	} else {
		settings, err = s.settingRepo.ListAll(ctx)
	}
	if err != nil {
		return nil, err
	}

	var result []*model.SystemSettingResponse
	for _, st := range settings {
		resp := &model.SystemSettingResponse{
			SettingGroup: st.SettingGroup,
			SettingKey:   st.SettingKey,
			IsSecret:     st.IsSecret,
			Description:  st.Description,
			UpdatedAt:    st.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
		var value interface{}
		if err := json.Unmarshal(st.ValueJSON, &value); err != nil {
			resp.Value = string(st.ValueJSON)
		} else {
			if st.IsSecret {
				resp.Value = "********"
			} else {
				resp.Value = value
			}
		}
		result = append(result, resp)
	}
	return result, nil
}

func (s *SettingService) UpdateSetting(ctx context.Context, group, key string, value interface{}, updatedBy *uuid.UUID) (*model.SystemSettingResponse, error) {
	existing, err := s.settingRepo.GetByGroupKey(ctx, group, key)
	if err != nil {
		return nil, err
	}

	isSecret := false
	var description *string
	if existing != nil {
		isSecret = existing.IsSecret
		description = existing.Description
	}

	st, err := s.settingRepo.SetByGroupKey(ctx, group, key, value, isSecret, description, updatedBy)
	if err != nil {
		return nil, err
	}

	resp := &model.SystemSettingResponse{
		SettingGroup: st.SettingGroup,
		SettingKey:   st.SettingKey,
		IsSecret:     st.IsSecret,
		Description:  st.Description,
		UpdatedAt:    st.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	var val interface{}
	if err := json.Unmarshal(st.ValueJSON, &val); err != nil {
		resp.Value = string(st.ValueJSON)
	} else {
		if st.IsSecret {
			resp.Value = "********"
		} else {
			resp.Value = val
		}
	}
	return resp, nil
}

func (s *SettingService) GetSettingValue(ctx context.Context, group, key string) (interface{}, error) {
	st, err := s.settingRepo.GetByGroupKey(ctx, group, key)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, ErrSettingNotFound
	}
	var value interface{}
	if err := json.Unmarshal(st.ValueJSON, &value); err != nil {
		return string(st.ValueJSON), nil
	}
	return value, nil
}
