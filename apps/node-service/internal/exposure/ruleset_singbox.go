package exposure

import (
	"github.com/airport-panel/subscription/ruleset"
)

func BuildSingboxRoute(rs *ruleset.RuleSet, groups map[string]ruleset.NodeGroup) (map[string]interface{}, []map[string]interface{}, error) {
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
		sbRule := map[string]interface{}{}

		switch rule.Type {
		case "geosite":
			sbRule["geosite"] = []string{rule.Value}
		case "geoip":
			sbRule["geoip"] = []string{rule.Value}
		case "domain_suffix":
			sbRule["domain_suffix"] = []string{rule.Value}
		case "domain_keyword":
			sbRule["domain_keyword"] = []string{rule.Value}
		case "domain":
			sbRule["domain"] = []string{rule.Value}
		case "ip_cidr":
			sbRule["ip_cidr"] = []string{rule.Value}
		case "port":
			sbRule["port"] = []string{rule.Value}
		case "process_name":
			sbRule["process_name"] = []string{rule.Value}
		case "network":
			sbRule["network"] = []string{rule.Value}
		}

		if rule.Invert {
			sbRule["invert"] = true
		}

		switch rule.Action {
		case ruleset.ActionDirect:
			sbRule["action"] = "route"
			sbRule["outbound"] = "direct"
		case ruleset.ActionBlock, ruleset.ActionReject:
			sbRule["action"] = "block"
		case ruleset.ActionProxy:
			sbRule["action"] = "route"
			sbRule["outbound"] = "proxy"
		case ruleset.ActionGroup:
			sbRule["action"] = "route"
			sbRule["outbound"] = rule.GroupTag
			if !outboundSet[rule.GroupTag] {
				outboundSet[rule.GroupTag] = true
				g, ok := groups[rule.GroupTag]
				if ok {
					ob := buildSingboxGroupOutbound(g)
					extraOutbounds = append(extraOutbounds, ob)
				}
			}
		}

		rules = append(rules, sbRule)
	}

	var finalAction interface{}
	switch rs.Final {
	case ruleset.ActionDirect:
		finalAction = "direct"
	case ruleset.ActionBlock, ruleset.ActionReject:
		finalAction = "block"
	case ruleset.ActionGroup:
		finalAction = map[string]interface{}{
			"action":   "route",
			"outbound": "",
		}
	default:
		finalAction = map[string]interface{}{
			"action":   "route",
			"outbound": "proxy",
		}
	}

	route := map[string]interface{}{
		"rules": rules,
		"final": finalAction,
	}

	return route, extraOutbounds, nil
}

func buildSingboxGroupOutbound(g ruleset.NodeGroup) map[string]interface{} {
	outbound := map[string]interface{}{
		"tag": g.Tag,
	}

	switch g.Strategy {
	case "least_ping":
		outbound["type"] = "urltest"
		outbound["outbounds"] = g.NodeIDs
		outbound["url"] = "https://www.gstatic.com/generate_204"
		outbound["interval"] = "3m"
		outbound["tolerance"] = 50
	case "failover":
		outbound["type"] = "urltest"
		outbound["outbounds"] = g.NodeIDs
		outbound["url"] = "https://www.gstatic.com/generate_204"
		outbound["interval"] = "3m"
		outbound["failover"] = true
	default:
		outbound["type"] = "selector"
		outbound["outbounds"] = g.NodeIDs
	}

	return outbound
}
