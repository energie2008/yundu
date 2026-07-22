package protocol

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// fakeProtocolStore 是 ProtocolStore 的内存实现
type fakeProtocolStore struct {
	byCombo map[string]*ProtocolRegistry
	byID    map[uuid.UUID]*ProtocolRegistry
}

func newFakeProtocolStore() *fakeProtocolStore {
	return &fakeProtocolStore{
		byCombo: make(map[string]*ProtocolRegistry),
		byID:   make(map[uuid.UUID]*ProtocolRegistry),
	}
}

func comboKey(p, t, s string) string {
	return p + "|" + t + "|" + s
}

func (f *fakeProtocolStore) Create(ctx context.Context, p *ProtocolRegistry) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	key := comboKey(p.ProtocolType, p.TransportType, p.SecurityType)
	f.byCombo[key] = p
	f.byID[p.ID] = p
	return nil
}
func (f *fakeProtocolStore) GetByID(ctx context.Context, id uuid.UUID) (*ProtocolRegistry, error) {
	if p, ok := f.byID[id]; ok {
		return p, nil
	}
	return nil, nil
}
func (f *fakeProtocolStore) FindByCombo(ctx context.Context, p, t, s string) (*ProtocolRegistry, error) {
	if r, ok := f.byCombo[comboKey(p, t, s)]; ok && r.IsEnabled {
		return r, nil
	}
	return nil, nil
}
func (f *fakeProtocolStore) Update(ctx context.Context, p *ProtocolRegistry) error {
	f.byID[p.ID] = p
	f.byCombo[comboKey(p.ProtocolType, p.TransportType, p.SecurityType)] = p
	return nil
}
func (f *fakeProtocolStore) List(ctx context.Context, page, pageSize int, q ProtocolListQuery) ([]*ProtocolRegistry, int, error) {
	var out []*ProtocolRegistry
	for _, p := range f.byID {
		out = append(out, p)
	}
	return out, len(out), nil
}

// fakeTemplateStore 是 TemplateStore 的内存实现
type fakeTemplateStore struct {
	byCode map[string]*ConfigTemplate
}

func newFakeTemplateStore() *fakeTemplateStore {
	return &fakeTemplateStore{byCode: make(map[string]*ConfigTemplate)}
}

func (f *fakeTemplateStore) GetByCode(ctx context.Context, code string) (*ConfigTemplate, error) {
	if t, ok := f.byCode[code]; ok {
		return t, nil
	}
	return nil, nil
}
func (f *fakeTemplateStore) GetByID(ctx context.Context, id uuid.UUID) (*ConfigTemplate, error) {
	for _, t := range f.byCode {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, nil
}
func (f *fakeTemplateStore) Update(ctx context.Context, t *ConfigTemplate) error {
	f.byCode[t.Code] = t
	return nil
}
func (f *fakeTemplateStore) List(ctx context.Context, page, pageSize int, q TemplateListQuery) ([]*ConfigTemplate, int, error) {
	var out []*ConfigTemplate
	for _, t := range f.byCode {
		out = append(out, t)
	}
	return out, len(out), nil
}

func TestCreateProtocol_Happy(t *testing.T) {
	store := newFakeProtocolStore()
	svc := NewProtocolService(store, nil)

	req := &CreateProtocolRequest{
		ProtocolType:  "trojan",
		TransportType: "tcp",
		SecurityType:  "tls",
		ConfigSchema: Map{
			"type":     "object",
			"required": []interface{}{"password"},
			"properties": map[string]interface{}{
				"password": map[string]interface{}{"type": "string"},
			},
		},
	}
	p, err := svc.Create(context.Background(), req)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if p.ProtocolType != "trojan" {
		t.Errorf("ProtocolType = %q, want trojan", p.ProtocolType)
	}
	if p.SchemaVersion != "v1" {
		t.Errorf("SchemaVersion = %q, want v1", p.SchemaVersion)
	}
	if !p.IsEnabled {
		t.Errorf("IsEnabled = false, want true")
	}
}

func TestCreateProtocol_DuplicateCombo_ReturnsConflict(t *testing.T) {
	store := newFakeProtocolStore()
	existing := &ProtocolRegistry{
		ID:            uuid.New(),
		ProtocolType:  "vless",
		TransportType: "tcp",
		SecurityType:  "reality",
		SchemaVersion: "v1",
		IsEnabled:     true,
		ConfigSchema:  Map{"type": "object"},
	}
	store.byCombo[comboKey("vless", "tcp", "reality")] = existing
	store.byID[existing.ID] = existing

	svc := NewProtocolService(store, nil)
	req := &CreateProtocolRequest{
		ProtocolType:  "vless",
		TransportType: "tcp",
		SecurityType:  "reality",
		ConfigSchema:  Map{"type": "object"},
	}
	_, err := svc.Create(context.Background(), req)
	if !errors.Is(err, ErrProtocolExists) {
		t.Fatalf("expected ErrProtocolExists, got %v", err)
	}
}

func TestValidateConfig_MissingRequired_ReturnsValidationFailed(t *testing.T) {
	store := newFakeProtocolStore()
	existing := &ProtocolRegistry{
		ID:            uuid.New(),
		ProtocolType:  "vless",
		TransportType: "tcp",
		SecurityType:  "reality",
		SchemaVersion: "v1",
		IsEnabled:     true,
		ConfigSchema: Map{
			"type":     "object",
			"required": []interface{}{"uuid"},
			"properties": map[string]interface{}{
				"uuid": map[string]interface{}{"type": "string"},
			},
		},
	}
	store.byCombo[comboKey("vless", "tcp", "reality")] = existing
	store.byID[existing.ID] = existing

	svc := NewProtocolService(store, nil)
	err := svc.ValidateConfig(context.Background(), "vless", "tcp", "reality", map[string]interface{}{})
	if !errors.Is(err, ErrSchemaValidation) {
		t.Fatalf("expected ErrSchemaValidation, got %v", err)
	}
}

func TestValidateConfig_HasRequired_Passes(t *testing.T) {
	store := newFakeProtocolStore()
	existing := &ProtocolRegistry{
		ID:            uuid.New(),
		ProtocolType:  "vless",
		TransportType: "tcp",
		SecurityType:  "reality",
		SchemaVersion: "v1",
		IsEnabled:     true,
		ConfigSchema: Map{
			"type":     "object",
			"required": []interface{}{"uuid"},
			"properties": map[string]interface{}{
				"uuid": map[string]interface{}{"type": "string"},
			},
		},
	}
	store.byCombo[comboKey("vless", "tcp", "reality")] = existing
	store.byID[existing.ID] = existing

	svc := NewProtocolService(store, nil)
	err := svc.ValidateConfig(context.Background(), "vless", "tcp", "reality", map[string]interface{}{
		"uuid": "test-uuid",
	})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestRenderTemplate_Happy(t *testing.T) {
	store := newFakeTemplateStore()
	tpl := &ConfigTemplate{
		ID:           uuid.New(),
		Code:         "test-tpl",
		Name:         "Test Template",
		RuntimeType:  "xray",
		TemplateType: "inbound",
		Content:      `{"port": {{.port}}, "name": "{{.name}}"}`,
	}
	store.byCode["test-tpl"] = tpl

	svc := NewTemplateService(store, nil)
	resp, err := svc.Render(context.Background(), "test-tpl", map[string]interface{}{
		"port": 8080,
		"name": "hello",
	})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	want := `{"port": 8080, "name": "hello"}`
	if resp.Rendered != want {
		t.Errorf("Rendered = %q, want %q", resp.Rendered, want)
	}
	if resp.Code != "test-tpl" {
		t.Errorf("Code = %q, want test-tpl", resp.Code)
	}
}

func TestRenderTemplate_NotFound_ReturnsTemplateNotFound(t *testing.T) {
	store := newFakeTemplateStore()
	svc := NewTemplateService(store, nil)
	_, err := svc.Render(context.Background(), "missing-code", map[string]interface{}{})
	if !errors.Is(err, ErrTemplateNotFound) {
		t.Fatalf("expected ErrTemplateNotFound, got %v", err)
	}
}

func TestUpdateTemplate_NotFound_ReturnsTemplateNotFound(t *testing.T) {
	store := newFakeTemplateStore()
	svc := NewTemplateService(store, nil)
	name := "new"
	_, err := svc.Update(context.Background(), "missing", &UpdateTemplateRequest{Name: &name})
	if !errors.Is(err, ErrTemplateNotFound) {
		t.Fatalf("expected ErrTemplateNotFound, got %v", err)
	}
}
