package exposure

import (
	"encoding/json"
	"testing"

	"github.com/airport-panel/subscription/ruleset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newChinaDirectRuleSet() *ruleset.RuleSet {
	return &ruleset.RuleSet{
		ID:      "china-direct",
		Name:    "Domestic Direct + Ad Block + Streaming Group + Proxy Others",
		Version: 1,
		Rules: []ruleset.Rule{
			{Type: "geosite", Value: "category-ads-all", Action: ruleset.ActionBlock},
			{Type: "geosite", Value: "cn", Action: ruleset.ActionDirect},
			{Type: "geoip", Value: "cn", Action: ruleset.ActionDirect},
			{Type: "geoip", Value: "private", Action: ruleset.ActionDirect},
			{Type: "domain_suffix", Value: "baidu.com", Action: ruleset.ActionDirect},
			{Type: "domain_suffix", Value: "aliyun.com", Action: ruleset.ActionDirect},
			{Type: "port", Value: "22", Action: ruleset.ActionDirect},
			{Type: "geosite", Value: "netflix", Action: ruleset.ActionGroup, GroupTag: "streaming-unlock"},
			{Type: "geosite", Value: "disney", Action: ruleset.ActionGroup, GroupTag: "streaming-unlock"},
		},
	}
}

func newTestGroups() map[string]ruleset.NodeGroup {
	return map[string]ruleset.NodeGroup{
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
}

func TestXrayRoutingGeneration(t *testing.T) {
	rs := newChinaDirectRuleSet()
	groups := newTestGroups()

	routing, extraOutbounds, err := BuildXrayRouting(rs, groups)
	require.NoError(t, err)

	routingJSON, err := json.MarshalIndent(routing, "", "  ")
	require.NoError(t, err)
	t.Logf("Xray routing:\n%s", string(routingJSON))

	outboundsJSON, err := json.MarshalIndent(extraOutbounds, "", "  ")
	require.NoError(t, err)
	t.Logf("Xray extra outbounds:\n%s", string(outboundsJSON))

	assert.Contains(t, routing, "domainStrategy")
	assert.Equal(t, "AsIs", routing["domainStrategy"])

	rules, ok := routing["rules"].([]map[string]interface{})
	require.True(t, ok, "rules must be a slice")

	assert.GreaterOrEqual(t, len(rules), 9, "should have at least 9 explicit rules + 1 final")

	rule0 := rules[0]
	assert.Equal(t, "field", rule0["type"])
	assert.Equal(t, "block", rule0["outboundTag"])
	domain0, ok := rule0["domain"].([]string)
	require.True(t, ok)
	assert.Contains(t, domain0, "geosite:category-ads-all")

	rule1 := rules[1]
	assert.Equal(t, "direct", rule1["outboundTag"])
	domain1, ok := rule1["domain"].([]string)
	require.True(t, ok)
	assert.Contains(t, domain1, "geosite:cn")

	rule2 := rules[2]
	assert.Equal(t, "direct", rule2["outboundTag"])
	ip2, ok := rule2["ip"].([]string)
	require.True(t, ok)
	assert.Contains(t, ip2, "geoip:cn")

	rule7 := rules[7]
	assert.Equal(t, "streaming-unlock", rule7["outboundTag"])
	domain7, ok := rule7["domain"].([]string)
	require.True(t, ok)
	assert.Contains(t, domain7, "geosite:netflix")

	lastRule := rules[len(rules)-1]
	assert.Equal(t, "proxy", lastRule["outboundTag"])

	assert.Len(t, extraOutbounds, 1, "should generate 1 group outbound for streaming-unlock")
	streamingOut := extraOutbounds[0]
	assert.Equal(t, "streaming-unlock", streamingOut["tag"])
}

func TestSingboxRouteGeneration(t *testing.T) {
	rs := newChinaDirectRuleSet()
	groups := newTestGroups()

	route, extraOutbounds, err := BuildSingboxRoute(rs, groups)
	require.NoError(t, err)

	routeJSON, err := json.MarshalIndent(route, "", "  ")
	require.NoError(t, err)
	t.Logf("Sing-box route:\n%s", string(routeJSON))

	outboundsJSON, err := json.MarshalIndent(extraOutbounds, "", "  ")
	require.NoError(t, err)
	t.Logf("Sing-box extra outbounds:\n%s", string(outboundsJSON))

	assert.Contains(t, route, "rules")
	assert.Contains(t, route, "final")

	rules, ok := route["rules"].([]map[string]interface{})
	require.True(t, ok, "rules must be a slice")
	assert.Len(t, rules, 9, "should have exactly 9 explicit rules (final is separate)")

	rule0 := rules[0]
	assert.Equal(t, "block", rule0["action"])
	geosite0, ok := rule0["geosite"].([]string)
	require.True(t, ok)
	assert.Contains(t, geosite0, "category-ads-all")
	_, hasOutbound0 := rule0["outbound"]
	assert.False(t, hasOutbound0, "block action should not have outbound field (using new 1.11+ syntax)")

	rule1 := rules[1]
	assert.Equal(t, "route", rule1["action"])
	assert.Equal(t, "direct", rule1["outbound"])
	geosite1, ok := rule1["geosite"].([]string)
	require.True(t, ok)
	assert.Contains(t, geosite1, "cn")

	rule4 := rules[4]
	assert.Equal(t, "route", rule4["action"])
	assert.Equal(t, "direct", rule4["outbound"])
	domainSuffix4, ok := rule4["domain_suffix"].([]string)
	require.True(t, ok)
	assert.Contains(t, domainSuffix4, "baidu.com")

	rule6 := rules[6]
	assert.Equal(t, "route", rule6["action"])
	assert.Equal(t, "direct", rule6["outbound"])
	port6, ok := rule6["port"].([]string)
	require.True(t, ok)
	assert.Contains(t, port6, "22")

	rule7 := rules[7]
	assert.Equal(t, "route", rule7["action"])
	assert.Equal(t, "streaming-unlock", rule7["outbound"])
	geosite7, ok := rule7["geosite"].([]string)
	require.True(t, ok)
	assert.Contains(t, geosite7, "netflix")

	final, ok := route["final"].(map[string]interface{})
	require.True(t, ok, "final should be an action object")
	assert.Equal(t, "route", final["action"])
	assert.Equal(t, "proxy", final["outbound"])

	assert.Len(t, extraOutbounds, 1)
	streamingOut := extraOutbounds[0]
	assert.Equal(t, "streaming-unlock", streamingOut["tag"])
	assert.Equal(t, "urltest", streamingOut["type"])
}

func TestDualKernelRuleSetEquivalence(t *testing.T) {
	rs := newChinaDirectRuleSet()
	groups := newTestGroups()

	xrayRouting, xrayOutbounds, err := BuildXrayRouting(rs, groups)
	require.NoError(t, err)

	sbRoute, sbOutbounds, err := BuildSingboxRoute(rs, groups)
	require.NoError(t, err)

	xrayRules := xrayRouting["rules"].([]map[string]interface{})
	sbRules := sbRoute["rules"].([]map[string]interface{})

	t.Run("geosite cn both point to direct", func(t *testing.T) {
		var xrayDirectForGeositeCN, sbDirectForGeositeCN bool

		for _, r := range xrayRules {
			domains, ok := r["domain"].([]string)
			if !ok {
				continue
			}
			for _, d := range domains {
				if d == "geosite:cn" {
					if r["outboundTag"] == "direct" {
						xrayDirectForGeositeCN = true
					}
				}
			}
		}

		for _, r := range sbRules {
			geosites, ok := r["geosite"].([]string)
			if !ok {
				continue
			}
			for _, g := range geosites {
				if g == "cn" {
					if r["action"] == "route" && r["outbound"] == "direct" {
						sbDirectForGeositeCN = true
					}
				}
			}
		}

		assert.True(t, xrayDirectForGeositeCN, "Xray: geosite:cn should route to direct")
		assert.True(t, sbDirectForGeositeCN, "Sing-box: geosite:cn should route to direct")
	})

	t.Run("ads both point to block", func(t *testing.T) {
		var xrayBlockForAds, sbBlockForAds bool

		for _, r := range xrayRules {
			domains, ok := r["domain"].([]string)
			if !ok {
				continue
			}
			for _, d := range domains {
				if d == "geosite:category-ads-all" {
					if r["outboundTag"] == "block" {
						xrayBlockForAds = true
					}
				}
			}
		}

		for _, r := range sbRules {
			geosites, ok := r["geosite"].([]string)
			if !ok {
				continue
			}
			for _, g := range geosites {
				if g == "category-ads-all" {
					if r["action"] == "block" {
						sbBlockForAds = true
					}
				}
			}
		}

		assert.True(t, xrayBlockForAds, "Xray: ads should route to block")
		assert.True(t, sbBlockForAds, "Sing-box: ads should route to block")
	})

	t.Run("streaming group outbounds contain same node IDs", func(t *testing.T) {
		var xrayStreamingNodeIDs []string
		var sbStreamingNodeIDs []string

		for _, ob := range xrayOutbounds {
			if ob["tag"] == "streaming-unlock" {
				settings, ok := ob["settings"].(map[string]interface{})
				if ok {
					if outbounds, ok := settings["outbounds"].([]string); ok {
						xrayStreamingNodeIDs = outbounds
					} else if selector, ok := settings["servers"].([]string); ok {
						xrayStreamingNodeIDs = selector
					}
				}
			}
		}

		for _, ob := range sbOutbounds {
			if ob["tag"] == "streaming-unlock" {
				if outbounds, ok := ob["outbounds"].([]string); ok {
					sbStreamingNodeIDs = outbounds
				}
			}
		}

		expected := groups["streaming-unlock"].NodeIDs

		assert.ElementsMatch(t, expected, xrayStreamingNodeIDs, "Xray streaming group should contain correct node IDs")
		assert.ElementsMatch(t, expected, sbStreamingNodeIDs, "Sing-box streaming group should contain correct node IDs")
		assert.ElementsMatch(t, xrayStreamingNodeIDs, sbStreamingNodeIDs, "both kernels must have identical node ID lists for streaming-unlock group")
	})

	t.Run("final default is proxy", func(t *testing.T) {
		xrayLastRule := xrayRules[len(xrayRules)-1]
		assert.Equal(t, "proxy", xrayLastRule["outboundTag"], "Xray final rule should point to proxy")

		sbFinal, ok := sbRoute["final"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "route", sbFinal["action"])
		assert.Equal(t, "proxy", sbFinal["outbound"], "Sing-box final should route to proxy")
	})

	t.Run("netflix both point to streaming-unlock group", func(t *testing.T) {
		var xrayNetflixToStreaming, sbNetflixToStreaming bool

		for _, r := range xrayRules {
			domains, ok := r["domain"].([]string)
			if !ok {
				continue
			}
			for _, d := range domains {
				if d == "geosite:netflix" {
					if r["outboundTag"] == "streaming-unlock" {
						xrayNetflixToStreaming = true
					}
				}
			}
		}

		for _, r := range sbRules {
			geosites, ok := r["geosite"].([]string)
			if !ok {
				continue
			}
			for _, g := range geosites {
				if g == "netflix" {
					if r["action"] == "route" && r["outbound"] == "streaming-unlock" {
						sbNetflixToStreaming = true
					}
				}
			}
		}

		assert.True(t, xrayNetflixToStreaming, "Xray: netflix should point to streaming-unlock")
		assert.True(t, sbNetflixToStreaming, "Sing-box: netflix should point to streaming-unlock")
	})
}

func TestSingboxUsesNewActionSyntax(t *testing.T) {
	rs := &ruleset.RuleSet{
		ID:   "test-syntax",
		Name: "Test New Syntax",
		Rules: []ruleset.Rule{
			{Type: "geosite", Value: "cn", Action: ruleset.ActionDirect},
			{Type: "geosite", Value: "ads", Action: ruleset.ActionBlock},
		},
	}
	groups := map[string]ruleset.NodeGroup{}

	route, _, err := BuildSingboxRoute(rs, groups)
	require.NoError(t, err)

	rules := route["rules"].([]map[string]interface{})

	directRule := rules[0]
	assert.Equal(t, "route", directRule["action"], "direct must use action:route (new 1.11+ syntax)")
	assert.Equal(t, "direct", directRule["outbound"])
	actionVal, hasAction := directRule["action"]
	assert.True(t, hasAction, "all rules must have action field")
	assert.NotEmpty(t, actionVal)

	blockRule := rules[1]
	assert.Equal(t, "block", blockRule["action"], "block must use action:block")
	_, hasOutboundOnBlock := blockRule["outbound"]
	assert.False(t, hasOutboundOnBlock, "block rules should NOT have outbound field")
}
