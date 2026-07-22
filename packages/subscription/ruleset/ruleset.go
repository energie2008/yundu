package ruleset

import (
	"errors"
	"strconv"
)

type RuleAction string

const (
	ActionDirect RuleAction = "direct"
	ActionProxy  RuleAction = "proxy"
	ActionBlock  RuleAction = "block"
	ActionGroup  RuleAction = "group"
	ActionReject RuleAction = "reject"
)

type Rule struct {
	Type     string     `json:"type"`
	Value    string     `json:"value"`
	Action   RuleAction `json:"action"`
	GroupTag string     `json:"group_tag,omitempty"`
	Invert   bool       `json:"invert,omitempty"`
}

type DNSRule struct {
	Type   string `json:"type"`
	Domain string `json:"domain"`
	Server string `json:"server"`
}

type RuleSet struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Version  int        `json:"version"`
	Rules    []Rule     `json:"rules"`
	Final    RuleAction `json:"final"`
	DNSRules []DNSRule  `json:"dns_rules,omitempty"`
}

type NodeGroup struct {
	Tag      string   `json:"tag"`
	Name     string   `json:"name"`
	NodeIDs  []string `json:"node_ids"`
	Kernel   string   `json:"kernel,omitempty"`
	Strategy string   `json:"strategy"`
}

var validRuleTypes = map[string]bool{
	"geosite":        true,
	"geoip":          true,
	"domain_suffix":  true,
	"domain_keyword": true,
	"domain":         true,
	"ip_cidr":        true,
	"port":           true,
	"process_name":   true,
	"network":        true,
}

var validStrategies = map[string]bool{
	"round_robin": true,
	"least_ping":  true,
	"failover":    true,
}

func (rs *RuleSet) Validate() error {
	if rs.Final == "" {
		rs.Final = ActionProxy
	}

	for i, rule := range rs.Rules {
		if !validRuleTypes[rule.Type] {
			return errors.New("invalid rule type: " + rule.Type + " at index " + strconv.Itoa(i))
		}
		if rule.Action == ActionGroup && rule.GroupTag == "" {
			return errors.New("group action requires GroupTag at index " + strconv.Itoa(i))
		}
	}

	return nil
}

func (ng *NodeGroup) Validate() error {
	if ng.Tag == "" {
		return errors.New("node group tag cannot be empty")
	}
	if len(ng.NodeIDs) == 0 {
		return errors.New("node group NodeIDs cannot be empty")
	}
	if ng.Strategy != "" && !validStrategies[ng.Strategy] {
		return errors.New("invalid strategy: " + ng.Strategy)
	}
	return nil
}
