package service

import (
	"context"
	"errors"
	"sync"

	"github.com/airport-panel/subscription-service/internal/model"
	"github.com/google/uuid"
)

// TemplateRepo 是订阅模板仓储接口（供 TemplateService 依赖，便于测试 mock）。
//
// 实现方为 repo.SubscribeTemplateRepo（基于 pgxpool）。
type TemplateRepo interface {
	GetByName(ctx context.Context, name string) (*model.SubscribeTemplate, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.SubscribeTemplate, error)
	List(ctx context.Context) ([]*model.SubscribeTemplate, error)
	UpdateContent(ctx context.Context, id uuid.UUID, content string, enabled *bool) (*model.SubscribeTemplate, error)
}

// 模板未找到的哨兵错误。
var ErrSubscribeTemplateNotFound = errors.New("subscribe template not found")

// TemplateService 提供按名称索引的订阅模板管理（对齐 Xboard subscribe_template helper）。
//
// 设计要点：
//   - 内存缓存（name→content）：渲染器高频读模板，避免每次订阅请求查库；对齐 Xboard
//     SubscribeTemplate::getContent 的 Cache::remember(3600) 行为。
//   - 写穿透：UpdateTemplate 写库后立即失效对应缓存项，下次读触发重载。
//   - ReloadCache：管理端可主动全量重载（如批量导入模板后）。
//
// 注意：缓存仅缓存 content（渲染器实际消费的部分），模板元数据（ID/enabled 等）始终查库。
type TemplateService struct {
	repo    TemplateRepo
	cache   map[string]string
	cacheMu sync.RWMutex
}

// NewTemplateService 构造模板服务。构造后应调用 ReloadCache 预热缓存。
func NewTemplateService(repo TemplateRepo) *TemplateService {
	return &TemplateService{
		repo:  repo,
		cache: make(map[string]string),
	}
}

// GetTemplate 获取模板内容（优先缓存）。
// 仅返回 enabled 模板；未找到或被禁用时返回 ErrSubscribeTemplateNotFound。
// 对齐 Xboard subscribe_template($name)：渲染器按内核/格式名取模板内容。
func (s *TemplateService) GetTemplate(ctx context.Context, name string) (string, error) {
	// 1. 读缓存（快路径）
	s.cacheMu.RLock()
	if content, ok := s.cache[name]; ok {
		s.cacheMu.RUnlock()
		return content, nil
	}
	s.cacheMu.RUnlock()

	// 2. 缓存未命中 → 查库
	t, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return "", err
	}
	if t == nil {
		return "", ErrSubscribeTemplateNotFound
	}

	// 3. 回填缓存
	s.cacheMu.Lock()
	s.cache[name] = t.Content
	s.cacheMu.Unlock()

	return t.Content, nil
}

// ListTemplates 列出所有模板（含禁用项，供管理端展示）。
func (s *TemplateService) ListTemplates(ctx context.Context) ([]*model.SubscribeTemplate, error) {
	return s.repo.List(ctx)
}

// GetByID 按 ID 获取模板（供管理端更新前校验）。
func (s *TemplateService) GetByID(ctx context.Context, id uuid.UUID) (*model.SubscribeTemplate, error) {
	return s.repo.GetByID(ctx, id)
}

// UpdateTemplate 更新模板内容并失效对应缓存。
// 内置模板允许编辑内容，但不允许通过此方法改名称/删除。
func (s *TemplateService) UpdateTemplate(ctx context.Context, id uuid.UUID, content string, enabled *bool) (*model.SubscribeTemplate, error) {
	// 先确认模板存在，拿到 name 以便精准失效缓存
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrSubscribeTemplateNotFound
	}

	updated, err := s.repo.UpdateContent(ctx, id, content, enabled)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, ErrSubscribeTemplateNotFound
	}

	// 失效缓存（若模板仍 enabled，下次读会重新回填；若被禁用则缓存清除后不再命中）
	s.cacheMu.Lock()
	delete(s.cache, existing.Name)
	s.cacheMu.Unlock()

	return updated, nil
}

// ReloadCache 重新加载缓存（全量）。
// 管理端批量修改后可调用此方法预热/刷新。仅缓存 enabled 模板。
func (s *TemplateService) ReloadCache(ctx context.Context) error {
	templates, err := s.repo.List(ctx)
	if err != nil {
		return err
	}
	s.cacheMu.Lock()
	s.cache = make(map[string]string, len(templates))
	for _, t := range templates {
		if t.Enabled {
			s.cache[t.Name] = t.Content
		}
	}
	s.cacheMu.Unlock()
	return nil
}

// Invalidate 失效指定名称的缓存（供事件总线等外部触发）。
func (s *TemplateService) Invalidate(name string) {
	s.cacheMu.Lock()
	delete(s.cache, name)
	s.cacheMu.Unlock()
}
