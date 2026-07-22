package audit

import (
	"encoding/json"
	"fmt"
	"log/slog"
)

const (
	RuleTypeBTBlock   = "bt_block"
	RuleTypeSSRFBlock = "ssrf_block"
	RuleTypeLANBlock  = "lan_block"
	RuleTypeCustom    = "custom"
)

type AuditRule struct {
	Type    string          `json:"type"`
	Enabled bool            `json:"enabled"`
	Config  json.RawMessage `json:"config,omitempty"`
}

type AuditConfig struct {
	Rules []AuditRule `json:"rules"`
}

var defaultAuditRules = []AuditRule{
	{Type: RuleTypeBTBlock, Enabled: true},
	{Type: RuleTypeSSRFBlock, Enabled: true},
}

func ParseAuditRules(raw interface{}, logger *slog.Logger) []AuditRule {
	if raw == nil {
		return defaultAuditRules
	}
	data, err := json.Marshal(raw)
	if err != nil {
		logger.Warn("failed to marshal _audit_rules, using defaults", "error", err)
		return defaultAuditRules
	}
	var cfg AuditConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		logger.Warn("failed to unmarshal _audit_rules, using defaults", "error", err)
		return defaultAuditRules
	}
	if len(cfg.Rules) == 0 {
		return defaultAuditRules
	}
	return cfg.Rules
}

func InjectIntoXray(config map[string]interface{}, rules []AuditRule, logger *slog.Logger) {
	routing, _ := config["routing"].(map[string]interface{})
	if routing == nil {
		routing = map[string]interface{}{}
		config["routing"] = routing
	}
	rulesList, _ := routing["rules"].([]interface{})
	ssrfIPs := []interface{}{
		"127.0.0.1/32", "0.0.0.0/32", "::1/128",
		"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "fc00::/7",
	}
	ssrfDomains := []interface{}{
		"localhost", "metadata.google.internal", "metadata.tencentyun.com",
	}
	btDomains := []interface{}{
		"tracker", "open.tracker", "opentracker",
		"announce", "peer", "dht",
	}
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		switch rule.Type {
		case RuleTypeSSRFBlock:
			rulesList = append(rulesList, map[string]interface{}{
				"type":        "field",
				"ip":          ssrfIPs,
				"outboundTag": "block",
			})
			rulesList = append(rulesList, map[string]interface{}{
				"type":        "field",
				"domain":      ssrfDomains,
				"outboundTag": "block",
			})
			logger.Info("audit: injected SSRF block rule")
		case RuleTypeBTBlock:
			rulesList = append(rulesList, map[string]interface{}{
				"type":        "field",
				"domain":      btDomains,
				"outboundTag": "block",
			})
			rulesList = append(rulesList, map[string]interface{}{
				"type":        "field",
				"protocol":    []interface{}{"bittorrent"},
				"outboundTag": "block",
			})
			logger.Info("audit: injected BT block rule")
		case RuleTypeLANBlock:
			rulesList = append(rulesList, map[string]interface{}{
				"type":        "field",
				"ip":          []interface{}{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "fc00::/7"},
				"outboundTag": "block",
			})
			logger.Info("audit: injected LAN block rule")
		case RuleTypeCustom:
			var custom map[string]interface{}
			if err := json.Unmarshal(rule.Config, &custom); err == nil {
				rulesList = append(rulesList, custom)
				logger.Info("audit: injected custom rule")
			}
		default:
			logger.Warn("audit: unknown rule type", "type", rule.Type)
		}
	}
	routing["rules"] = rulesList

	outbounds, _ := config["outbounds"].([]interface{})
	hasBlock := false
	for _, ob := range outbounds {
		if m, ok := ob.(map[string]interface{}); ok {
			if tag, _ := m["tag"].(string); tag == "block" {
				hasBlock = true
				break
			}
		}
	}
	if !hasBlock {
		outbounds = append(outbounds, map[string]interface{}{
			"tag":  "block",
			"protocol": "blackhole",
			"settings": map[string]interface{}{
				"response": map[string]interface{}{"type": "http"},
			},
		})
		config["outbounds"] = outbounds
	}
}

func InjectIntoSingbox(config map[string]interface{}, rules []AuditRule, logger *slog.Logger) {
	outbounds, _ := config["outbounds"].([]interface{})
	route, _ := config["route"].(map[string]interface{})
	if route == nil {
		route = map[string]interface{}{}
		config["route"] = route
	}
	routeRules, _ := route["rules"].([]interface{})
	hasBlock := false
	for _, ob := range outbounds {
		if m, ok := ob.(map[string]interface{}); ok {
			if tag, _ := m["tag"].(string); tag == "block" {
				hasBlock = true
				break
			}
		}
	}
	if !hasBlock {
		outbounds = append(outbounds, map[string]interface{}{
			"type": "block",
			"tag":  "block",
		})
		config["outbounds"] = outbounds
	}
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		switch rule.Type {
		case RuleTypeSSRFBlock:
			routeRules = append(routeRules, map[string]interface{}{
				"ip_cidr":      []interface{}{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "fc00::/7"},
				"outbound":     "block",
			})
			logger.Info("audit: injected sing-box SSRF block rule")
		case RuleTypeBTBlock:
			routeRules = append(routeRules, map[string]interface{}{
				"protocol": []interface{}{"bittorrent"},
				"outbound": "block",
			})
			logger.Info("audit: injected sing-box BT block rule")
		default:
			logger.Warn("audit: sing-box unsupported rule type", "type", rule.Type)
		}
	}
	route["rules"] = routeRules
}

func ExtractRules(config map[string]interface{}, logger *slog.Logger) []AuditRule {
	rawRules, hasRules := config["_audit_rules"]
	delete(config, "_audit_rules")
	rules := ParseAuditRules(rawRules, logger)
	if hasRules {
		logger.Info("audit: loaded dynamic rules", "count", len(rules))
	}
	return rules
}

func ApplyToConfig(config map[string]interface{}, runtimeType string, rules []AuditRule, logger *slog.Logger) error {
	switch runtimeType {
	case "xray":
		InjectIntoXray(config, rules, logger)
	case "sing-box":
		InjectIntoSingbox(config, rules, logger)
	default:
		return fmt.Errorf("unsupported runtime type for audit injection: %s", runtimeType)
	}
	return nil
}

func Apply(config map[string]interface{}, runtimeType string, logger *slog.Logger) error {
	rules := ExtractRules(config, logger)
	return ApplyToConfig(config, runtimeType, rules, logger)
}
