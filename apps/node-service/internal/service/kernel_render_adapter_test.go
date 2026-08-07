package service

import (
	"fmt"
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

func TestMergeRenderedLimiterMetaDeterministicOrder(t *testing.T) {
	users := map[string]interface{}{
		"users": []interface{}{
			map[string]interface{}{
				"email":            "c@example.com",
				"uuid":             "33333333-3333-3333-3333-333333333333",
				"speed_limit_mbps": float64(20),
				"device_limit":     float64(2),
			},
			map[string]interface{}{
				"email":            "a@example.com",
				"uuid":             "11111111-1111-1111-1111-111111111111",
				"speed_limit_mbps": float64(50),
				"device_limit":     float64(5),
			},
			map[string]interface{}{
				"email":            "b@example.com",
				"uuid":             "22222222-2222-2222-2222-222222222222",
				"speed_limit_mbps": float64(10),
				"device_limit":     float64(1),
			},
		},
	}

	first := mergeRenderedLimiterMeta([]interface{}{users})
	second := mergeRenderedLimiterMeta([]interface{}{users})
	if fmt.Sprint(first) != fmt.Sprint(second) {
		t.Fatalf("merge output is not deterministic:\nfirst: %#v\nsecond: %#v", first, second)
	}

	m, ok := first.(map[string]interface{})
	if !ok {
		t.Fatalf("merged type = %T", first)
	}
	out, ok := m["users"].([]interface{})
	if !ok || len(out) != 3 {
		t.Fatalf("merged users = %#v, want 3 entries", m["users"])
	}
	gotEmails := make([]string, 0, 3)
	for _, u := range out {
		um := u.(map[string]interface{})
		gotEmails = append(gotEmails, um["email"].(string))
	}
	want := []string{"a@example.com", "b@example.com", "c@example.com"}
	for i := range want {
		if gotEmails[i] != want[i] {
			t.Fatalf("users order = %v, want %v", gotEmails, want)
		}
	}
}
