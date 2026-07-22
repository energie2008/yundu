package compat

import (
	"context"
	"errors"
	"testing"
)

// fakeProfileRepo 内存版 ClientProfileRepo 用于测试
type fakeProfileRepo struct {
	profiles map[string]*ClientProfile
	err     error
}

func (f *fakeProfileRepo) GetByCode(ctx context.Context, code string) (*ClientProfile, error) {
	if f.err != nil {
		return nil, f.err
	}
	if p, ok := f.profiles[code]; ok {
		return p, nil
	}
	return nil, nil
}

func (f *fakeProfileRepo) ListAll(ctx context.Context, page, pageSize int, status, code string) ([]*ClientProfile, int, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	var all []*ClientProfile
	for _, p := range f.profiles {
		if code != "" && p.Code != code {
			continue
		}
		if status != "" && p.Status != status {
			continue
		}
		all = append(all, p)
	}
	return all, len(all), nil
}

// fakeMatrixRepo 内存版 CompatMatrixRepo 用于测试
type fakeMatrixRepo struct {
	entries map[string]map[string]*CompatMatrixEntry // client_code -> feature_code -> entry
	err    error
}

func (f *fakeMatrixRepo) GetByClientFeature(ctx context.Context, clientCode, featureCode string) (*CompatMatrixEntry, error) {
	if f.err != nil {
		return nil, f.err
	}
	if m, ok := f.entries[clientCode]; ok {
		if e, ok2 := m[featureCode]; ok2 {
			return e, nil
		}
	}
	return nil, nil
}

func (f *fakeMatrixRepo) ListByClientCode(ctx context.Context, clientCode string) ([]*CompatMatrixEntry, error) {
	if f.err != nil {
		return nil, f.err
	}
	if m, ok := f.entries[clientCode]; ok {
		out := make([]*CompatMatrixEntry, 0, len(m))
		for _, e := range m {
			out = append(out, e)
		}
		return out, nil
	}
	return nil, nil
}

func (f *fakeMatrixRepo) ListAll(ctx context.Context, page, pageSize int, clientCode, featureCode string) ([]*CompatMatrixEntry, int, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	var all []*CompatMatrixEntry
	for _, m := range f.entries {
		for _, e := range m {
			if clientCode != "" && e.ClientCode != clientCode {
				continue
			}
			if featureCode != "" && e.FeatureCode != featureCode {
				continue
			}
			all = append(all, e)
		}
	}
	return all, len(all), nil
}

func (f *fakeMatrixRepo) Upsert(ctx context.Context, entry *CompatMatrixEntry) error {
	if f.err != nil {
		return f.err
	}
	if f.entries == nil {
		f.entries = map[string]map[string]*CompatMatrixEntry{}
	}
	if _, ok := f.entries[entry.ClientCode]; !ok {
		f.entries[entry.ClientCode] = map[string]*CompatMatrixEntry{}
	}
	f.entries[entry.ClientCode][entry.FeatureCode] = entry
	return nil
}

// buildTestService 构造一个 CompatService，注入 fake repos
func buildTestService() (*CompatService, *fakeProfileRepo, *fakeMatrixRepo) {
	encV := "1.10.0"
	echV := "1.11.0"
	profileRepo := &fakeProfileRepo{
		profiles: map[string]*ClientProfile{
			"sing-box": {Code: "sing-box", Name: "Sing-box", Platform: "multi", Status: "active"},
			"clash-meta": {Code: "clash-meta", Name: "Clash Meta", Platform: "multi", Status: "active"},
		},
	}
	matrixRepo := &fakeMatrixRepo{
		entries: map[string]map[string]*CompatMatrixEntry{
			"sing-box": {
				FeatureReality:        {ClientCode: "sing-box", FeatureCode: FeatureReality, Supported: true, SupportedSinceVersion: ptrString("1.9.0")},
				FeatureVLSSEncryption: {ClientCode: "sing-box", FeatureCode: FeatureVLSSEncryption, Supported: true, SupportedSinceVersion: ptrString(encV)},
				FeatureECH:            {ClientCode: "sing-box", FeatureCode: FeatureECH, Supported: true, SupportedSinceVersion: ptrString(echV)},
				FeatureXHTTP:          {ClientCode: "sing-box", FeatureCode: FeatureXHTTP, Supported: true, SupportedSinceVersion: ptrString("1.10.0")},
				FeatureWS:             {ClientCode: "sing-box", FeatureCode: FeatureWS, Supported: true, SupportedSinceVersion: ptrString("1.9.0")},
			},
			"clash-meta": {
				FeatureReality:        {ClientCode: "clash-meta", FeatureCode: FeatureReality, Supported: true, SupportedSinceVersion: ptrString("1.18.0")},
				FeatureVLSSEncryption: {ClientCode: "clash-meta", FeatureCode: FeatureVLSSEncryption, Supported: false},
				FeatureECH:            {ClientCode: "clash-meta", FeatureCode: FeatureECH, Supported: false},
				FeatureXHTTP:          {ClientCode: "clash-meta", FeatureCode: FeatureXHTTP, Supported: true, SupportedSinceVersion: ptrString("1.19.0")},
				FeatureWS:             {ClientCode: "clash-meta", FeatureCode: FeatureWS, Supported: true, SupportedSinceVersion: ptrString("1.18.0")},
			},
		},
	}
	svc := &CompatService{
		profileRepo: profileRepo,
		matrixRepo:  matrixRepo,
	}
	return svc, profileRepo, matrixRepo
}

func ptrString(s string) *string { return &s }

// ===== 测试用例 =====

// TestGetClientFeatures_HappyPath happy path: sing-box 1.10.0 应支持 reality/ws/xhttp，但 ech 需要 1.11.0
func TestGetClientFeatures_HappyPath(t *testing.T) {
	svc, _, _ := buildTestService()

	features, err := svc.GetClientFeatures(context.Background(), "sing-box", "1.10.0")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expect := map[string]bool{
		FeatureReality:        true,
		FeatureVLSSEncryption: true,
		FeatureXHTTP:          true,
		FeatureWS:             true,
		FeatureECH:            false, // 1.10.0 < 1.11.0
	}
	got := map[string]bool{}
	for _, f := range features {
		got[f] = true
	}
	for code, want := range expect {
		if want != got[code] {
			t.Errorf("feature %s: want supported=%v, got in list=%v", code, want, got[code])
		}
	}
}

// TestGetClientFeatures_ClientNotFound error path: 未知客户端应返回 ErrClientNotFound
func TestGetClientFeatures_ClientNotFound(t *testing.T) {
	svc, _, _ := buildTestService()

	_, err := svc.GetClientFeatures(context.Background(), "unknown-client", "1.0.0")
	if !errors.Is(err, ErrClientNotFound) {
		t.Fatalf("expected ErrClientNotFound, got %v", err)
	}
}

// TestFilterNodeForClient_RealityUnsupported clash-meta 不被过滤 REALITY 节点（支持）
func TestFilterNodeForClient_RealitySupported(t *testing.T) {
	svc, _, _ := buildTestService()

	node := map[string]interface{}{
		"protocol_type":  "vless",
		"transport_type": "tcp",
		"security_type":  "reality",
	}
	ok, reason, err := svc.FilterNodeForClient(context.Background(), node, "clash-meta", "1.18.0")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !ok {
		t.Errorf("expected node compatible, but filtered: %s", reason)
	}
}

// TestRenderWithCompat_VLSSEncryptionDowngrade clash-meta 不支持 vless_encryption，应降级为 none
func TestRenderWithCompat_VLSSEncryptionDowngrade(t *testing.T) {
	svc, _, _ := buildTestService()

	node := map[string]interface{}{
		"protocol_type":  "vless",
		"transport_type": "ws",
		"security_type":  "tls",
		"encryption":     "xchacha20-poly1305",
		"ech":            map[string]interface{}{"enabled": true},
	}
	out, warnings, err := svc.RenderWithCompat(context.Background(), node, "clash-meta", "1.18.0")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out["encryption"] != "none" {
		t.Errorf("expected encryption downgraded to none, got %v", out["encryption"])
	}
	if _, ok := out["ech"]; ok {
		t.Errorf("expected ech removed, but still present")
	}
	if len(warnings) == 0 {
		t.Errorf("expected warnings, got none")
	}
	// 确保不修改原 map
	if node["encryption"] != "xchacha20-poly1305" {
		t.Errorf("original node map was modified")
	}
}

// TestBatchUpdateMatrix_HappyPath happy path
func TestBatchUpdateMatrix_HappyPath(t *testing.T) {
	svc, _, _ := buildTestService()

	req := &CompatMatrixBatchUpdateRequest{
		Entries: []CompatMatrixUpdateEntry{
			{
				ClientCode:            "clash-meta",
				FeatureCode:           FeatureVLSSEncryption,
				Supported:             true,
				SupportedSinceVersion: ptrString("1.20.0"),
			},
		},
	}
	updated, err := svc.BatchUpdateMatrix(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if updated != 1 {
		t.Errorf("expected updated=1, got %d", updated)
	}
}

// TestBatchUpdateMatrix_InvalidEntry error path
func TestBatchUpdateMatrix_InvalidEntry(t *testing.T) {
	svc, _, _ := buildTestService()

	req := &CompatMatrixBatchUpdateRequest{
		Entries: []CompatMatrixUpdateEntry{
			{ClientCode: "", FeatureCode: FeatureReality, Supported: true},
		},
	}
	_, err := svc.BatchUpdateMatrix(context.Background(), req)
	if !errors.Is(err, ErrMatrixEntryInvalid) {
		t.Fatalf("expected ErrMatrixEntryInvalid, got %v", err)
	}
}

// TestCompareVersions 验证简单版本比较
func TestCompareVersions(t *testing.T) {
	cases := []struct {
		v1, v2 string
		want    int
	}{
		{"1.10.0", "1.9.0", 1},
		{"1.9.0", "1.10.0", -1},
		{"1.9.0", "1.9.0", 0},
		{"2.0.0", "1.99.99", 1},
		{"1.18.0", "1.18", 0},   // "1.18" 视为 1.18.0
		{"v1.18.0", "1.18.0", 0},
		{"1.10.0-beta", "1.10.0", 0}, // 非数字截断
	}
	for _, c := range cases {
		got := compareVersions(c.v1, c.v2)
		if got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.v1, c.v2, got, c.want)
		}
	}
}

// TestSyncFromSource 验证未配置时返回 synced=0
func TestSyncFromSource(t *testing.T) {
	svc, _, _ := buildTestService()

	resp, err := svc.SyncFromSource(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.Synced != 0 {
		t.Errorf("expected synced=0, got %d", resp.Synced)
	}
	if resp.Message == "" {
		t.Errorf("expected non-empty message")
	}
}
