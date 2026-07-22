package renderer

import (
	"context"
	"errors"
	"testing"
)

// fakeCompatFilter 测试用 CompatFilter 实现
type fakeCompatFilter struct {
	filterOK       bool
	filterReason   string
	filterErr      error
	renderOut      map[string]interface{}
	renderWarnings []string
	renderErr      error
}

func (f *fakeCompatFilter) FilterNodeForClient(ctx context.Context, node map[string]interface{}, clientCode, version string) (bool, string, error) {
	return f.filterOK, f.filterReason, f.filterErr
}

func (f *fakeCompatFilter) RenderWithCompat(ctx context.Context, node map[string]interface{}, clientCode, version string) (map[string]interface{}, []string, error) {
	if f.renderErr != nil {
		return nil, nil, f.renderErr
	}
	if f.renderOut != nil {
		return f.renderOut, f.renderWarnings, nil
	}
	// 默认返回原节点的浅拷贝
	out := map[string]interface{}{}
	for k, v := range node {
		out[k] = v
	}
	return out, f.renderWarnings, nil
}

// TestRender_DowngradeHidden 测试 REALITY 不支持时降级标记 hidden
func TestRender_DowngradeHidden(t *testing.T) {
	// 场景：节点是 REALITY 节点，compat FilterNodeForClient 仍返回 ok=true（兼容），
	// 但 RenderWithCompat 返回带 _hidden=true 的降级节点 + warning
	fake := &fakeCompatFilter{
		filterOK:     true,
		filterReason: "",
		renderOut: map[string]interface{}{
			"protocol_type":  "vless",
			"transport_type": "tcp",
			"security_type":  "reality",
			"_hidden":         true,
			"_hidden_reason": "REALITY not supported by this client",
		},
		renderWarnings: []string{"marked node as hidden (REALITY not supported)"},
	}

	r := newRendererWithFilter(fake)

	node := map[string]interface{}{
		"protocol_type":  "vless",
		"transport_type": "tcp",
		"security_type":  "reality",
	}

	out, warnings := r.Render(context.Background(), node, "clash-meta", "1.18.0")
	if out == nil {
		t.Fatalf("expected non-nil node output")
	}
	if hidden, _ := out["_hidden"].(bool); !hidden {
		t.Errorf("expected node to be marked as _hidden=true, got %v", out["_hidden"])
	}
	if len(warnings) == 0 {
		t.Errorf("expected warnings, got none")
	}
}

// TestRender_FilteredOut 节点被过滤时应返回 nil + warning
func TestRender_FilteredOut(t *testing.T) {
	fake := &fakeCompatFilter{
		filterOK:     false,
		filterReason: "REALITY not supported by this client",
	}
	r := newRendererWithFilter(fake)

	node := map[string]interface{}{
		"protocol_type":  "vless",
		"security_type":  "reality",
	}
	out, warnings := r.Render(context.Background(), node, "clash-meta", "1.18.0")
	if out != nil {
		t.Errorf("expected nil node when filtered out, got %v", out)
	}
	if len(warnings) == 0 {
		t.Errorf("expected warnings when filtered out")
	}
}

// TestRender_NilCompat 无 compat 依赖时直接返回原节点
func TestRender_NilCompat(t *testing.T) {
	r := &NodeRenderer{}
	node := map[string]interface{}{"protocol_type": "vless"}
	out, warnings := r.Render(context.Background(), node, "clash-meta", "1.18.0")
	if out == nil {
		t.Fatalf("expected non-nil node output when compat is nil")
	}
	if warnings != nil {
		t.Errorf("expected nil warnings, got %v", warnings)
	}
}

// TestRender_FilterError compat 过滤出错时降级为原节点 + warning
func TestRender_FilterError(t *testing.T) {
	fake := &fakeCompatFilter{
		filterOK:  false,
		filterErr: errors.New("db connection lost"),
	}
	r := newRendererWithFilter(fake)
	node := map[string]interface{}{"protocol_type": "vless"}
	out, warnings := r.Render(context.Background(), node, "clash-meta", "1.18.0")
	if out == nil {
		t.Fatalf("expected non-nil node output (degraded)")
	}
	if len(warnings) == 0 {
		t.Errorf("expected warnings on filter error")
	}
}
