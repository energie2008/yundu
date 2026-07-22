package provider

import (
	"context"
	"testing"
)

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	mock := NewMockProvider()
	r.Register(mock)

	got, err := r.Get("mock")
	if err != nil {
		t.Fatalf("Get(\"mock\") error: %v", err)
	}
	if got != mock {
		t.Error("returned provider is not the same instance")
	}
}

func TestRegistryGetNotFound(t *testing.T) {
	r := NewRegistry()
	_, err := r.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent provider")
	}
}

func TestRegistryTypes(t *testing.T) {
	r := NewRegistry()
	r.Register(NewMockProvider())

	types := r.Types()
	if len(types) != 1 {
		t.Fatalf("expected 1 type, got %d", len(types))
	}
	if types[0] != "mock" {
		t.Errorf("expected type \"mock\", got %q", types[0])
	}
}

func TestMockProviderRegisterAndPush(t *testing.T) {
	ctx := context.Background()
	mock := NewMockProvider()

	// 注册 runtime
	ref, err := mock.RegisterRuntime(ctx, RuntimeSpec{
		ServerCode:  "srv-001",
		RuntimeType: "xray",
		Version:     "1.8.0",
	})
	if err != nil {
		t.Fatalf("RegisterRuntime error: %v", err)
	}
	if ref == "" {
		t.Fatal("runtimeRef should not be empty")
	}

	// 推送配置
	config := `{"inbounds":[]}`
	if err := mock.PushConfig(ctx, ref, config); err != nil {
		t.Fatalf("PushConfig error: %v", err)
	}

	// 验证配置已保存
	got, ok := mock.GetConfig(ref)
	if !ok {
		t.Fatal("config not found after push")
	}
	if got != config {
		t.Errorf("config mismatch: got %q, want %q", got, config)
	}
}

func TestMockProviderPullStats(t *testing.T) {
	ctx := context.Background()
	mock := NewMockProvider()
	ref, _ := mock.RegisterRuntime(ctx, RuntimeSpec{ServerCode: "srv-001"})

	stats, err := mock.PullStats(ctx, ref)
	if err != nil {
		t.Fatalf("PullStats error: %v", err)
	}
	if stats.Status != "running" {
		t.Errorf("expected status \"running\", got %q", stats.Status)
	}
}

func TestMockProviderFetchCapabilities(t *testing.T) {
	ctx := context.Background()
	mock := NewMockProvider()
	caps, err := mock.FetchCapabilities(ctx)
	if err != nil {
		t.Fatalf("FetchCapabilities error: %v", err)
	}
	// mock provider 应该支持所有 6 种能力
	expected := map[string]bool{
		CapConfigPush: false, CapHealthReport: false, CapWarpSidecar: false,
		CapDryRun: false, CapRuntimeUpgrade: false, CapStatsPull: false,
	}
	for _, c := range caps {
		if _, ok := expected[c]; ok {
			expected[c] = true
		}
	}
	for cap, found := range expected {
		if !found {
			t.Errorf("mock provider missing capability: %s", cap)
		}
	}
}

func TestMockProviderShouldFail(t *testing.T) {
	ctx := context.Background()
	mock := NewMockProvider()
	mock.SetShouldFail(true)

	_, err := mock.RegisterRuntime(ctx, RuntimeSpec{ServerCode: "srv-001"})
	if err == nil {
		t.Fatal("expected error when shouldFail=true")
	}
}

func TestThreeXUIProviderCapabilities(t *testing.T) {
	ctx := context.Background()
	p := NewThreeXUIProvider("http://localhost:2053", "admin", "admin")
	caps, err := p.FetchCapabilities(ctx)
	if err != nil {
		t.Fatalf("FetchCapabilities error: %v", err)
	}
	// 3x-ui 只支持 2 种能力
	if len(caps) != 2 {
		t.Fatalf("expected 2 capabilities, got %d", len(caps))
	}
	hasConfigPush := false
	hasStatsPull := false
	for _, c := range caps {
		if c == CapConfigPush {
			hasConfigPush = true
		}
		if c == CapStatsPull {
			hasStatsPull = true
		}
	}
	if !hasConfigPush {
		t.Error("3x-ui should support config_push")
	}
	if !hasStatsPull {
		t.Error("3x-ui should support stats_pull")
	}
}

func TestThreeXUIProviderRegisterUnsupported(t *testing.T) {
	ctx := context.Background()
	p := NewThreeXUIProvider("http://localhost:2053", "admin", "admin")
	_, err := p.RegisterRuntime(ctx, RuntimeSpec{ServerCode: "srv-001"})
	if err == nil {
		t.Fatal("expected error for 3x-ui RegisterRuntime (unsupported)")
	}
}

func TestThreeXUIProviderRollbackUnsupported(t *testing.T) {
	ctx := context.Background()
	p := NewThreeXUIProvider("http://localhost:2053", "admin", "admin")
	err := p.Rollback(ctx, "any-ref")
	if err == nil {
		t.Fatal("expected error for 3x-ui Rollback (unsupported)")
	}
}
