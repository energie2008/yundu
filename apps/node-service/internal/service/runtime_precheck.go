package service

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/airport-panel/node-service/internal/model"
)

var ErrPrecheckRuntime = fmt.Errorf("runtime precheck failed")

func (s *DeploymentService) precheckRuntimePerServer(ctx context.Context, nodes []*model.Node) error {
	if len(nodes) == 0 {
		return nil
	}

	usedPorts := make(map[int]string) // server_port -> node code
	usedStreamSNI := make(map[string]string) // sni(host) -> node code
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 3 * time.Second}
			return d.DialContext(ctx, "udp", "223.5.5.5:53")
		},
	}

	for _, n := range nodes {
		if n == nil || n.DeletedAt != nil || !n.IsEnabled {
			continue
		}
		code := n.Code

		// --- Extract fields ---
		security := strings.ToLower(toStringOrEmpty(n.ConfigJSON, "security"))
		if n.SecurityType != nil && *n.SecurityType != "" {
			security = strings.ToLower(*n.SecurityType)
		}
		protocol := strings.ToLower(n.ProtocolType)

		serverPort := 0
		if n.ServerPort != nil && *n.ServerPort > 0 {
			serverPort = *n.ServerPort
		} else if sp, ok := toFloat64(n.ConfigJSON, "server_port"); ok && sp > 0 && sp <= 65535 {
			serverPort = int(sp)
		}
		externalPort := n.Port
		if externalPort == 0 {
			if p, ok := toFloat64(n.ConfigJSON, "port"); ok && p > 0 && p <= 65535 {
				externalPort = int(p)
			}
		}

		cdnAddr := strings.TrimSpace(toStringOrEmpty(n.ConfigJSON, "cdn_address"))
		realitySNI := strings.TrimSpace(toStringOrEmpty(n.ConfigJSON, "reality_server_name"))
		if realitySNI == "" {
			if rs, ok := n.ConfigJSON["reality_settings"].(map[string]interface{}); ok {
				realitySNI = strings.TrimSpace(toStringOrEmpty(rs, "server_name"))
			}
		}
		if realitySNI == "" && n.SNI != nil {
			realitySNI = strings.TrimSpace(*n.SNI)
		}

		nodeType := strings.ToLower(string(n.NodeType))

		// --- 1. REALITY hard checks (cannot be bypassed by manual ConfigJSON edits) ---
		if security == "reality" {
			if serverPort <= 0 {
				return fmt.Errorf("%w: node %s: REALITY requires server_port > 0 (auto-allocate failed or missing)", ErrPrecheckRuntime, code)
			}
			if serverPort == 443 {
				return fmt.Errorf("%w: node %s: REALITY server_port must NOT be 443 (443 is for external nginx stream SNI, REALITY listens on a high port)", ErrPrecheckRuntime, code)
			}
			if realitySNI == "" {
				return fmt.Errorf("%w: node %s: REALITY requires reality_server_name (SNI)", ErrPrecheckRuntime, code)
			}
		}

		// --- 2. TCP nodes external port must be 443 (zero-SSH architecture) ---
		if nodeType == "tcp" || (protocol != "hysteria2" && protocol != "tuic" && protocol != "udp") {
			if externalPort != 443 && externalPort != 0 {
				// Not a hard fail — warn but allow legacy nodes (new/updated nodes via standardizeNodeFields are forced to 443)
				s.logger.Warn("precheck: TCP node external port is not 443 (may bypass nginx stream SNI)",
					"code", code, "external_port", externalPort)
			}
		}

		// --- 3. Port collision check (server_port must be unique within the server) ---
		if serverPort > 0 {
			if owner, exists := usedPorts[serverPort]; exists {
				return fmt.Errorf("%w: server port collision on %d: node %s conflicts with %s", ErrPrecheckRuntime, serverPort, code, owner)
			}
			usedPorts[serverPort] = code
		}

		// --- 4. Stream SNI collision check ---
		// CDN SNI
		if cdnAddr != "" {
			if owner, exists := usedStreamSNI[strings.ToLower(cdnAddr)]; exists {
				// CDN域名相同时（如多节点共用同一CDN域名但路径不同），允许配置下发，
				// nginx会按path分流。仅记录警告不阻断。
				s.logger.Warn("precheck: CDN SNI reused across nodes (allowed via path-based routing)",
					"sni", cdnAddr, "node", code, "conflicts_with", owner)
			} else {
				usedStreamSNI[strings.ToLower(cdnAddr)] = code
			}
		}
		// REALITY SNI
		if security == "reality" && realitySNI != "" && cdnAddr == "" {
			sni := strings.ToLower(realitySNI)
			// REALITY SNI 允许多个节点共用同一外部伪装域名（如 mesu.apple.com），
			// 因为 REALITY 通过独立的 server_port 区分入站，不影响配置下发。
			// 仅记录警告，不阻断部署。
			if owner, exists := usedStreamSNI[sni]; exists {
				s.logger.Warn("precheck: REALITY SNI reused across nodes (allowed via distinct server_port)",
					"sni", realitySNI, "node", code, "conflicts_with", owner)
			} else {
				usedStreamSNI[sni] = code
			}
		}

		// --- 5. DNS resolvability check (best-effort, warn only for REALITY SNI which can be any public domain) ---
		// CDN address MUST resolve (it's the panel-controlled domain)
		if cdnAddr != "" && !strings.Contains(cdnAddr, "*") {
			if err := checkDNS(ctx, resolver, cdnAddr); err != nil {
				s.logger.Warn("precheck: CDN address DNS lookup failed", "code", code, "cdn_addr", cdnAddr, "error", err)
				// Don't hard-fail: DNS may be blocked from panel's network even though client side resolves
			}
		}
	}

	return nil
}

func (s *DeploymentService) precheckRuntimeGroupedByServer(ctx context.Context, nodes []*model.Node) error {
	// Nodes passed to buildRuntimeConfig all belong to the same runtime (and hence the same server)
	// for single-runtime deploys. Machine-mode aggregation uses BuildNginxVhostsForServer which calls
	// precheckRuntimePerServer directly with all nodes on that server.
	if len(nodes) == 0 {
		return nil
	}
	return s.precheckRuntimePerServer(ctx, nodes)
}

func toStringOrEmpty(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func toFloat64(m map[string]interface{}, key string) (float64, bool) {
	if m == nil {
		return 0, false
	}
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return n, true
		case int:
			return float64(n), true
		case int64:
			return float64(n), true
		}
	}
	return 0, false
}

func checkDNS(ctx context.Context, r *net.Resolver, host string) error {
	ctx2, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	addrs, err := r.LookupHost(ctx2, host)
	if err != nil {
		return err
	}
	if len(addrs) == 0 {
		return fmt.Errorf("no A/AAAA records for %s", host)
	}
	return nil
}
