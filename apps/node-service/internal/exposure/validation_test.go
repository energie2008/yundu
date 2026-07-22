package exposure

import (
	"testing"

	"github.com/airport-panel/subscription/nodespec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestInvalidConfigReturnsError(t *testing.T) {
	invalidSpec := &nodespec.NodeSpec{
		ID:       "test-invalid-001",
		Code:     "INVALID",
		Name:     "Invalid Node",
		Protocol: nodespec.ProtocolVLESS,
		Address:  "",
		Port:     443,
		Transport: nodespec.TransportConfig{
			Type: nodespec.TransportWS,
		},
		Security: nodespec.SecurityTLS,
		Credentials: nodespec.VLESSCredentials{
			UUID: "not-a-valid-uuid",
		},
		TrafficRate: 1.0,
	}

	err := invalidSpec.Validate()
	assert.Error(t, err, "Invalid spec should return validation error")
	assert.Contains(t, err.Error(), "address", "Should error about missing address")
}

func TestYAMLParseValidation(t *testing.T) {
	yamlInput := `
id: yaml-test-001
code: YAMLNODE
name: YAML Test Node
protocol: vless
address: yaml.example.com
port: 443
transport:
  type: ws
  ws:
    path: /ws
    host: yaml.example.com
security: tls
tls:
  sni: yaml.example.com
credentials:
  uuid: 12345678-1234-1234-1234-1234567890ab
traffic_rate: 1
`
	spec := &nodespec.NodeSpec{}
	err := yaml.Unmarshal([]byte(yamlInput), spec)
	require.NoError(t, err, "YAML should unmarshal successfully")

	err = spec.Validate()
	require.NoError(t, err, "YAML-parsed spec should be valid")
	assert.Equal(t, "yaml.example.com", spec.Address)
	assert.Equal(t, nodespec.TransportWS, spec.Transport.Type)
	assert.Equal(t, "/ws", spec.Transport.WS.Path)
}
