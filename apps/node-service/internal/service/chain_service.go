package service

import (
	"context"
	"errors"

	"github.com/airport-panel/config"
	"github.com/airport-panel/node-service/internal/model"
	"github.com/airport-panel/node-service/internal/repo"
	"github.com/google/uuid"
)

type ChainService struct {
	chainRepo *repo.ChainRepo
	nodeRepo  *repo.NodeRepo
}

func NewChainService(chainRepo *repo.ChainRepo, nodeRepo *repo.NodeRepo) *ChainService {
	return &ChainService{
		chainRepo: chainRepo,
		nodeRepo:  nodeRepo,
	}
}

func (s *ChainService) CreateChain(ctx context.Context, req *model.CreateChainRequest) (*model.ProxyChain, error) {
	chain := &model.ProxyChain{
		ID:        uuid.New(),
		Code:      req.Code,
		Name:      req.Name,
		Status:    model.ChainStatusActive,
		ChainMode: model.ChainModeSingle,
		Strategy:  model.ChainStrategyOrdered,
		MaxHops:   1,
		Metadata:  req.Metadata,
	}

	if req.ChainMode != "" {
		chain.ChainMode = req.ChainMode
	}
	if req.Strategy != "" {
		chain.Strategy = req.Strategy
	}
	if req.MaxHops > 0 {
		chain.MaxHops = req.MaxHops
	}
	if chain.Metadata == nil {
		chain.Metadata = make(map[string]interface{})
	}
	chain.HealthPolicyID = req.HealthPolicyID

	if err := s.chainRepo.Create(ctx, chain); err != nil {
		return nil, err
	}
	return chain, nil
}

func (s *ChainService) GetChain(ctx context.Context, id uuid.UUID) (*model.ProxyChain, error) {
	chain, err := s.chainRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if chain == nil {
		return nil, ErrChainNotFound
	}
	return chain, nil
}

func (s *ChainService) ListChains(ctx context.Context, page, pageSize int, status model.ChainStatus) ([]*model.ProxyChain, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.chainRepo.List(ctx, page, pageSize, status)
}

func (s *ChainService) AddHop(ctx context.Context, chainID uuid.UUID, req *model.AddHopRequest) (*model.ProxyChainHop, error) {
	chain, err := s.chainRepo.GetByID(ctx, chainID)
	if err != nil {
		return nil, err
	}
	if chain == nil {
		return nil, ErrChainNotFound
	}

	hops, err := s.chainRepo.ListHops(ctx, chainID)
	if err != nil {
		return nil, err
	}
	if len(hops) >= chain.MaxHops {
		return nil, ErrMaxHopsExceeded
	}

	if req.HopType == model.HopTypeNode && req.UpstreamNodeID != nil {
		node, err := s.nodeRepo.GetByID(ctx, *req.UpstreamNodeID)
		if err != nil {
			return nil, err
		}
		if node == nil {
			return nil, ErrNodeNotFound
		}
	}

	if req.OutboundConfigJSON == nil {
		req.OutboundConfigJSON = make(map[string]interface{})
	}

	hop := &model.ProxyChainHop{
		ID:                  uuid.New(),
		ChainID:             chainID,
		HopIndex:            req.HopIndex,
		HopType:             req.HopType,
		UpstreamNodeID:      req.UpstreamNodeID,
		UpstreamRuntimeID:   req.UpstreamRuntimeID,
		OutboundProtocolType: req.OutboundProtocolType,
		OutboundConfigJSON:  req.OutboundConfigJSON,
	}

	if err := s.chainRepo.AddHop(ctx, hop); err != nil {
		return nil, err
	}
	return hop, nil
}

func (s *ChainService) RemoveHop(ctx context.Context, chainID uuid.UUID, hopIndex int) error {
	chain, err := s.chainRepo.GetByID(ctx, chainID)
	if err != nil {
		return err
	}
	if chain == nil {
		return ErrChainNotFound
	}
	return s.chainRepo.RemoveHop(ctx, chainID, hopIndex)
}

func (s *ChainService) ListHops(ctx context.Context, chainID uuid.UUID) ([]*model.ProxyChainHop, error) {
	return s.chainRepo.ListHops(ctx, chainID)
}

func (s *ChainService) BindNode(ctx context.Context, chainID uuid.UUID, req *model.BindNodeRequest) error {
	chain, err := s.chainRepo.GetByID(ctx, chainID)
	if err != nil {
		return err
	}
	if chain == nil {
		return ErrChainNotFound
	}

	node, err := s.nodeRepo.GetByID(ctx, req.NodeID)
	if err != nil {
		return err
	}
	if node == nil {
		return ErrNodeNotFound
	}

	bindMode := req.BindMode
	if bindMode == "" {
		bindMode = model.BindModeDefault
	}
	priority := req.Priority
	if priority == 0 {
		priority = 100
	}

	binding := &model.NodeChainBinding{
		NodeID:   req.NodeID,
		ChainID:  chainID,
		BindMode: bindMode,
		Priority: priority,
	}

	return s.chainRepo.BindNode(ctx, binding)
}

func (s *ChainService) UnbindNode(ctx context.Context, chainID, nodeID uuid.UUID) error {
	return s.chainRepo.UnbindNode(ctx, nodeID, chainID)
}

func (s *ChainService) ListNodeBindings(ctx context.Context, chainID uuid.UUID) ([]*model.NodeChainBinding, error) {
	return s.chainRepo.ListNodeBindings(ctx, chainID)
}

func MapChainErrorToCode(err error) (config.ErrorCode, string) {
	switch {
	case errors.Is(err, ErrChainAlreadyExists):
		return config.CodeConflict, err.Error()
	case errors.Is(err, ErrChainNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrHopNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrNodeAlreadyBound):
		return config.CodeConflict, err.Error()
	case errors.Is(err, ErrInvalidHopType):
		return config.CodeValidationFailed, err.Error()
	case errors.Is(err, ErrMaxHopsExceeded):
		return config.CodeConflict, err.Error()
	case errors.Is(err, ErrNodeNotFound):
		return config.CodeNotFound, err.Error()
	default:
		return config.CodeInternalError, ""
	}
}
