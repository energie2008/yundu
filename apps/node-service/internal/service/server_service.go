package service

import (
	"context"
	"errors"

	"github.com/airport-panel/config"
	"github.com/airport-panel/node-service/internal/model"
	"github.com/airport-panel/node-service/internal/repo"
	"github.com/google/uuid"
)

type ServerService struct {
	serverRepo  *repo.ServerRepo
	runtimeRepo *repo.RuntimeRepo
	nodeRepo    *repo.NodeRepo
}

func NewServerService(serverRepo *repo.ServerRepo, runtimeRepo *repo.RuntimeRepo, nodeRepo *repo.NodeRepo) *ServerService {
	return &ServerService{
		serverRepo:  serverRepo,
		runtimeRepo: runtimeRepo,
		nodeRepo:    nodeRepo,
	}
}

func (s *ServerService) CreateServer(ctx context.Context, req *model.CreateServerRequest) (*model.Server, error) {
	existing, err := s.serverRepo.GetByCode(ctx, req.Code)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrServerAlreadyExists
	}

	role := req.Role
	if role == "" {
		role = model.ServerRoleNode
	}

	if req.Labels == nil {
		req.Labels = make(map[string]string)
	}
	if req.Metadata == nil {
		req.Metadata = make(map[string]interface{})
	}

	server := &model.Server{
		ID:       uuid.New(),
		Code:     req.Code,
		Name:     req.Name,
		RegionID: req.RegionID,
		Provider: req.Provider,
		Host:     req.Host,
		IPv4:     req.IPv4,
		IPv6:     req.IPv6,
		SSHPort:  req.SSHPort,
		OSName:   req.OSName,
		OSVersion: req.OSVersion,
		Arch:     req.Arch,
		Status:   model.ServerStatusProvisioning,
		Role:     role,
		Labels:   req.Labels,
		Metadata: req.Metadata,
	}

	if err := s.serverRepo.Create(ctx, server); err != nil {
		return nil, err
	}
	return server, nil
}

func (s *ServerService) GetServer(ctx context.Context, id uuid.UUID) (*model.Server, error) {
	server, err := s.serverRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if server == nil {
		return nil, ErrServerNotFound
	}
	return server, nil
}

func (s *ServerService) GetServerByCode(ctx context.Context, code string) (*model.Server, error) {
	server, err := s.serverRepo.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if server == nil {
		return nil, ErrServerNotFound
	}
	return server, nil
}

func (s *ServerService) ListServers(ctx context.Context, page, pageSize int, status model.ServerStatus, search string) ([]*model.Server, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.serverRepo.List(ctx, page, pageSize, status, search)
}

func (s *ServerService) UpdateServerHeartbeat(ctx context.Context, id uuid.UUID) error {
	return s.serverRepo.UpdateHeartbeat(ctx, id)
}

func (s *ServerService) ListRuntimesByServer(ctx context.Context, serverID uuid.UUID) ([]*model.Runtime, error) {
	return s.runtimeRepo.ListByServer(ctx, serverID)
}

func (s *ServerService) CountNodesByServer(ctx context.Context, serverID uuid.UUID) (int, error) {
	return s.nodeRepo.CountByServerID(ctx, serverID)
}

// ListNodesByServer 返回服务器关联的所有启用节点（用于服务器详情页关联节点展示）
func (s *ServerService) ListNodesByServer(ctx context.Context, serverID uuid.UUID) ([]*model.Node, error) {
	return s.nodeRepo.ListByServerID(ctx, serverID)
}

func MapServerErrorToCode(err error) (config.ErrorCode, string) {
	switch {
	case errors.Is(err, ErrServerAlreadyExists):
		return config.CodeConflict, err.Error()
	case errors.Is(err, ErrServerNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrCodeRequired):
		return config.CodeBadRequest, err.Error()
	default:
		return config.CodeInternalError, ""
	}
}
