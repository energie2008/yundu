package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"

	"github.com/airport-panel/config"
	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/repo"
	"github.com/google/uuid"
)

var (
	ErrPlanCodeExists  = errors.New("plan code already exists")
	ErrPlanCodeInvalid = errors.New("套餐编码只能包含小写字母、数字和连字符，且不能以连字符开头或结尾")
)

// planCodeRegex 套餐编码格式：小写字母+数字+连字符，不以连字符开头/结尾
// 与前端 slugify 输出对齐：^[a-z0-9]+(-[a-z0-9]+)*$
var planCodeRegex = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

type PlanService struct {
	planRepo *repo.PlanRepo
	logger   *slog.Logger
}

func NewPlanService(planRepo *repo.PlanRepo, logger *slog.Logger) *PlanService {
	return &PlanService{planRepo: planRepo, logger: logger}
}

func (s *PlanService) GetByID(ctx context.Context, id uuid.UUID) (*model.Plan, error) {
	p, err := s.planRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrPlanNotFound
	}
	prices, err := s.planRepo.GetPrices(ctx, id)
	if err != nil {
		s.logger.Warn("failed to get plan prices", "error", err)
	} else {
		p.Prices = prices
	}
	return p, nil
}

func (s *PlanService) GetByCode(ctx context.Context, code string) (*model.Plan, error) {
	p, err := s.planRepo.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrPlanNotFound
	}
	prices, err := s.planRepo.GetPrices(ctx, p.ID)
	if err != nil {
		s.logger.Warn("failed to get plan prices", "error", err)
	} else {
		p.Prices = prices
	}
	return p, nil
}

func (s *PlanService) List(ctx context.Context, page, pageSize int, query model.PlanListQuery) ([]*model.Plan, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	items, total, err := s.planRepo.List(ctx, page, pageSize, query)
	if err != nil {
		return nil, 0, err
	}
	for _, p := range items {
		prices, err := s.planRepo.GetPrices(ctx, p.ID)
		if err != nil {
			s.logger.Warn("failed to get plan prices", "error", err)
		} else {
			p.Prices = prices
		}
	}
	return items, total, nil
}

func (s *PlanService) ListActive(ctx context.Context) ([]*model.Plan, error) {
	items, err := s.planRepo.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range items {
		prices, err := s.planRepo.GetPrices(ctx, p.ID)
		if err != nil {
			s.logger.Warn("failed to get plan prices", "error", err)
		} else {
			p.Prices = prices
		}
		nc, err := s.planRepo.CountNodesForPlan(ctx, p.ID)
		if err != nil {
			s.logger.Warn("failed to count plan nodes", "error", err)
		} else {
			p.NodeCount = nc
		}
	}
	return items, nil
}

func (s *PlanService) ListNodesForPlan(ctx context.Context, planID uuid.UUID) ([]*model.PlanNodeInfo, error) {
	return s.planRepo.ListNodesForPlan(ctx, planID)
}

// ReplacePlanNodes 批量替换套餐的节点绑定
func (s *PlanService) ReplacePlanNodes(ctx context.Context, planID uuid.UUID, nodeIDs []uuid.UUID) error {
	// 验证套餐存在
	p, err := s.planRepo.GetByID(ctx, planID)
	if err != nil {
		return err
	}
	if p == nil {
		return ErrPlanNotFound
	}
	return s.planRepo.ReplacePlanNodes(ctx, planID, nodeIDs)
}

func (s *PlanService) Create(ctx context.Context, req *model.CreatePlanRequest) (*model.Plan, error) {
	// 校验 code 格式：只允许小写字母、数字、连字符（与前端 slugify 输出对齐）
	if !planCodeRegex.MatchString(req.Code) {
		return nil, ErrPlanCodeInvalid
	}

	existing, err := s.planRepo.GetByCode(ctx, req.Code)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrPlanCodeExists
	}

	featureFlags := req.FeatureFlags
	if featureFlags == nil {
		featureFlags = make(map[string]interface{})
	}
	ffJSON, _ := json.Marshal(featureFlags)

	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}

	p := &model.Plan{
		Code:           req.Code,
		Name:           req.Name,
		Description:    req.Description,
		Content:        req.Content,
		Status:         req.Status,
		BillingType:    req.BillingType,
		TrafficBytes:   req.TrafficBytes,
		SpeedLimitMbps: req.SpeedLimitMbps,
		DeviceLimit:    req.DeviceLimit,
		IPLimit:        req.IPLimit,
		ResetCycle:     req.ResetCycle,
		DurationDays:   req.DurationDays,
		CanRenew:       req.CanRenew,
		SortOrder:      req.SortOrder,
		GroupID:        req.GroupID,
		Tags:           tags,
		FeatureFlags:   ffJSON,
	}

	if err := s.planRepo.Create(ctx, p); err != nil {
		return nil, err
	}

	if len(req.Prices) > 0 {
		prices := make(map[string]model.PlanPriceEntry)
		for _, pp := range req.Prices {
			prices[pp.PeriodCode] = model.PlanPriceEntry{USDT: pp.PriceUSDT, CNY: pp.PriceCNY}
		}
		if err := s.planRepo.SetPrices(ctx, p.ID, prices); err != nil {
			s.logger.Error("failed to set plan prices", "error", err)
		}
		p.Prices = prices
	}

	return p, nil
}

func (s *PlanService) Update(ctx context.Context, id uuid.UUID, req *model.UpdatePlanRequest) (*model.Plan, error) {
	p, err := s.planRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrPlanNotFound
	}

	if req.Name != nil {
		p.Name = *req.Name
	}
	if req.Description != nil {
		p.Description = *req.Description
	}
	if req.Content != nil {
		p.Content = *req.Content
	}
	if req.Status != nil {
		p.Status = *req.Status
	}
	if req.BillingType != nil {
		p.BillingType = *req.BillingType
	}
	if req.TrafficBytes != nil {
		p.TrafficBytes = *req.TrafficBytes
	}
	if req.SpeedLimitMbps != nil {
		p.SpeedLimitMbps = req.SpeedLimitMbps
	}
	if req.DeviceLimit != nil {
		p.DeviceLimit = req.DeviceLimit
	}
	if req.IPLimit != nil {
		p.IPLimit = req.IPLimit
	}
	if req.ResetCycle != nil {
		p.ResetCycle = req.ResetCycle
	}
	if req.DurationDays != nil {
		p.DurationDays = req.DurationDays
	}
	if req.CanRenew != nil {
		p.CanRenew = *req.CanRenew
	}
	if req.SortOrder != nil {
		p.SortOrder = *req.SortOrder
	}
	if req.GroupID != nil {
		p.GroupID = req.GroupID
	}
	if req.Tags != nil {
		p.Tags = req.Tags
	}
	if req.FeatureFlags != nil {
		ff, _ := json.Marshal(req.FeatureFlags)
		p.FeatureFlags = ff
	}

	if err := s.planRepo.Update(ctx, p); err != nil {
		return nil, err
	}

	if req.Prices != nil {
		prices := make(map[string]model.PlanPriceEntry)
		for _, pp := range req.Prices {
			prices[pp.PeriodCode] = model.PlanPriceEntry{USDT: pp.PriceUSDT, CNY: pp.PriceCNY}
		}
		if err := s.planRepo.SetPrices(ctx, p.ID, prices); err != nil {
			s.logger.Error("failed to set plan prices", "error", err)
		}
		p.Prices = prices
	} else {
		prices, err := s.planRepo.GetPrices(ctx, p.ID)
		if err == nil {
			p.Prices = prices
		}
	}

	return p, nil
}

func (s *PlanService) Delete(ctx context.Context, id uuid.UUID) error {
	p, err := s.planRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if p == nil {
		return ErrPlanNotFound
	}
	return s.planRepo.Delete(ctx, id)
}

func MapPlanErrorToCode(err error) (config.ErrorCode, string) {
	switch {
	case errors.Is(err, ErrPlanNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrPlanCodeExists):
		return config.CodeConflict, err.Error()
	case errors.Is(err, ErrPlanCodeInvalid):
		return config.CodeValidationFailed, err.Error()
	default:
		return config.CodeInternalError, ""
	}
}
