package exposure

import (
	"fmt"

	"github.com/airport-panel/subscription/ruleset"
)

func BuildXrayRouting(rs *ruleset.RuleSet, groups map[string]ruleset.NodeGroup) (map[string]interface{}, []map[string]interface{}, error) {
	if err := rs.Validate(); err != nil {
		return nil, nil, err
	}

	rules := make([]map[string]interface{}, 0)
	extraOutbounds := make([]map[string]interface{}, 0)

	outboundSet := make(map[string]bool)
	outboundSet["direct"] = true
	outboundSet["block"] = true
	outboundSet["proxy"] = true

	for _, rule := range rs.Rules {
		xrayRule := map[string]interface{}{
			"type": "field",
		}

		switch rule.Type {
		case "geosite":
			xrayRule["domain"] = []string{fmt.Sprintf("geosite:%s", rule.Value)}
		case "geoip":
			xrayRule["ip"] = []string{fmt.Sprintf("geoip:%s", rule.Value)}
		case "domain_suffix":
			xrayRule["domain"] = []string{fmt.Sprintf("domain:%s", rule.Value)}
		case "domain_keyword":
			xrayRule["domain"] = []string{fmt.Sprintf("keyword:%s", rule.Value)}
		case "domain":
			xrayRule["domain"] = []string{fmt.Sprintf("full:%s", rule.Value)}
		case "ip_cidr":
			xrayRule["ip"] = []string{rule.Value}
		case "port":
			xrayRule["port"] = rule.Value
		case "process_name":
			xrayRule["processName"] = []string{rule.Value}
		case "network":
			xrayRule["network"] = rule.Value
		}

		if rule.Invert {
			xrayRule["invert"] = true
		}

		var outboundTag string
		switch rule.Action {
		case ruleset.ActionDirect:
			outboundTag = "direct"
		case ruleset.ActionBlock, ruleset.ActionReject:
			outboundTag = "block"
		case ruleset.ActionProxy:
			outboundTag = "proxy"
		case ruleset.ActionGroup:
			outboundTag = rule.GroupTag
			if !outboundSet[rule.GroupTag] {
				outboundSet[rule.GroupTag] = true
				g, ok := groups[rule.GroupTag]
				if ok {
					ob := buildXrayGroupOutbound(g)
					extraOutbounds = append(extraOutbounds, ob)
				}
			}
		}
		xrayRule["outboundTag"] = outboundTag

		rules = append(rules, xrayRule)
	}

	var finalOutbound string
	switch rs.Final {
	case ruleset.ActionDirect:
		finalOutbound = "direct"
	case ruleset.ActionBlock, ruleset.ActionReject:
		finalOutbound = "block"
	case ruleset.ActionGroup:
		finalOutbound = ""
	default:
		finalOutbound = "proxy"
	}

	if finalOutbound != "" {
		finalRule := map[string]interface{}{
			"type":        "field",
			"outboundTag": finalOutbound,
		}
		rules = append(rules, finalRule)
	}

	routing := map[string]interface{}{
		"domainStrategy": "AsIs",
		"rules":          rules,
	}

	return routing, extraOutbounds, nil
}

func buildXrayGroupOutbound(g ruleset.NodeGroup) map[string]interface{} {
	outbound := map[string]interface{}{
		"tag": g.Tag,
	}

	switch g.Strategy {
	case "least_ping", "urltest":
		outbound["protocol"] = "urltest"
		outbound["settings"] = map[string]interface{}{
			"outbounds": g.NodeIDs,
			"url":       "https://www.gstatic.com/generate_204",
			"interval":  "3m",
			"tolerance": 50,
		}
	default:
		outbound["protocol"] = "selector"
		outbound["settings"] = map[string]interface{}{
			"outbounds": g.NodeIDs,
		}
	}

	return outbound
}
