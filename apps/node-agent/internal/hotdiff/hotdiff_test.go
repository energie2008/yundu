package hotdiff

import (
	"testing"
)

func TestComputeHotDiff_UserOnly(t *testing.T) {
	old := baseConfig()
	new := baseConfig()
	// 仅在 clients 中新增一个用户
	setClients(new, "inb-1", []interface{}{
		map[string]interface{}{"email": "user1@example.com", "id": "uuid-1", "level": float64(0)},
		map[string]interface{}{"email": "user2@example.com", "id": "uuid-2", "level": float64(0)},
	})

	result := ComputeHotDiff(old, new)
	if result.Level != DiffHotUserOnly {
		t.Errorf("expected %s, got %s (fields: %v, summary: %s)",
			DiffHotUserOnly, result.Level, result.ChangedFields, result.Summary)
	}
}

func TestComputeHotDiff_PortChange(t *testing.T) {
	old := baseConfig()
	new := baseConfig()
	// 修改端口
	setInboundField(new, "inb-1", "port", float64(8443))

	result := ComputeHotDiff(old, new)
	if result.Level != DiffRestartNeeded {
		t.Errorf("expected %s, got %s (fields: %v, summary: %s)",
			DiffRestartNeeded, result.Level, result.ChangedFields, result.Summary)
	}
}

func TestComputeHotDiff_RoutingOnly(t *testing.T) {
	old := baseConfig()
	new := baseConfig()
	// 仅修改路由规则
	new["routing"] = map[string]interface{}{
		"domainStrategy": "AsIs",
		"rules": []interface{}{
			map[string]interface{}{
				"type":        "field",
				"outboundTag": "block",
				"domain":      []interface{}{"geosite:category-ads-all"},
			},
		},
	}

	result := ComputeHotDiff(old, new)
	if result.Level != DiffHotRoutingOnly {
		t.Errorf("expected %s, got %s (fields: %v, summary: %s)",
			DiffHotRoutingOnly, result.Level, result.ChangedFields, result.Summary)
	}
}

func TestComputeHotDiff_TLSReload(t *testing.T) {
	old := baseConfig()
	new := baseConfig()
	// 仅修改 TLS 证书路径
	setStreamSettings(new, "inb-1", map[string]interface{}{
		"network":  "tcp",
		"security": "tls",
		"tls": map[string]interface{}{
			"certificates": []interface{}{
				map[string]interface{}{
					"certificateFile": "/new/cert.pem",
					"keyFile":         "/new/key.pem",
				},
			},
		},
	})

	result := ComputeHotDiff(old, new)
	if result.Level != DiffHotTLSReload {
		t.Errorf("expected %s, got %s (fields: %v, summary: %s)",
			DiffHotTLSReload, result.Level, result.ChangedFields, result.Summary)
	}
}

func TestComputeHotDiff_NoChange(t *testing.T) {
	old := baseConfig()
	new := baseConfig()

	result := ComputeHotDiff(old, new)
	if result.Level != "" {
		t.Errorf("expected empty level, got %s (fields: %v, summary: %s)",
			result.Level, result.ChangedFields, result.Summary)
	}
	if len(result.ChangedFields) != 0 {
		t.Errorf("expected no changed fields, got %v", result.ChangedFields)
	}
}

func TestComputeHotDiff_FirstDeployment(t *testing.T) {
	result := ComputeHotDiff(nil, baseConfig())
	if result.Level != DiffRestartNeeded {
		t.Errorf("expected %s for first deployment, got %s",
			DiffRestartNeeded, result.Level)
	}
}

func TestComputeHotDiff_BothEmpty(t *testing.T) {
	result := ComputeHotDiff(nil, nil)
	if result.Level != "" {
		t.Errorf("expected empty level for both nil, got %s", result.Level)
	}
}

// --- 测试辅助函数 ---

func baseConfig() map[string]interface{} {
	return map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{
				"tag":      "inb-1",
				"port":     float64(443),
				"protocol": "vless",
				"settings": map[string]interface{}{
					"clients": []interface{}{
						map[string]interface{}{
							"email": "user1@example.com",
							"id":    "uuid-1",
							"level": float64(0),
						},
					},
				},
				"streamSettings": map[string]interface{}{
					"network":  "tcp",
					"security": "tls",
					"tls": map[string]interface{}{
						"certificates": []interface{}{
							map[string]interface{}{
								"certificateFile": "/old/cert.pem",
								"keyFile":         "/old/key.pem",
							},
						},
					},
				},
			},
		},
		"routing": map[string]interface{}{
			"domainStrategy": "AsIs",
			"rules": []interface{}{
				map[string]interface{}{
					"type":        "field",
					"outboundTag": "direct",
					"domain":      []interface{}{"geosite:cn"},
				},
			},
		},
		"outbounds": []interface{}{
			map[string]interface{}{"tag": "direct", "protocol": "freedom"},
			map[string]interface{}{"tag": "block", "protocol": "blackhole"},
		},
	}
}

func setClients(cfg map[string]interface{}, tag string, clients []interface{}) {
	inbounds := cfg["inbounds"].([]interface{})
	for _, inb := range inbounds {
		m := inb.(map[string]interface{})
		if m["tag"] == tag {
			m["settings"] = map[string]interface{}{"clients": clients}
			return
		}
	}
}

func setInboundField(cfg map[string]interface{}, tag, field string, value interface{}) {
	inbounds := cfg["inbounds"].([]interface{})
	for _, inb := range inbounds {
		m := inb.(map[string]interface{})
		if m["tag"] == tag {
			m[field] = value
			return
		}
	}
}

func setStreamSettings(cfg map[string]interface{}, tag string, ss map[string]interface{}) {
	inbounds := cfg["inbounds"].([]interface{})
	for _, inb := range inbounds {
		m := inb.(map[string]interface{})
		if m["tag"] == tag {
			m["streamSettings"] = ss
			return
		}
	}
}
