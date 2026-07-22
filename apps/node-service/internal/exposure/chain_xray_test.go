package exposure

import (
	"testing"

	"github.com/airport-panel/subscription/chain"
	"github.com/airport-panel/subscription/nodespec"
	"github.com/stretchr/testify/assert"
)

func TestXrayChainOutbounds(t *testing.T) {
	landing := &nodespec.NodeSpec{
		ID:          "landing-1",
		Code:        "landing",
		Name:        "VLESS Landing",
		Protocol:    nodespec.ProtocolVLESS,
		Address:     "10.0.0.1",
		Port:        443,
		Transport:   nodespec.TransportConfig{Type: nodespec.TransportTCP},
		Security:    nodespec.SecurityNone,
		Credentials: nodespec.VLESSCredentials{UUID: "a3482e88-686a-4a58-8126-99c9df64b7bf"},
		AllowUDP:    true,
		TrafficRate: 1.0,
	}

	c := &chain.ChainSpec{
		ID:          "chain-test",
		Name:        "Test Chain",
		LandingNode: landing,
		Relays: []chain.ChainHop{
			{
				NodeID:      "relay-1",
				Protocol:    nodespec.ProtocolTrojan,
				Address:     "relay1.example.com",
				Port:        443,
				Credentials: nodespec.TrojanCredentials{Password: "pass1"},
				Transport:   nodespec.TransportConfig{Type: nodespec.TransportTCP},
				Security:    nodespec.SecurityNone,
			},
			{
				NodeID:      "relay-2",
				Protocol:    nodespec.ProtocolVLESS,
				Address:     "relay2.example.com",
				Port:        443,
				Credentials: nodespec.VLESSCredentials{UUID: "b3482e88-686a-4a58-8126-99c9df64b7bg"},
				Transport:   nodespec.TransportConfig{Type: nodespec.TransportTCP},
				Security:    nodespec.SecurityNone,
			},
		},
	}

	outbounds, routing, err := BuildXrayChainOutbounds(c)
	assert.NoError(t, err)

	assert.Len(t, outbounds, 5)

	assert.Equal(t, "direct", outbounds[0]["tag"])
	assert.Equal(t, "block", outbounds[1]["tag"])

	relay0 := outbounds[2]
	assert.Equal(t, "relay-0", relay0["tag"])
	assert.Equal(t, "trojan", relay0["protocol"])
	_, hasProxy := relay0["proxySettings"]
	assert.False(t, hasProxy, "first relay should not have proxySettings")

	relay1 := outbounds[3]
	assert.Equal(t, "relay-1", relay1["tag"])
	assert.Equal(t, "vless", relay1["protocol"])
	proxySettings, ok := relay1["proxySettings"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "relay-0", proxySettings["tag"])

	landingOut := outbounds[4]
	assert.Equal(t, "landing", landingOut["tag"])
	assert.Equal(t, "vless", landingOut["protocol"])
	landingProxy, ok := landingOut["proxySettings"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "relay-1", landingProxy["tag"])

	assert.Equal(t, "landing", routing["final"])
}

func buildTestChain() *chain.ChainSpec {
	landing := &nodespec.NodeSpec{
		ID:          "landing-reality",
		Code:        "landing",
		Name:        "VLESS Reality",
		Protocol:    nodespec.ProtocolVLESS,
		Address:     "10.0.0.3",
		Port:        443,
		Transport:   nodespec.TransportConfig{Type: nodespec.TransportTCP},
		Security:    nodespec.SecurityReality,
		Reality: &nodespec.RealityConfig{
			SNI:       "www.microsoft.com",
			PublicKey: "jDxv8rK2D0XqFm5pJnR9sT3uY7wA8bC2dE4fG6hH0jK=",
			ShortID:   "12345678",
		},
		Credentials: nodespec.VLESSCredentials{UUID: "a3482e88-686a-4a58-8126-99c9df64b7bf"},
		AllowUDP:    true,
		TrafficRate: 1.0,
	}

	return &chain.ChainSpec{
		ID:          "chain-three",
		Name:        "Three Hop",
		LandingNode: landing,
		Relays: []chain.ChainHop{
			{
				NodeID:      "hop-0",
				Tag:         "entry",
				Protocol:    nodespec.ProtocolTrojan,
				Address:     "entry.example.com",
				Port:        443,
				Credentials: nodespec.TrojanCredentials{Password: "entry-pass"},
				Transport:   nodespec.TransportConfig{Type: nodespec.TransportWS, WS: &nodespec.WSConfig{Path: "/trojan"}},
				Security:    nodespec.SecurityTLS,
				TLS:         &nodespec.TLSConfig{SNI: "entry.example.com"},
			},
			{
				NodeID:      "hop-1",
				Tag:         "middle",
				Protocol:    nodespec.ProtocolVLESS,
				Address:     "middle.example.com",
				Port:        8443,
				Credentials: nodespec.VLESSCredentials{UUID: "b3482e88-686a-4a58-8126-99c9df64b7bg"},
				Transport:   nodespec.TransportConfig{Type: nodespec.TransportGRPC, GRPC: &nodespec.GRPCConfig{ServiceName: "vless"}},
				Security:    nodespec.SecurityTLS,
				TLS:         &nodespec.TLSConfig{SNI: "middle.example.com"},
			},
			{
				NodeID:      "hop-2",
				Tag:         "exit",
				Protocol:    nodespec.ProtocolVMess,
				Address:     "exit.example.com",
				Port:        443,
				Credentials: nodespec.VMessCredentials{UUID: "c3482e88-686a-4a58-8126-99c9df64b7ch"},
				Transport:   nodespec.TransportConfig{Type: nodespec.TransportTCP},
				Security:    nodespec.SecurityNone,
			},
		},
	}
}
