package service

import (
	"testing"

	"github.com/airport-panel/node-service/internal/model"
)

// 阶段2.1: 混合节点冲突回归测试
// 场景：节点同时配置 cdn_address 和 cloudflared_tunnel_id
// 期望：determineExposureMode 必须返回 argo_tunnel（优先级高于 cdn_saas）
// 防止未来重构时优先级被误改，导致 Argo 隧道套 CDN 优选 IP 的组合方案失效
func TestDetermineExposureMode_MixedConflict(t *testing.T) {
	tests := []struct {
		name     string
		node     *model.Node
		expected string
	}{
		{
			name: "cdn_address + tunnel_id 同时存在 → argo_tunnel",
			node: &model.Node{
				ConfigJSON: map[string]interface{}{
					"cdn_address":            "example.yundu.space",
					"cloudflared_tunnel_id":  "d7105b20-8f1b-4772-988b-c8f5c3e87982",
				},
			},
			expected: "argo_tunnel",
		},
		{
			name: "cdn_address + tunnel_token 同时存在 → argo_tunnel",
			node: &model.Node{
				ConfigJSON: map[string]interface{}{
					"cdn_address":              "example.yundu.space",
					"cloudflared_tunnel_token": "eyJhIjoiAAA",
				},
			},
			expected: "argo_tunnel",
		},
		{
			name: "仅 cdn_address → cdn_saas",
			node: &model.Node{
				ConfigJSON: map[string]interface{}{
					"cdn_address": "example.yundu.space",
				},
			},
			expected: "cdn_saas",
		},
		{
			name: "仅 tunnel_id → argo_tunnel",
			node: &model.Node{
				ConfigJSON: map[string]interface{}{
					"cloudflared_tunnel_id": "d7105b20-8f1b-4772-988b-c8f5c3e87982",
				},
			},
			expected: "argo_tunnel",
		},
		{
			name: "前端显式 exposure_mode=cdn_saas + tunnel_id → 保留 cdn_saas",
			node: &model.Node{
				ConfigJSON: map[string]interface{}{
					"exposure_mode":          "cdn_saas",
					"cloudflared_tunnel_id":  "d7105b20-8f1b-4772-988b-c8f5c3e87982",
				},
			},
			expected: "cdn_saas",
		},
		{
			name: "无任何 CDN 字段 → direct",
			node: &model.Node{
				ConfigJSON: map[string]interface{}{},
			},
			expected: "direct",
		},
		{
			name:     "nil ConfigJSON → direct",
			node:     &model.Node{},
			expected: "direct",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := determineExposureMode(tt.node)
			if got != tt.expected {
				t.Errorf("determineExposureMode() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// 阶段2.1: isArgoTunnelExposureNode 判定测试
func TestIsArgoTunnelExposureNode(t *testing.T) {
	tests := []struct {
		name     string
		node     *model.Node
		expected bool
	}{
		{
			name: "argo_tunnel 显式标记 → true",
			node: &model.Node{
				ConfigJSON: map[string]interface{}{
					"exposure_mode": "argo_tunnel",
				},
			},
			expected: true,
		},
		{
			name: "有 tunnel_id → true",
			node: &model.Node{
				ConfigJSON: map[string]interface{}{
					"cloudflared_tunnel_id": "d7105b20-8f1b-4772-988b-c8f5c3e87982",
				},
			},
			expected: true,
		},
		{
			name: "仅 cdn_address → false",
			node: &model.Node{
				ConfigJSON: map[string]interface{}{
					"cdn_address": "example.yundu.space",
				},
			},
			expected: false,
		},
		{
			name:     "空节点 → false",
			node:     &model.Node{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isArgoTunnelExposureNode(tt.node)
			if got != tt.expected {
				t.Errorf("isArgoTunnelExposureNode() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// 阶段2.1: isArgoTunnelNode（渲染层判定）测试
// 确保渲染层只看 exposure_mode 字段，与 determineExposureMode 的推断结果一致
func TestIsArgoTunnelNode(t *testing.T) {
	tests := []struct {
		name     string
		node     *model.Node
		expected bool
	}{
		{
			name: "config_json.exposure_mode=argo_tunnel → true",
			node: &model.Node{
				ConfigJSON: map[string]interface{}{
					"exposure_mode": "argo_tunnel",
				},
			},
			expected: true,
		},
		{
			name: "metadata.exposure_mode=argo_tunnel → true",
			node: &model.Node{
				Metadata: map[string]interface{}{
					"exposure_mode": "argo_tunnel",
				},
			},
			expected: true,
		},
		{
			name: "仅 tunnel_id 但无 exposure_mode → false",
			node: &model.Node{
				ConfigJSON: map[string]interface{}{
					"cloudflared_tunnel_id": "d7105b20",
				},
			},
			expected: false,
		},
		{
			name:     "nil 节点 → false",
			node:     nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isArgoTunnelNode(tt.node)
			if got != tt.expected {
				t.Errorf("isArgoTunnelNode() = %v, want %v", got, tt.expected)
			}
		})
	}
}
