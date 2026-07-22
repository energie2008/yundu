package service

import (
	"context"
	"testing"

	"github.com/airport-panel/node-service/internal/model"
)

// TestEnsureREALITYKeys_GeneratesWhenMissing 验证 autoGenerateREALITYKeys
// 为缺少 private_key 的 REALITY 节点自动补全密钥。
// Bug-B1: xray_config.go:599 硬编码 REALITY 私钥 cHAWz_DP00iHGudE9Uq-8txkbwiZGCTAV1GvDQ8Z7U4,
// 所有未配置密钥的 REALITY 节点共享同一密钥对，构成安全风险。
// 修复后: CreateNode/UpdateNode 自动调用 GenerateREALITYKeypair() 生成独立密钥对。
func TestEnsureREALITYKeys_GeneratesWhenMissing(t *testing.T) {
	tests := []struct {
		name        string
		securityType *string
		configJSON  map[string]interface{}
		wantHasKey  bool
	}{
		{
			name:        "REALITY node with no private_key",
			securityType: ptrString("reality"),
			configJSON:   map[string]interface{}{"sni": "rust-lang.org"},
			wantHasKey:  true,
		},
		{
			name: "REALITY node with existing private_key (should not overwrite)",
			securityType: ptrString("reality"),
			configJSON: map[string]interface{}{
				"private_key": "existing_key_12345",
				"sni":         "rust-lang.org",
			},
			wantHasKey: true,
		},
		{
			name:        "TLS node (should not touch config)",
			securityType: ptrString("tls"),
			configJSON:   map[string]interface{}{"sni": "example.com"},
			wantHasKey:  false,
		},
		{
			// 嵌套路径中的 private_key (reality.private_key)
			name: "REALITY with nested private_key (reality.private_key)",
			securityType: ptrString("reality"),
			configJSON: map[string]interface{}{
				"reality": map[string]interface{}{
                    "private_key": "nested_key_abcde",
                },
			},
			wantHasKey: true,
		},
		{
			// 嵌套路径中的 private_key (reality_settings.private_key)
			name: "REALITY with deeply nested private_key (reality_settings.private_key)",
			securityType: ptrString("reality"),
			configJSON: map[string]interface{}{
				"reality_settings": map[string]interface{}{
                    "private_key": "deep_key_xyz",
                },
			},
			wantHasKey: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			// 构建一个虚拟节点
			node := &model.Node{
				SecurityType: tc.securityType,
				ConfigJSON:   tc.configJSON,
			}

			// 调用 autoGenerateREALITYKeys
			err := autoGenerateREALITYKeys(ctx, node)
			if err != nil {
				t.Fatalf("autoGenerateREALITYKeys returned error: %v", err)
			}

			// 验证
			if !tc.wantHasKey {
				return
			}

			// 检查 private_key 是否存在于顶层或嵌套路径
			hasKey := false
			if v, ok := node.ConfigJSON["private_key"]; ok && v != "" {
				hasKey = true
			}
			if !hasKey {
				if reality, ok := node.ConfigJSON["reality"].(map[string]interface{}); ok {
                    if pk, ok := reality["private_key"]; ok && pk != "" {
						hasKey = true
                    }
                }
			}
			if !hasKey {
				if rs, ok := node.ConfigJSON["reality_settings"].(map[string]interface{}); ok {
                    if pk, ok := rs["private_key"]; ok && pk != "" {
						hasKey = true
                    }
                }
            }

			if !hasKey {
				t.Errorf("REALITY node should have private_key after auto-generation, ConfigJSON: %+v", node.ConfigJSON)
			}

			// 关键: private_key 不应是已知的硬编码值
			privKey := ""
			if v, ok := node.ConfigJSON["private_key"]; ok {
				privKey = v.(string)
			} else if reality, ok := node.ConfigJSON["reality"].(map[string]interface{}); ok {
				if pk, ok := reality["private_key"]; ok {
					privKey = pk.(string)
				}
			} else if rs, ok := node.ConfigJSON["reality_settings"].(map[string]interface{}); ok {
				if pk, ok := rs["private_key"]; ok {
					privKey = pk.(string)
				}
			}

			// 如果原始配置已有 private_key，保持原值
			originalKey := ""
			if v, ok := tc.configJSON["private_key"]; ok {
				originalKey = v.(string)
			}
			if reality, ok := tc.configJSON["reality"].(map[string]interface{}); ok {
				if pk, ok := reality["private_key"]; ok {
					originalKey = pk.(string)
				}
			} else if rs, ok := tc.configJSON["reality_settings"].(map[string]interface{}); ok {
				if pk, ok := rs["private_key"]; ok {
                    originalKey = pk.(string)
                }
            }

			if originalKey != "" && privKey != originalKey {
				t.Errorf("existing private_key %q should be preserved, got %q", originalKey, privKey)
			}

			// 如果没有原始密钥，生成的密钥不应是硬编码值
			if originalKey == "" {
				if privKey == "cHAWz_DP00iHGudE9Uq-8txkbwiZGCTAV1GvDQ8Z7U4" {
                    t.Error("auto-generated private_key should NOT be the hardcoded legacy value")
                }
				if privKey == "" {
					t.Error("auto-generated private_key should not be empty")
				}
			}

            // 验证 public_key 也已生成
            hasPubKey := false
            if v, ok := node.ConfigJSON["public_key"]; ok && v != "" {
                hasPubKey = true
            }
            if !hasPubKey && privKey != "" && originalKey == "" {
                t.Error("public_key should be generated alongside private_key")
            }
		})
	}
}

func ptrString(s string) *string { return &s }
