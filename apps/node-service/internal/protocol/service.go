package protocol

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"text/template"

	"github.com/google/uuid"
)

// ProtocolStore 抽象 ProtocolRegistryRepo 的数据访问（便于测试注入 mock）
type ProtocolStore interface {
	Create(ctx context.Context, p *ProtocolRegistry) error
	GetByID(ctx context.Context, id uuid.UUID) (*ProtocolRegistry, error)
	FindByCombo(ctx context.Context, protocolType, transportType, securityType string) (*ProtocolRegistry, error)
	Update(ctx context.Context, p *ProtocolRegistry) error
	List(ctx context.Context, page, pageSize int, query ProtocolListQuery) ([]*ProtocolRegistry, int, error)
}

// TemplateStore 抽象 ConfigTemplateRepo 的数据访问
type TemplateStore interface {
	GetByCode(ctx context.Context, code string) (*ConfigTemplate, error)
	GetByID(ctx context.Context, id uuid.UUID) (*ConfigTemplate, error)
	Update(ctx context.Context, t *ConfigTemplate) error
	List(ctx context.Context, page, pageSize int, query TemplateListQuery) ([]*ConfigTemplate, int, error)
}

// ProtocolService 封装协议注册表的业务逻辑
type ProtocolService struct {
	store  ProtocolStore
	logger *slog.Logger
}

func NewProtocolService(store ProtocolStore, logger *slog.Logger) *ProtocolService {
	return &ProtocolService{store: store, logger: logger}
}

// FindByCombo 按 protocol_type + transport_type + security_type 查找启用的 schema
func (s *ProtocolService) FindByCombo(ctx context.Context, protocolType, transportType, securityType string) (*ProtocolRegistry, error) {
	p, err := s.store.FindByCombo(ctx, protocolType, transportType, securityType)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrProtocolNotFound
	}
	return p, nil
}

func (s *ProtocolService) GetByID(ctx context.Context, id uuid.UUID) (*ProtocolRegistry, error) {
	p, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrProtocolNotFound
	}
	return p, nil
}

func (s *ProtocolService) List(ctx context.Context, page, pageSize int, query ProtocolListQuery) ([]*ProtocolRegistry, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.store.List(ctx, page, pageSize, query)
}

// Create 创建协议注册项。若同组合已存在返回 ErrProtocolExists
func (s *ProtocolService) Create(ctx context.Context, req *CreateProtocolRequest) (*ProtocolRegistry, error) {
	if req.ConfigSchema == nil {
		return nil, ErrInvalidSchemaPayload
	}
	existing, err := s.store.FindByCombo(ctx, req.ProtocolType, req.TransportType, req.SecurityType)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrProtocolExists
	}

	schemaVersion := req.SchemaVersion
	if schemaVersion == "" {
		schemaVersion = "v1"
	}

	p := &ProtocolRegistry{
		ProtocolType:  req.ProtocolType,
		TransportType: req.TransportType,
		SecurityType:  req.SecurityType,
		SchemaVersion: schemaVersion,
		ConfigSchema:  req.ConfigSchema,
		Description:   req.Description,
		IsEnabled:     true,
	}

	if err := s.store.Create(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// Update 更新协议 schema（仅允许更新 config_schema / description / is_enabled）
func (s *ProtocolService) Update(ctx context.Context, id uuid.UUID, req *UpdateProtocolRequest) (*ProtocolRegistry, error) {
	p, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrProtocolNotFound
	}

	if req.ConfigSchema != nil {
		p.ConfigSchema = *req.ConfigSchema
	}
	if req.Description != nil {
		p.Description = req.Description
	}
	if req.IsEnabled != nil {
		p.IsEnabled = *req.IsEnabled
	}

	if err := s.store.Update(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// ValidateConfig 用协议 schema 的 required 字段对入参 config 做基本校验。
// 不引入完整 JSON Schema 库，仅校验 required 字段存在性 + 顶层类型匹配。
func (s *ProtocolService) ValidateConfig(ctx context.Context, protocolType, transportType, securityType string, config map[string]interface{}) error {
	p, err := s.store.FindByCombo(ctx, protocolType, transportType, securityType)
	if err != nil {
		return err
	}
	if p == nil {
		return ErrProtocolNotFound
	}
	return validateAgainstSchema(config, p.ConfigSchema)
}

// validateAgainstSchema 递归校验 required 字段。
// schema 形如 {"type":"object","properties":{...},"required":[...]}
func validateAgainstSchema(config map[string]interface{}, schema Map) error {
	if schema == nil {
		return nil
	}
	schemaType, _ := schema["type"].(string)
	if schemaType != "object" && schemaType != "" {
		// 非 object 类型的 schema 顶层不校验（本批只处理 object）
		return nil
	}

	props, _ := schema["properties"].(map[string]interface{})
	requiredRaw, _ := schema["required"].([]interface{})

	// 检查 required 字段
	for _, r := range requiredRaw {
		field, ok := r.(string)
		if !ok {
			continue
		}
		val, exists := config[field]
		if !exists {
			return fmt.Errorf("%w: missing required field %q", ErrSchemaValidation, field)
		}
		// 递归校验嵌套 object
		if propSchema, ok := props[field].(Map); ok {
			if nested, ok := val.(map[string]interface{}); ok {
				if t, _ := propSchema["type"].(string); t == "object" {
					if err := validateAgainstSchema(nested, propSchema); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// TemplateService 封装配置模板的业务逻辑
type TemplateService struct {
	store  TemplateStore
	logger *slog.Logger
}

func NewTemplateService(store TemplateStore, logger *slog.Logger) *TemplateService {
	return &TemplateService{store: store, logger: logger}
}

func (s *TemplateService) GetByCode(ctx context.Context, code string) (*ConfigTemplate, error) {
	t, err := s.store.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, ErrTemplateNotFound
	}
	return t, nil
}

func (s *TemplateService) List(ctx context.Context, page, pageSize int, query TemplateListQuery) ([]*ConfigTemplate, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.store.List(ctx, page, pageSize, query)
}

// Update 按 code 更新模板（name/content/variables_schema/is_default）
func (s *TemplateService) Update(ctx context.Context, code string, req *UpdateTemplateRequest) (*ConfigTemplate, error) {
	t, err := s.store.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, ErrTemplateNotFound
	}

	if req.Name != nil {
		t.Name = *req.Name
	}
	if req.Content != nil {
		t.Content = *req.Content
	}
	if req.VariablesSchema != nil {
		t.VariablesSchema = *req.VariablesSchema
	}
	if req.IsDefault != nil {
		t.IsDefault = *req.IsDefault
	}

	if err := s.store.Update(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// Render 用 Go text/template 渲染模板。variables 是模板变量 map。
// 模板内容本身是 JSON 字符串，变量用 {{.xxx}} 形式注入。
func (s *TemplateService) Render(ctx context.Context, code string, variables map[string]interface{}) (*RenderTemplateResponse, error) {
	t, err := s.store.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, ErrTemplateNotFound
	}

	tmpl, err := template.New(code).Parse(t.Content)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTemplateRender, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, variables); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTemplateRender, err)
	}

	rendered := strings.TrimSpace(buf.String())
	return &RenderTemplateResponse{Code: code, Rendered: rendered}, nil
}
