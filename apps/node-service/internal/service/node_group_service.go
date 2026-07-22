package service

import (
	"context"
	"strings"

	"github.com/airport-panel/node-service/internal/model"
	"github.com/airport-panel/node-service/internal/repo"
	"github.com/google/uuid"
)

// NodeGroupService 会员分组业务逻辑
type NodeGroupService struct {
	groupRepo *repo.NodeGroupRepo
}

func NewNodeGroupService(groupRepo *repo.NodeGroupRepo) *NodeGroupService {
	return &NodeGroupService{groupRepo: groupRepo}
}

func (s *NodeGroupService) Create(ctx context.Context, req *model.CreateNodeGroupRequest) (*model.NodeGroup, error) {
	existing, err := s.groupRepo.GetByCode(ctx, req.Code)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrNodeGroupAlreadyExists
	}

	visibility := req.Visibility
	if visibility == "" {
		visibility = "public"
	}
	sortOrder := 0
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}

	g := &model.NodeGroup{
		ID:          uuid.New(),
		Code:        req.Code,
		Name:        req.Name,
		Description: req.Description,
		Visibility:  visibility,
		SortOrder:   sortOrder,
	}
	if err := s.groupRepo.Create(ctx, g); err != nil {
		return nil, err
	}
	return g, nil
}

func (s *NodeGroupService) Get(ctx context.Context, id uuid.UUID) (*model.NodeGroup, error) {
	g, err := s.groupRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if g == nil {
		return nil, ErrNodeGroupNotFound
	}
	return g, nil
}

func (s *NodeGroupService) List(ctx context.Context, page, pageSize int, search string) ([]*model.NodeGroup, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}
	return s.groupRepo.List(ctx, page, pageSize, search)
}

// ListAll 返回所有分组（不分页，供下拉框使用）
func (s *NodeGroupService) ListAll(ctx context.Context) ([]*model.NodeGroup, error) {
	return s.groupRepo.ListAll(ctx)
}

func (s *NodeGroupService) Update(ctx context.Context, id uuid.UUID, req *model.UpdateNodeGroupRequest) (*model.NodeGroup, error) {
	g, err := s.groupRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if g == nil {
		return nil, ErrNodeGroupNotFound
	}

	if req.Name != nil {
		g.Name = *req.Name
	}
	if req.Description != nil {
		g.Description = req.Description
	}
	if req.Visibility != nil {
		g.Visibility = *req.Visibility
	}
	if req.SortOrder != nil {
		g.SortOrder = *req.SortOrder
	}

	if err := s.groupRepo.Update(ctx, g); err != nil {
		return nil, err
	}
	return g, nil
}

func (s *NodeGroupService) Delete(ctx context.Context, id uuid.UUID) error {
	g, err := s.groupRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if g == nil {
		return ErrNodeGroupNotFound
	}
	if err := s.groupRepo.Delete(ctx, id); err != nil {
		// repo 返回 "无法删除：仍有 N 个节点..." 时转为 ErrNodeGroupInUse
		if strings.Contains(err.Error(), "无法删除") {
			return ErrNodeGroupInUse
		}
		return err
	}
	return nil
}

// CountNodes 统计分组下节点数（供 handler 填充 NodeCount 字段）
func (s *NodeGroupService) CountNodes(ctx context.Context, groupID uuid.UUID) (int, error) {
	return s.groupRepo.CountNodes(ctx, groupID)
}

// BatchBindNodes 批量绑定节点到分组
// 返回实际绑定的节点数
func (s *NodeGroupService) BatchBindNodes(ctx context.Context, groupID uuid.UUID, nodeIDs []uuid.UUID) (int, error) {
	// 确认分组存在
	g, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return 0, err
	}
	if g == nil {
		return 0, ErrNodeGroupNotFound
	}
	return s.groupRepo.BatchBindNodes(ctx, groupID, nodeIDs)
}

// BatchUnbindNodes 批量解绑节点（从分组移除）
// 返回实际解绑的节点数
func (s *NodeGroupService) BatchUnbindNodes(ctx context.Context, groupID uuid.UUID, nodeIDs []uuid.UUID) (int, error) {
	// 确认分组存在
	g, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return 0, err
	}
	if g == nil {
		return 0, ErrNodeGroupNotFound
	}
	return s.groupRepo.BatchUnbindNodes(ctx, groupID, nodeIDs)
}

// ListNodeIDsByGroup 返回分组下的所有节点 ID
func (s *NodeGroupService) ListNodeIDsByGroup(ctx context.Context, groupID uuid.UUID) ([]uuid.UUID, error) {
	return s.groupRepo.ListNodeIDsByGroup(ctx, groupID)
}
