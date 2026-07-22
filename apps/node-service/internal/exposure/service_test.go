package exposure

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/airport-panel/node-service/internal/cert"
	"github.com/google/uuid"
)

// fakeExposureStore 内存实现
type fakeExposureStore struct {
	byCode map[string]*EdgeExposure
	byID   map[uuid.UUID]*EdgeExposure
}

func newFakeExposureStore() *fakeExposureStore {
	return &fakeExposureStore{byCode: map[string]*EdgeExposure{}, byID: map[uuid.UUID]*EdgeExposure{}}
}

func (f *fakeExposureStore) Create(ctx context.Context, e *EdgeExposure) error {
	f.byCode[e.Code] = e
	f.byID[e.ID] = e
	return nil
}
func (f *fakeExposureStore) GetByID(ctx context.Context, id uuid.UUID) (*EdgeExposure, error) {
	if e, ok := f.byID[id]; ok {
		return e, nil
	}
	return nil, nil
}
func (f *fakeExposureStore) GetByCode(ctx context.Context, code string) (*EdgeExposure, error) {
	if e, ok := f.byCode[code]; ok {
		return e, nil
	}
	return nil, nil
}
func (f *fakeExposureStore) GetByServerID(ctx context.Context, serverID uuid.UUID) (*EdgeExposure, error) {
	for _, e := range f.byID {
		if e.ServerID == serverID {
			return e, nil
		}
	}
	return nil, nil
}
func (f *fakeExposureStore) Update(ctx context.Context, e *EdgeExposure) error {
	f.byID[e.ID] = e
	f.byCode[e.Code] = e
	return nil
}
func (f *fakeExposureStore) Delete(ctx context.Context, id uuid.UUID) error {
	if e, ok := f.byID[id]; ok {
		delete(f.byID, id)
		delete(f.byCode, e.Code)
	}
	return nil
}
func (f *fakeExposureStore) List(ctx context.Context, page, pageSize int, status string) ([]*EdgeExposure, int, error) {
	var out []*EdgeExposure
	for _, e := range f.byID {
		out = append(out, e)
	}
	return out, len(out), nil
}

// fakeNginxStore no-op
type fakeNginxStore struct{}

func (f *fakeNginxStore) GetByExposureAndHash(ctx context.Context, exposureID uuid.UUID, hash string) (*NginxGeneratedConfig, error) {
	return nil, nil
}
func (f *fakeNginxStore) CreateIfAbsent(ctx context.Context, c *NginxGeneratedConfig) (*NginxGeneratedConfig, error) {
	return c, nil
}

// fakeCompatStore 可配置返回规则
type fakeCompatStore struct {
	rule *ExposureCompatRule
	err  error
}

func (f *fakeCompatStore) FindMatch(ctx context.Context, protocol, transport, security, mode string) (*ExposureCompatRule, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.rule, nil
}

// fakeNodeFetcher 可配置返回 NodeInfo
type fakeNodeFetcher struct {
	info *NodeInfo
	err  error
}

func (f *fakeNodeFetcher) FetchByServerID(ctx context.Context, serverID uuid.UUID) (*NodeInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.info, nil
}

// fakeCertFetcher 返回 nil（renderer 在 profile=nil 时也能工作）
type fakeCertFetcher struct{}

func (f *fakeCertFetcher) FetchProfile(ctx context.Context, profileID uuid.UUID) (*cert.TLSProfile, error) {
	return nil, nil
}

func TestRenderNginxServerBlock_ContainsListenAndProxyPass(t *testing.T) {
	host := "example.com"
	wsPath := "/ws"
	e := &EdgeExposure{
		ID:             uuid.New(),
		Code:           "exp-001",
		PublicHostname: &host,
		PublicPort:     443,
		OriginHost:     "127.0.0.1",
		OriginPort:     10000,
		NginxWSPath:    &wsPath,
	}
	conf, err := RenderNginxServerBlock(e, nil)
	if err != nil {
		t.Fatalf("RenderNginxServerBlock failed: %v", err)
	}
	if !strings.Contains(conf, "listen 443 ssl http2") {
		t.Errorf("nginx conf missing listen directive:\n%s", conf)
	}
	if !strings.Contains(conf, "server_name example.com") {
		t.Errorf("nginx conf missing server_name:\n%s", conf)
	}
	if !strings.Contains(conf, "proxy_pass http://127.0.0.1:10000") {
		t.Errorf("nginx conf missing proxy_pass:\n%s", conf)
	}
	if !strings.Contains(conf, "proxy_set_header Upgrade $http_upgrade") {
		t.Errorf("nginx conf missing WS upgrade header:\n%s", conf)
	}
	if !strings.Contains(conf, "location /ws {") {
		t.Errorf("nginx conf missing location block:\n%s", conf)
	}
}

func TestValidate_RealityPlusNginx_NotAllowed(t *testing.T) {
	store := newFakeExposureStore()
	serverID := uuid.New()
	e := &EdgeExposure{
		ID:           uuid.New(),
		ServerID:     serverID,
		Code:         "exp-reality-nginx",
		Name:         "reality-nginx",
		ExposureMode: "nginx_reverse_proxy",
		PublicPort:   443,
		OriginHost:   "127.0.0.1",
		OriginPort:   8443,
		Status:       "pending",
	}
	store.byID[e.ID] = e

	reason := "REALITY 不兼容 Nginx 反代"
	compatStore := &fakeCompatStore{
		rule: &ExposureCompatRule{
			ProtocolType:  "vless",
			TransportType: strPtr("tcp"),
			SecurityType:  strPtr("reality"),
			ExposureMode:  "nginx_reverse_proxy",
			IsAllowed:     false,
			Reason:        &reason,
		},
	}
	nodeFetcher := &fakeNodeFetcher{
		info: &NodeInfo{
			ProtocolType:  "vless",
			TransportType: "tcp",
			SecurityType: "reality",
		},
	}

	svc := NewExposureService(store, &fakeNginxStore{}, compatStore, nodeFetcher, &fakeCertFetcher{}, nil, nil)
	resp, err := svc.Validate(context.Background(), e.ID)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if resp.IsAllowed {
		t.Errorf("IsAllowed = true, want false (REALITY+nginx should be rejected)")
	}
	if resp.Reason != reason {
		t.Errorf("Reason = %q, want %q", resp.Reason, reason)
	}
}

func TestValidate_NoRule_DefaultsAllowed(t *testing.T) {
	store := newFakeExposureStore()
	serverID := uuid.New()
	e := &EdgeExposure{
		ID:           uuid.New(),
		ServerID:     serverID,
		Code:         "exp-norule",
		ExposureMode: "direct_public_ip",
		PublicPort:   443,
		OriginHost:   "127.0.0.1",
		OriginPort:   8443,
		Status:       "pending",
	}
	store.byID[e.ID] = e

	compatStore := &fakeCompatStore{rule: nil} // 无匹配规则
	nodeFetcher := &fakeNodeFetcher{
		info: &NodeInfo{ProtocolType: "vless", TransportType: "tcp", SecurityType: "tls"},
	}

	svc := NewExposureService(store, &fakeNginxStore{}, compatStore, nodeFetcher, &fakeCertFetcher{}, nil, nil)
	resp, err := svc.Validate(context.Background(), e.ID)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if !resp.IsAllowed {
		t.Errorf("IsAllowed = false, want true when no rule matches")
	}
}

func TestCreateExposure_DuplicateCode_ReturnsConflict(t *testing.T) {
	store := newFakeExposureStore()
	existing := &EdgeExposure{ID: uuid.New(), Code: "dup", Name: "old", Status: "pending"}
	store.byCode["dup"] = existing
	store.byID[existing.ID] = existing

	svc := NewExposureService(store, &fakeNginxStore{}, &fakeCompatStore{}, nil, &fakeCertFetcher{}, nil, nil)
	_, err := svc.Create(context.Background(), &CreateExposureRequest{
		ServerID:     uuid.New(),
		Code:         "dup",
		Name:         "new",
		ExposureMode: "direct_public_ip",
		OriginPort:   8443,
	})
	if !errors.Is(err, ErrExposureAlreadyExists) {
		t.Fatalf("expected ErrExposureAlreadyExists, got %v", err)
	}
}
