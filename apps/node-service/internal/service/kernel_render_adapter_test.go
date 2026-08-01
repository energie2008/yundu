package service

import (
	"testing"
)

func TestMergeRenderedLimiterMeta(t *testing.T) {
	// kernelrender 内存直出：users 为 []map[string]interface{}
	inMemory := map[string]interface{}{
		"users": []map[string]interface{}{
			{
				"email":            "a@example.com",
				"uuid":             "11111111-1111-1111-1111-111111111111",
				"speed_limit_mbps": 50,
				"device_limit":     5,
			},
		},
	}
	// JSON 往返：users 为 []interface{}
	jsonRoundTrip := map[string]interface{}{
		"users": []interface{}{
			map[string]interface{}{
				"email":            "b@example.com",
				"uuid":             "22222222-2222-2222-2222-222222222222",
				"speed_limit_mbps": float64(100),
				"device_limit":     float64(3),
			},
		},
		"node_device_limit": 10,
	}

	merged := mergeRenderedLimiterMeta([]interface{}{inMemory, jsonRoundTrip})
	if merged == nil {
		t.Fatal("mergeRenderedLimiterMeta returned nil")
	}
	m, ok := merged.(map[string]interface{})
	if !ok {
		t.Fatalf("merged type = %T", merged)
	}
	users, ok := m["users"].([]interface{})
	if !ok || len(users) != 2 {
		t.Fatalf("merged users = %#v, want 2 entries", m["users"])
	}
	if v := metaInt(m["node_device_limit"]); v != 10 {
		t.Fatalf("node_device_limit = %d, want 10", v)
	}
}

func TestMergeRenderedLimiterMetaNilWhenNoLimits(t *testing.T) {
	meta := map[string]interface{}{
		"users": []map[string]interface{}{
			{"email": "a@example.com", "uuid": "11111111-1111-1111-1111-111111111111"},
		},
	}
	if got := mergeRenderedLimiterMeta([]interface{}{meta}); got != nil {
		t.Fatalf("expected nil for no limits, got %#v", got)
	}
}
