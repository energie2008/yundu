package outbound

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// fakePolicyStore 是 PolicyStore 的内存实现
type fakePolicyStore struct {
	byID   map[uuid.UUID]*OutboundPolicy
	byNode map[uuid.UUID][]*OutboundPolicy
}

func newFakePolicyStore() *fakePolicyStore {
	return &fakePolicyStore{
		byID:   make(map[uuid.UUID]*OutboundPolicy),
		byNode: make(map[uuid.UUID][]*OutboundPolicy),
	}
}

func (f *fakePolicyStore) Create(ctx context.Context, p *OutboundPolicy) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	f.byID[p.ID] = p
	f.byNode[p.NodeID] = append(f.byNode[p.NodeID], p)
	return nil
}
func (f *fakePolicyStore) GetByID(ctx context.Context, id uuid.UUID) (*OutboundPolicy, error) {
	if p, ok := f.byID[id]; ok {
		return p, nil
	}
	return nil, nil
}
func (f *fakePolicyStore) Update(ctx context.Context, p *OutboundPolicy) error {
	f.byID[p.ID] = p
	for i, existing := range f.byNode[p.NodeID] {
		if existing.ID == p.ID {
			f.byNode[p.NodeID][i] = p
			break
		}
	}
	return nil
}
func (f *fakePolicyStore) Delete(ctx context.Context, id uuid.UUID) error {
	if p, ok := f.byID[id]; ok {
		delete(f.byID, id)
		list := f.byNode[p.NodeID]
		for i, existing := range list {
			if existing.ID == id {
				f.byNode[p.NodeID] = append(list[:i], list[i+1:]...)
				break
			}
		}
	}
	return nil
}
func (f *fakePolicyStore) ListByNode(ctx context.Context, nodeID uuid.UUID) ([]*OutboundPolicy, error) {
	return f.byNode[nodeID], nil
}

// fakeWarpStore 是 WarpProfileStore 的内存实现
type fakeWarpStore struct {
	byCode map[string]*WarpProfile
	byID   map[uuid.UUID]*WarpProfile
}

func newFakeWarpStore() *fakeWarpStore {
	return &fakeWarpStore{
		byCode: make(map[string]*WarpProfile),
		byID:   make(map[uuid.UUID]*WarpProfile),
	}
}

func (f *fakeWarpStore) Create(ctx context.Context, w *WarpProfile) error {
	if w.ID == uuid.Nil {
		w.ID = uuid.New()
	}
	f.byCode[w.Code] = w
	f.byID[w.ID] = w
	return nil
}
func (f *fakeWarpStore) GetByID(ctx context.Context, id uuid.UUID) (*WarpProfile, error) {
	if w, ok := f.byID[id]; ok {
		return w, nil
	}
	return nil, nil
}
func (f *fakeWarpStore) GetByCode(ctx context.Context, code string) (*WarpProfile, error) {
	if w, ok := f.byCode[code]; ok {
		return w, nil
	}
	return nil, nil
}
func (f *fakeWarpStore) List(ctx context.Context) ([]*WarpProfile, error) {
	var out []*WarpProfile
	for _, w := range f.byID {
		out = append(out, w)
	}
	return out, nil
}

func TestCreatePolicy_Happy(t *testing.T) {
	store := newFakePolicyStore()
	svc := NewOutboundService(store, nil)

	nodeID := uuid.New()
	priority := 50
	req := &CreatePolicyRequest{
		PolicyType: "direct",
		Priority:   &priority,
		ConfigJSON: Map{},
	}
	p, err := svc.Create(context.Background(), nodeID, req)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if p.NodeID != nodeID {
		t.Errorf("NodeID = %v, want %v", p.NodeID, nodeID)
	}
	if p.Priority != 50 {
		t.Errorf("Priority = %d, want 50", p.Priority)
	}
	if !p.IsEnabled {
		t.Errorf("IsEnabled = false, want true")
	}
}

func TestCreatePolicy_Socks5MissingServer_ReturnsInvalidConfig(t *testing.T) {
	store := newFakePolicyStore()
	svc := NewOutboundService(store, nil)

	nodeID := uuid.New()
	req := &CreatePolicyRequest{
		PolicyType: "socks5",
		ConfigJSON: Map{"port": 1080}, // 缺少 server
	}
	_, err := svc.Create(context.Background(), nodeID, req)
	if !errors.Is(err, ErrInvalidPolicyConfig) {
		t.Fatalf("expected ErrInvalidPolicyConfig, got %v", err)
	}
}

func TestUpdatePolicy_NotFound_ReturnsPolicyNotFound(t *testing.T) {
	store := newFakePolicyStore()
	svc := NewOutboundService(store, nil)

	enabled := true
	_, err := svc.Update(context.Background(), uuid.New(), &UpdatePolicyRequest{IsEnabled: &enabled})
	if !errors.Is(err, ErrPolicyNotFound) {
		t.Fatalf("expected ErrPolicyNotFound, got %v", err)
	}
}

func TestApplyAll_RendersBothRuntimes(t *testing.T) {
	store := newFakePolicyStore()
	svc := NewOutboundService(store, nil)

	nodeID := uuid.New()
	// 预置两条策略
	p1 := &OutboundPolicy{ID: uuid.New(), NodeID: nodeID, PolicyType: "direct", Priority: 10, IsEnabled: true, ConfigJSON: Map{}}
	p2 := &OutboundPolicy{ID: uuid.New(), NodeID: nodeID, PolicyType: "blackhole", Priority: 20, IsEnabled: true, ConfigJSON: Map{}}
	store.byID[p1.ID] = p1
	store.byID[p2.ID] = p2
	store.byNode[nodeID] = []*OutboundPolicy{p1, p2}

	resp, err := svc.ApplyAll(context.Background(), nodeID)
	if err != nil {
		t.Fatalf("ApplyAll failed: %v", err)
	}
	if len(resp.Xray.Outbounds) < 2 {
		t.Errorf("Xray outbounds = %d, want >= 2", len(resp.Xray.Outbounds))
	}
	if len(resp.SingBox.Outbounds) < 2 {
		t.Errorf("SingBox outbounds = %d, want >= 2", len(resp.SingBox.Outbounds))
	}
	// 确认有 direct 和 block 兜底
	if !hasTag(resp.Xray.Outbounds, "direct") {
		t.Errorf("xray missing direct outbound")
	}
	if !hasTag(resp.SingBox.Outbounds, "block") {
		t.Errorf("sing-box missing block outbound")
	}
}

func TestCreateWarpProfile_Happy(t *testing.T) {
	store := newFakeWarpStore()
	svc := NewWarpProfileService(store, nil)

	isDefault := true
	req := &CreateWarpProfileRequest{
		Code:       "warp-1",
		Name:       "WARP 1",
		WarpMode:   "warp+",
		Endpoint:   "engage.cloudflareclient.com:2408",
		LicenseKey: "key-xxx",
		ConfigJSON: Map{"mtu": 1280},
		IsDefault:  &isDefault,
	}
	w, err := svc.Create(context.Background(), req)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if w.Code != "warp-1" {
		t.Errorf("Code = %q, want warp-1", w.Code)
	}
	if w.WarpMode != "warp+" {
		t.Errorf("WarpMode = %q, want warp+", w.WarpMode)
	}
	if !w.IsDefault {
		t.Errorf("IsDefault = false, want true")
	}
	if w.Endpoint == nil || *w.Endpoint != "engage.cloudflareclient.com:2408" {
		t.Errorf("Endpoint mismatch")
	}
}

func TestCreateWarpProfile_DuplicateCode_ReturnsConflict(t *testing.T) {
	store := newFakeWarpStore()
	existing := &WarpProfile{ID: uuid.New(), Code: "dup", Name: "old"}
	store.byCode["dup"] = existing
	store.byID[existing.ID] = existing

	svc := NewWarpProfileService(store, nil)
	req := &CreateWarpProfileRequest{
		Code: "dup",
		Name: "new",
	}
	_, err := svc.Create(context.Background(), req)
	if !errors.Is(err, ErrWarpProfileExists) {
		t.Fatalf("expected ErrWarpProfileExists, got %v", err)
	}
}

func TestDeletePolicy_Happy(t *testing.T) {
	store := newFakePolicyStore()
	svc := NewOutboundService(store, nil)

	nodeID := uuid.New()
	p := &OutboundPolicy{ID: uuid.New(), NodeID: nodeID, PolicyType: "direct"}
	store.byID[p.ID] = p
	store.byNode[nodeID] = []*OutboundPolicy{p}

	err := svc.Delete(context.Background(), p.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if _, ok := store.byID[p.ID]; ok {
		t.Errorf("policy still exists after delete")
	}
}
