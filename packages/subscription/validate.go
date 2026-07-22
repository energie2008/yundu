package subscription

import (
	"fmt"

	"github.com/airport-panel/subscription/nodespec"
)

func ValidateNodeSpec(spec *nodespec.NodeSpec) error {
	if spec == nil {
		return fmt.Errorf("node spec is nil")
	}
	return spec.Validate()
}

func CanRenderToXray(spec *nodespec.NodeSpec) error {
	if err := ValidateNodeSpec(spec); err != nil {
		return err
	}

	switch spec.Protocol {
	case nodespec.ProtocolVLESS, nodespec.ProtocolVMess, nodespec.ProtocolTrojan, nodespec.ProtocolShadowsocks:
	default:
		return fmt.Errorf("protocol %s may not be fully supported by Xray", spec.Protocol)
	}

	switch spec.Transport.Type {
	case nodespec.TransportTCP, nodespec.TransportWS, nodespec.TransportGRPC,
		nodespec.TransportHTTP2, nodespec.TransportHTTPUpgrade, nodespec.TransportKCP, nodespec.TransportQUIC:
	default:
		return fmt.Errorf("transport %s may not be supported by Xray", spec.Transport.Type)
	}

	if spec.Security == nodespec.SecurityTLS && spec.TLS == nil {
		return fmt.Errorf("tls config recommended when security is tls")
	}

	return nil
}
