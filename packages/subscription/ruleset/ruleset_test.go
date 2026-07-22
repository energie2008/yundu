package ruleset

import "testing"

func TestRuleSetValidation(t *testing.T) {
	t.Run("default final is proxy", func(t *testing.T) {
		rs := &RuleSet{
			ID:   "test-empty",
			Name: "Empty RuleSet",
		}
		if err := rs.Validate(); err != nil {
			t.Fatalf("Validate failed: %v", err)
		}
		if rs.Final != ActionProxy {
			t.Errorf("expected Final=ActionProxy, got %s", rs.Final)
		}
	})

	t.Run("group action requires tag", func(t *testing.T) {
		rs := &RuleSet{
			ID:    "test-group-notag",
			Name:  "Invalid Group",
			Rules: []Rule{{Type: "geosite", Value: "netflix", Action: ActionGroup}},
		}
		err := rs.Validate()
		if err == nil {
			t.Fatal("expected validation error for group without tag")
		}
	})

	t.Run("group action with tag passes", func(t *testing.T) {
		rs := &RuleSet{
			ID:   "test-group-ok",
			Name: "Valid Group",
			Rules: []Rule{
				{Type: "geosite", Value: "netflix", Action: ActionGroup, GroupTag: "streaming"},
			},
		}
		if err := rs.Validate(); err != nil {
			t.Fatalf("Validate failed: %v", err)
		}
	})

	t.Run("invalid rule type fails", func(t *testing.T) {
		rs := &RuleSet{
			ID:    "test-invalid-type",
			Name:  "Invalid Type",
			Rules: []Rule{{Type: "invalid_type", Value: "x", Action: ActionDirect}},
		}
		err := rs.Validate()
		if err == nil {
			t.Fatal("expected validation error for invalid type")
		}
	})
}

func TestNodeGroupValidation(t *testing.T) {
	t.Run("tag required", func(t *testing.T) {
		ng := &NodeGroup{NodeIDs: []string{"n1"}}
		if err := ng.Validate(); err == nil {
			t.Fatal("expected error for empty tag")
		}
	})

	t.Run("node ids required", func(t *testing.T) {
		ng := &NodeGroup{Tag: "g1"}
		if err := ng.Validate(); err == nil {
			t.Fatal("expected error for empty node ids")
		}
	})

	t.Run("valid group passes", func(t *testing.T) {
		ng := &NodeGroup{Tag: "g1", NodeIDs: []string{"n1", "n2"}, Strategy: "round_robin"}
		if err := ng.Validate(); err != nil {
			t.Fatalf("Validate failed: %v", err)
		}
	})
}

func TestDefaultChinaDirect(t *testing.T) {
	rs := &RuleSet{
		ID:      "china-direct",
		Name:    "Domestic Direct + Ad Block + Proxy Others",
		Version: 1,
		Rules: []Rule{
			{Type: "geosite", Value: "category-ads-all", Action: ActionBlock},
			{Type: "geosite", Value: "cn", Action: ActionDirect},
			{Type: "geoip", Value: "cn", Action: ActionDirect},
			{Type: "geoip", Value: "private", Action: ActionDirect},
			{Type: "domain_suffix", Value: "baidu.com", Action: ActionDirect},
			{Type: "domain_suffix", Value: "aliyun.com", Action: ActionDirect},
			{Type: "port", Value: "22", Action: ActionDirect},
			{Type: "geosite", Value: "netflix", Action: ActionGroup, GroupTag: "streaming-unlock"},
			{Type: "geosite", Value: "disney", Action: ActionGroup, GroupTag: "streaming-unlock"},
		},
		DNSRules: []DNSRule{
			{Type: "domain", Domain: "geosite:cn", Server: "223.5.5.5"},
		},
	}

	if err := rs.Validate(); err != nil {
		t.Fatalf("china_direct ruleset validation failed: %v", err)
	}

	if rs.Final != ActionProxy {
		t.Errorf("expected Final=ActionProxy by default, got %s", rs.Final)
	}

	groups := map[string]NodeGroup{
		"streaming-unlock": {
			Tag:      "streaming-unlock",
			Name:     "Streaming Unlock",
			NodeIDs:  []string{"node-hk-1", "node-jp-1", "node-us-1"},
			Strategy: "least_ping",
		},
		"proxy": {
			Tag:      "proxy",
			Name:     "Default Proxy",
			NodeIDs:  []string{"node-sg-1", "node-us-2"},
			Strategy: "round_robin",
		},
	}

	for tag, g := range groups {
		if err := g.Validate(); err != nil {
			t.Fatalf("group %s validation failed: %v", tag, err)
		}
	}
}
