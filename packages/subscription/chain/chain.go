package chain

import (
	"errors"
	"fmt"

	"github.com/airport-panel/subscription/nodespec"
)

const (
	TrafficPolicyBillAtLanding = "bill_at_landing"
	TrafficPolicyBillAtEntry   = "bill_at_entry"
)

type ChainHop struct {
	NodeID      string                 `json:"node_id"`
	Protocol    nodespec.Protocol      `json:"protocol"`
	Address     string                 `json:"address"`
	Port        int                    `json:"port"`
	Credentials interface{}            `json:"credentials"`
	Transport   nodespec.TransportConfig `json:"transport"`
	Security    nodespec.Security      `json:"security"`
	TLS         *nodespec.TLSConfig    `json:"tls,omitempty"`
	Reality     *nodespec.RealityConfig `json:"reality,omitempty"`
	Tag         string                 `json:"tag,omitempty"`
}

type ChainSpec struct {
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	LandingNode   *nodespec.NodeSpec `json:"landing_node"`
	Relays        []ChainHop   `json:"relays"`
	TrafficPolicy string       `json:"traffic_policy,omitempty"`
}

func (c *ChainSpec) Validate() error {
	if c.ID == "" {
		return errors.New("chain id is required")
	}
	if c.Name == "" {
		return errors.New("chain name is required")
	}
	if c.LandingNode == nil {
		return errors.New("landing node is required")
	}
	if err := c.LandingNode.Validate(); err != nil {
		return fmt.Errorf("landing node validation failed: %w", err)
	}

	for i, hop := range c.Relays {
		if err := validateChainHop(i, hop); err != nil {
			return err
		}
	}

	if c.TrafficPolicy == "" {
		c.TrafficPolicy = TrafficPolicyBillAtLanding
	}
	if c.TrafficPolicy != TrafficPolicyBillAtLanding && c.TrafficPolicy != TrafficPolicyBillAtEntry {
		return fmt.Errorf("invalid traffic_policy: %s", c.TrafficPolicy)
	}

	return nil
}

func validateChainHop(index int, hop ChainHop) error {
	if hop.Address == "" {
		return fmt.Errorf("relay[%d] address is required", index)
	}
	if hop.Port < 1 || hop.Port > 65535 {
		return fmt.Errorf("relay[%d] invalid port: %d (must be 1-65535)", index, hop.Port)
	}
	if !nodespec.ValidProtocols[hop.Protocol] {
		return fmt.Errorf("relay[%d] invalid protocol: %s", index, hop.Protocol)
	}
	if !nodespec.ValidTransports[hop.Transport.Type] {
		return fmt.Errorf("relay[%d] invalid transport: %s", index, hop.Transport.Type)
	}
	if !nodespec.ValidSecurity[hop.Security] {
		return fmt.Errorf("relay[%d] invalid security: %s", index, hop.Security)
	}
	return nil
}

func (c *ChainSpec) ToNodeSpecs() []*nodespec.NodeSpec {
	nodes := make([]*nodespec.NodeSpec, 0, len(c.Relays)+1)

	var prevTag string
	for i, hop := range c.Relays {
		tag := hop.Tag
		if tag == "" {
			tag = fmt.Sprintf("relay-%d", i)
		}

		node := &nodespec.NodeSpec{
			ID:          hop.NodeID,
			Code:        tag,
			Name:        fmt.Sprintf("relay-%d", i),
			Protocol:    hop.Protocol,
			Address:     hop.Address,
			Port:        hop.Port,
			Credentials: hop.Credentials,
			Transport:   hop.Transport,
			Security:    hop.Security,
			TLS:         hop.TLS,
			Reality:     hop.Reality,
			AllowUDP:    true,
			TrafficRate: 1.0,
		}

		if prevTag != "" {
			node.ParentNodeID = prevTag
			node.Tags = []string{prevTag}
		}

		nodes = append(nodes, node)
		prevTag = tag
	}

	landing := *c.LandingNode
	landingTag := "landing"
	landing.Code = landingTag
	landing.Tags = []string{landingTag}
	if prevTag != "" {
		landing.ParentNodeID = prevTag
		landing.Tags = append(landing.Tags, prevTag)
	}
	nodes = append(nodes, &landing)

	return nodes
}
