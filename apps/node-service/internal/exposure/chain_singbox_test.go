package exposure

import (
	"testing"

	"github.com/airport-panel/subscription/chain"
	"github.com/airport-panel/subscription/nodespec"
	"github.com/stretchr/testify/assert"
)

func TestSingboxChainOutbounds(t *testing.T) {
	c := buildTestChain()

	outbounds, err := BuildSingboxChainOutbounds(c)
	assert.NoError(t, err)

	assert.Len(t, outbounds, 7)

	assert.Equal(t, "direct", outbounds[0]["tag"])
	assert.Equal(t, "block", outbounds[1]["tag"])
	assert.Equal(t, "dns-out", outbounds[2]["tag"])

	entry := outbounds[3]
	assert.Equal(t, "entry", entry["tag"])
	assert.Equal(t, "trojan", entry["type"])
	_, hasDetour := entry["detour"]
	assert.False(t, hasDetour, "first relay should not have detour")

	middle := outbounds[4]
	assert.Equal(t, "middle", middle["tag"])
	assert.Equal(t, "vless", middle["type"])
	detour, ok := middle["detour"].(string)
	assert.True(t, ok)
	assert.Equal(t, "entry", detour)

	exit := outbounds[5]
	assert.Equal(t, "exit", exit["tag"])
	assert.Equal(t, "vmess", exit["type"])
	detour, ok = exit["detour"].(string)
	assert.True(t, ok)
	assert.Equal(t, "middle", detour)

	landing := outbounds[6]
	assert.Equal(t, "landing", landing["tag"])
	assert.Equal(t, "vless", landing["type"])
	detour, ok = landing["detour"].(string)
	assert.True(t, ok)
	assert.Equal(t, "exit", detour)

	tlsConfig, ok := landing["tls"].(map[string]interface{})
	assert.True(t, ok)
	assert.True(t, tlsConfig["enabled"].(bool))
	realityConfig, ok := tlsConfig["reality"].(map[string]interface{})
	assert.True(t, ok)
	assert.True(t, realityConfig["enabled"].(bool))
	assert.Equal(t, "jDxv8rK2D0XqFm5pJnR9sT3uY7wA8bC2dE4fG6hH0jK=", realityConfig["public_key"])
	assert.Equal(t, "12345678", realityConfig["short_id"])
}

func TestDualKernelChainEquivalence(t *testing.T) {
	landing := &nodespec.NodeSpec{
		ID:          "landing-simple",
		Code:        "landing",
		Name:        "Simple Landing",
		Protocol:    nodespec.ProtocolVLESS,
		Address:     "10.0.0.1",
		Port:        443,
		Transport:   nodespec.TransportConfig{Type: nodespec.TransportTCP},
		Security:    nodespec.SecurityTLS,
		TLS:         &nodespec.TLSConfig{SNI: "landing.example.com"},
		Credentials: nodespec.VLESSCredentials{UUID: "a3482e88-686a-4a58-8126-99c9df64b7bf"},
		AllowUDP:    true,
		TrafficRate: 1.0,
	}

	c := &chain.ChainSpec{
		ID:          "chain-equiv",
		Name:        "Equivalence Test Chain",
		LandingNode: landing,
		Relays: []chain.ChainHop{
			{
				NodeID:      "r1",
				Tag:         "relay-a",
				Protocol:    nodespec.ProtocolTrojan,
				Address:     "a.example.com",
				Port:        443,
				Credentials: nodespec.TrojanCredentials{Password: "pw-a"},
				Transport:   nodespec.TransportConfig{Type: nodespec.TransportWS, WS: &nodespec.WSConfig{Path: "/ws"}},
				Security:    nodespec.SecurityTLS,
				TLS:         &nodespec.TLSConfig{SNI: "a.example.com"},
			},
			{
				NodeID:      "r2",
				Tag:         "relay-b",
				Protocol:    nodespec.ProtocolVMess,
				Address:     "b.example.com",
				Port:        8443,
				Credentials: nodespec.VMessCredentials{UUID: "b3482e88-686a-4a58-8126-99c9df64b7bg", AlterID: 0},
				Transport:   nodespec.TransportConfig{Type: nodespec.TransportTCP},
				Security:    nodespec.SecurityNone,
			},
		},
	}

	xrayOutbounds, xrayRouting, err := BuildXrayChainOutbounds(c)
	assert.NoError(t, err)

	sbOutbounds, err := BuildSingboxChainOutbounds(c)
	assert.NoError(t, err)

	assert.Len(t, xrayOutbounds, 5, "xray: direct+block+2relays+landing")
	assert.Len(t, sbOutbounds, 6, "singbox: direct+block+dns+2relays+landing")

	xrayChainTags := getXrayProxyChainTags(xrayOutbounds)
	sbChainTags := getSingboxDetourChainTags(sbOutbounds)

	assert.Equal(t, []string{"relay-a", "relay-b", "landing"}, xrayChainTags)
	assert.Equal(t, []string{"relay-a", "relay-b", "landing"}, sbChainTags)

	assert.Equal(t, "landing", xrayRouting["final"])

	sbRoute := BuildSingboxChainRoute(c)
	assert.Equal(t, "landing", sbRoute["final"])
}

func getXrayProxyChainTags(outbounds []map[string]interface{}) []string {
	tagOrder := make([]string, 0)
	for _, ob := range outbounds {
		tag, _ := ob["tag"].(string)
		if tag == "direct" || tag == "block" {
			continue
		}
		tagOrder = append(tagOrder, tag)
	}
	return tagOrder
}

func getSingboxDetourChainTags(outbounds []map[string]interface{}) []string {
	tagOrder := make([]string, 0)
	for _, ob := range outbounds {
		tag, _ := ob["tag"].(string)
		if tag == "direct" || tag == "block" || tag == "dns-out" {
			continue
		}
		tagOrder = append(tagOrder, tag)
	}
	return tagOrder
}
