package chain

import (
	"testing"

	"github.com/airport-panel/subscription/nodespec"
	"github.com/stretchr/testify/assert"
)

func TestChainValidation(t *testing.T) {
	validLanding := &nodespec.NodeSpec{
		ID:          "landing-1",
		Code:        "landing",
		Name:        "Landing Node",
		Protocol:    nodespec.ProtocolVLESS,
		Address:     "1.2.3.4",
		Port:        443,
		Transport:   nodespec.TransportConfig{Type: nodespec.TransportTCP},
		Security:    nodespec.SecurityNone,
		Credentials: nodespec.VLESSCredentials{UUID: "a3482e88-686a-4a58-8126-99c9df64b7bf"},
		AllowUDP:    true,
		TrafficRate: 1.0,
	}

	t.Run("empty landing should error", func(t *testing.T) {
		c := &ChainSpec{
			ID:   "chain-1",
			Name: "Test Chain",
		}
		err := c.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "landing node is required")
	})

	t.Run("invalid port should error", func(t *testing.T) {
		c := &ChainSpec{
			ID:          "chain-1",
			Name:        "Test Chain",
			LandingNode: validLanding,
			Relays: []ChainHop{
				{
					NodeID:    "relay-1",
					Protocol:  nodespec.ProtocolTrojan,
					Address:   "relay.example.com",
					Port:      99999,
					Transport: nodespec.TransportConfig{Type: nodespec.TransportTCP},
					Security:  nodespec.SecurityNone,
				},
			},
		}
		err := c.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid port")
	})

	t.Run("valid chain should pass", func(t *testing.T) {
		c := &ChainSpec{
			ID:          "chain-1",
			Name:        "Test Chain",
			LandingNode: validLanding,
		}
		err := c.Validate()
		assert.NoError(t, err)
		assert.Equal(t, TrafficPolicyBillAtLanding, c.TrafficPolicy)
	})
}

func TestChainSingleRelay(t *testing.T) {
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

	c := &ChainSpec{
		ID:          "chain-single",
		Name:        "Single Relay Chain",
		LandingNode: landing,
		Relays: []ChainHop{
			{
				NodeID:      "relay-1",
				Protocol:    nodespec.ProtocolTrojan,
				Address:     "relay.example.com",
				Port:        443,
				Credentials: nodespec.TrojanCredentials{Password: "secret123"},
				Transport:   nodespec.TransportConfig{Type: nodespec.TransportWS, WS: &nodespec.WSConfig{Path: "/ws"}},
				Security:    nodespec.SecurityTLS,
				TLS:         &nodespec.TLSConfig{SNI: "relay.example.com"},
			},
		},
	}

	err := c.Validate()
	assert.NoError(t, err)

	nodes := c.ToNodeSpecs()
	assert.Len(t, nodes, 2)

	relayNode := nodes[0]
	assert.Equal(t, "relay-0", relayNode.Code)
	assert.Equal(t, nodespec.ProtocolTrojan, relayNode.Protocol)
	assert.Equal(t, "relay.example.com", relayNode.Address)
	assert.Equal(t, 443, relayNode.Port)
	assert.Equal(t, "", relayNode.ParentNodeID)

	landingNode := nodes[1]
	assert.Equal(t, "landing", landingNode.Code)
	assert.Equal(t, nodespec.ProtocolVLESS, landingNode.Protocol)
	assert.Equal(t, "relay-0", landingNode.ParentNodeID)
}

func TestChainThreeHops(t *testing.T) {
	landing := &nodespec.NodeSpec{
		ID:          "landing-reality",
		Code:        "landing",
		Name:        "VLESS Reality Landing",
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

	c := &ChainSpec{
		ID:          "chain-three",
		Name:        "Three Hop Mixed Chain",
		LandingNode: landing,
		Relays: []ChainHop{
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
		TrafficPolicy: TrafficPolicyBillAtEntry,
	}

	err := c.Validate()
	assert.NoError(t, err)
	assert.Equal(t, TrafficPolicyBillAtEntry, c.TrafficPolicy)

	nodes := c.ToNodeSpecs()
	assert.Len(t, nodes, 4)

	assert.Equal(t, "entry", nodes[0].Code)
	assert.Equal(t, "", nodes[0].ParentNodeID)

	assert.Equal(t, "middle", nodes[1].Code)
	assert.Equal(t, "entry", nodes[1].ParentNodeID)

	assert.Equal(t, "exit", nodes[2].Code)
	assert.Equal(t, "middle", nodes[2].ParentNodeID)

	assert.Equal(t, "landing", nodes[3].Code)
	assert.Equal(t, "exit", nodes[3].ParentNodeID)
	assert.Equal(t, nodespec.SecurityReality, nodes[3].Security)
}
