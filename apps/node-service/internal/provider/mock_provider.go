package provider

import (
	"context"
	"fmt"
	"sync"
)

// MockProvider 用于单元测试和本地开发，不依赖任何真实后端。
type MockProvider struct {
	mu              sync.Mutex
	capabilities    []string
	runtimes        map[string]string // runtimeRef -> config
	stats           map[string]*RuntimeStats
	shouldFail      bool
}

func NewMockProvider() *MockProvider {
	return &MockProvider{
		capabilities: []string{
			CapConfigPush, CapHealthReport, CapWarpSidecar,
			CapDryRun, CapRuntimeUpgrade, CapStatsPull,
		},
		runtimes: make(map[string]string),
		stats:    make(map[string]*RuntimeStats),
	}
}

func (m *MockProvider) Type() string { return "mock" }

func (m *MockProvider) RegisterRuntime(ctx context.Context, spec RuntimeSpec) (string, error) {
	if m.shouldFail {
		return "", fmt.Errorf("mock: register failed (shouldFail=true)")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	ref := fmt.Sprintf("mock-runtime-%s", spec.ServerCode)
	m.runtimes[ref] = ""
	m.stats[ref] = &RuntimeStats{OnlineUsers: 0, Status: "running"}
	return ref, nil
}

func (m *MockProvider) PushConfig(ctx context.Context, runtimeRef string, config string) error {
	if m.shouldFail {
		return fmt.Errorf("mock: push config failed")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.runtimes[runtimeRef]; !ok {
		return fmt.Errorf("mock: runtime not found: %s", runtimeRef)
	}
	m.runtimes[runtimeRef] = config
	return nil
}

func (m *MockProvider) PullStats(ctx context.Context, runtimeRef string) (*RuntimeStats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.stats[runtimeRef]
	if !ok {
		return nil, fmt.Errorf("mock: stats not found for runtime: %s", runtimeRef)
	}
	return s, nil
}

func (m *MockProvider) Reload(ctx context.Context, runtimeRef string) error {
	if m.shouldFail {
		return fmt.Errorf("mock: reload failed")
	}
	return nil
}

func (m *MockProvider) Rollback(ctx context.Context, runtimeRef string) error {
	return nil
}

func (m *MockProvider) FetchCapabilities(ctx context.Context) ([]string, error) {
	return m.capabilities, nil
}

// SetShouldFail 测试用：控制 MockProvider 是否返回错误
func (m *MockProvider) SetShouldFail(v bool) {
	m.shouldFail = v
}

// GetConfig 测试用：读取已推送的配置
func (m *MockProvider) GetConfig(runtimeRef string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.runtimes[runtimeRef]
	return c, ok
}
