package firewall

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

// Protocol is TCP or UDP.
type Protocol string

const (
	TCP Protocol = "tcp"
	UDP Protocol = "udp"
)

// PortRule represents a port that must be accessible from outside.
type PortRule struct {
	Port     int      `json:"port"`
	Protocol Protocol `json:"protocol"`
}

// Manager abstracts firewall backends. All methods are best-effort:
// they return errors but callers should log and continue.
type Manager interface {
	EnsurePortOpen(port int, proto Protocol) error
	Name() string
}

// Detect auto-detects the active firewall backend.
// Returns nil if no firewall is active (ports are open by default).
func Detect(logger *slog.Logger) Manager {
	// Try ufw first (most user-friendly, wraps iptables)
	if path, err := exec.LookPath("ufw"); err == nil {
		if out, err := exec.Command(path, "status").Output(); err == nil {
			if strings.Contains(string(out), "Status: active") {
				logger.Info("firewall detected: ufw (active)")
				return &ufwManager{binPath: path}
			}
			logger.Debug("ufw found but inactive")
		}
	}

	// Try firewalld
	if path, err := exec.LookPath("firewall-cmd"); err == nil {
		if err := exec.Command(path, "--state").Run(); err == nil {
			logger.Info("firewall detected: firewalld (running)")
			return &firewalldManager{binPath: path}
		}
		logger.Debug("firewall-cmd found but not running")
	}

	// Try iptables — only treat as active if INPUT chain has DROP policy
	if path, err := exec.LookPath("iptables"); err == nil {
		out, err := exec.Command(path, "-S", "INPUT").Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				// Look for default DROP policy: "-P INPUT DROP"
				if strings.HasPrefix(line, "-P INPUT DROP") {
					logger.Info("firewall detected: iptables (INPUT DROP policy)")
					return &iptablesManager{binPath: path}
				}
			}
			logger.Debug("iptables found but INPUT policy is not DROP (open)")
		}
	}

	logger.Info("no active firewall detected, ports are open by default")
	return nil
}

// ExtractPortsFromConfig parses an xray or sing-box config map and extracts
// inbound listen ports with their protocol (TCP/UDP).
func ExtractPortsFromConfig(config map[string]interface{}) []PortRule {
	var rules []PortRule
	seen := make(map[string]bool)

	inbounds, ok := config["inbounds"].([]interface{})
	if !ok {
		return rules
	}

	for _, ib := range inbounds {
		inbound, ok := ib.(map[string]interface{})
		if !ok {
			continue
		}

		// xray uses "port", sing-box uses "listen_port"
		port := extractInt(inbound, "port")
		if port == 0 {
			port = extractInt(inbound, "listen_port")
		}
		if port == 0 || port < 1 || port > 65535 {
			continue
		}

		// Determine protocol type
		proto := TCP
		protoType, _ := inbound["protocol"].(string)
		transport := extractString(inbound, "transport")
		network := extractString(inbound, "network")

		// Hysteria2/TUIC use UDP (QUIC-based)
		if protoType == "hysteria2" || protoType == "tuic" ||
			transport == "quic" || network == "quic" {
			proto = UDP
		}

		key := fmt.Sprintf("%d/%s", port, proto)
		if !seen[key] {
			seen[key] = true
			rules = append(rules, PortRule{Port: port, Protocol: proto})
		}

		// If it's hysteria2/TUIC with port hopping (stored in _port_hopping metadata),
		// also open UDP for the hop range. The hop range is configured via
		// inbound._port_hopping.port_range (sing-box doesn't natively support hop_ports,
		// so kernelrender writes port hopping info to _port_hopping metadata field).
		// DNAT redirects hop range → listen_port, but firewall still needs ACCEPT
		// rules for the hop range ports to allow incoming traffic.
		if hopRange := extractPortHopRange(inbound); hopRange != nil {
			for i := hopRange[0]; i <= hopRange[1]; i++ {
				key := fmt.Sprintf("%d/udp", i)
				if !seen[key] {
					seen[key] = true
					rules = append(rules, PortRule{Port: i, Protocol: UDP})
				}
			}
		}
	}

	return rules
}

// extractInt extracts an integer from a map, handling both int and float64
// (JSON unmarshaling produces float64 for numbers).
func extractInt(m map[string]interface{}, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	}
	return 0
}

func extractString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// extractPortHopRange extracts port hopping range from sing-box hysteria2/TUIC inbound.
// sing-box doesn't natively support hop_ports field (parse error on 1.13.x),
// so port hopping info is stored in inbound._port_hopping metadata field
// (written by kernelrender/singbox.go, stripped before applying to sing-box kernel).
// Returns [start, end] or nil if not configured.
func extractPortHopRange(inbound map[string]interface{}) []int {
	// Primary: read from _port_hopping metadata (kernelrender convention)
	if ph, ok := inbound["_port_hopping"].(map[string]interface{}); ok {
		if enabled, _ := ph["enabled"].(bool); enabled {
			hopPorts := extractString(ph, "port_range")
			if hopPorts == "" {
				return nil
			}
			return parsePortRange(hopPorts)
		}
		return nil
	}
	// Legacy fallback: hop_ports field (deprecated, sing-box rejects it)
	// Kept for backward compatibility with configs rendered before this fix.
	hopPorts := extractString(inbound, "hop_ports")
	if hopPorts == "" {
		// Also check nested obfs or settings
		if settings, ok := inbound["settings"].(map[string]interface{}); ok {
			hopPorts = extractString(settings, "hop_ports")
		}
	}
	if hopPorts == "" {
		return nil
	}
	return parsePortRange(hopPorts)
}

// parsePortRange parses "start-end" port range string into [start, end].
// Returns nil if invalid.
func parsePortRange(hopPorts string) []int {
	parts := strings.SplitN(hopPorts, "-", 2)
	if len(parts) != 2 {
		return nil
	}
	start, err1 := parseIntStr(parts[0])
	end, err2 := parseIntStr(parts[1])
	if err1 != nil || err2 != nil || start > end || start < 1 || end > 65535 {
		return nil
	}
	return []int{start, end}
}

// PortHoppingRule represents a port hopping DNAT rule.
// Traffic to port_range (UDP) is redirected to listen_port (sing-box actual listen port).
type PortHoppingRule struct {
	Tag        string // inbound tag, for idempotent rule management
	PortRange  string // "40020-40200" format
	StartPort  int    // 40020
	EndPort    int    // 40200
	ListenPort int    // 40020 (sing-box actual listen port)
	Protocol   string // "udp" (hysteria2/TUIC are UDP-based)
}

// ExtractPortHoppingFromSingboxConfig extracts _port_hopping metadata from each
// sing-box inbound, strips the field from the config (so sing-box kernel won't
// reject it as "unknown field"), and returns the list of port hopping rules.
//
// This must be called BEFORE passing singboxConfig to StartNative, otherwise
// sing-box JSON parsing will fail with: "inbounds[N]._port_hopping: json: unknown field".
//
// Returns nil if singboxConfig is nil or no inbound has _port_hopping.
func ExtractPortHoppingFromSingboxConfig(singboxConfig map[string]interface{}, logger *slog.Logger) []PortHoppingRule {
	if singboxConfig == nil {
		return nil
	}
	inbounds, ok := singboxConfig["inbounds"].([]interface{})
	if !ok {
		return nil
	}

	var rules []PortHoppingRule
	for _, ib := range inbounds {
		inbound, ok := ib.(map[string]interface{})
		if !ok {
			continue
		}
		phRaw, exists := inbound["_port_hopping"]
		if !exists {
			continue
		}
		ph, ok := phRaw.(map[string]interface{})
		if !ok {
			continue
		}
		enabled, _ := ph["enabled"].(bool)
		if !enabled {
			// Strip even if disabled, to keep config clean
			delete(inbound, "_port_hopping")
			continue
		}
		portRange := extractString(ph, "port_range")
		listenPort := extractInt(ph, "listen_port")
		if listenPort == 0 {
			listenPort = extractInt(inbound, "listen_port")
		}
		tag := extractString(inbound, "tag")
		if portRange == "" || listenPort == 0 {
			logger.Warn("port hopping rule skipped: missing port_range or listen_port",
				"tag", tag, "port_range", portRange, "listen_port", listenPort)
			delete(inbound, "_port_hopping")
			continue
		}
		rng := parsePortRange(portRange)
		if rng == nil {
			logger.Warn("port hopping rule skipped: invalid port_range",
				"tag", tag, "port_range", portRange)
			delete(inbound, "_port_hopping")
			continue
		}
		// Determine protocol from inbound type
		proto := "udp"
		if protoType, _ := inbound["type"].(string); protoType == "hysteria2" || protoType == "tuic" {
			proto = "udp"
		}
		rules = append(rules, PortHoppingRule{
			Tag:        tag,
			PortRange:  portRange,
			StartPort:  rng[0],
			EndPort:    rng[1],
			ListenPort: listenPort,
			Protocol:   proto,
		})
		// ★ Critical: strip _port_hopping from inbound before sing-box parses it
		delete(inbound, "_port_hopping")
		logger.Info("port hopping rule extracted",
			"tag", tag, "port_range", portRange,
			"listen_port", listenPort, "protocol", proto)
	}
	return rules
}

// ApplyPortHoppingDNAT applies iptables/nftables DNAT rules to redirect
// port hopping range traffic to the actual sing-box listen port.
//
// For each rule: traffic to UDP ports [StartPort..EndPort] is redirected to
// ListenPort (where sing-box actually listens). This is the standard solution
// for sing-box not supporting native port hopping.
//
// Rules are idempotent: re-applying the same rule is a no-op (iptables -C check).
// Rules are tagged with a comment for easy identification and cleanup.
//
// Reference: https://v2.hysteria.network/docs/advanced/Port-Hopping/#server
//            https://sqybi.com/blog/solving-the-issue-openwrt-homeproxy-hysteria2-port-hopping/
func ApplyPortHoppingDNAT(rules []PortHoppingRule, logger *slog.Logger) error {
	if len(rules) == 0 {
		return nil
	}
	path, err := exec.LookPath("iptables")
	if err != nil {
		// Try nftables fallback (rare on Ubuntu/Debian servers)
		if nftPath, nftErr := exec.LookPath("nft"); nftErr == nil {
			return applyPortHoppingNft(nftPath, rules, logger)
		}
		logger.Warn("no iptables/nft binary found, port hopping DNAT skipped")
		return nil
	}

	var errs []error
	for _, r := range rules {
		// iptables DNAT range uses "start-end" format (colon also works but dash is clearer)
		dportRange := fmt.Sprintf("%d:%d", r.StartPort, r.EndPort)
		// Use --to-destination :PORT to keep dest IP unchanged, only rewrite dest port
		toDest := fmt.Sprintf(":%d", r.ListenPort)

		// Idempotent: check if rule already exists
		check := exec.Command(path, "-t", "nat", "-C", "PREROUTING",
			"-p", r.Protocol,
			"--dport", dportRange,
			"-j", "DNAT",
			"--to-destination", toDest)
		if check.Run() == nil {
			logger.Debug("port hopping DNAT rule already exists",
				"tag", r.Tag, "range", r.PortRange, "listen_port", r.ListenPort)
			continue
		}

		// Add the rule
		add := exec.Command(path, "-t", "nat", "-A", "PREROUTING",
			"-p", r.Protocol,
			"--dport", dportRange,
			"-j", "DNAT",
			"--to-destination", toDest)
		if out, err := add.CombinedOutput(); err != nil {
			errs = append(errs, fmt.Errorf("iptables DNAT add failed for tag=%s range=%s: %w, out: %s",
				r.Tag, r.PortRange, err, string(out)))
		} else {
			logger.Info("port hopping DNAT rule applied",
				"tag", r.Tag, "range", r.PortRange,
				"listen_port", r.ListenPort, "protocol", r.Protocol)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%d/%d DNAT rules failed: %v", len(errs), len(rules), errs[0])
	}
	return nil
}

// applyPortHoppingNft applies port hopping DNAT rules using nftables.
// Used as fallback when iptables binary is not available (rare on YunDu VPS).
func applyPortHoppingNft(nftPath string, rules []PortHoppingRule, logger *slog.Logger) error {
	// nft add rule inet nat prerouting udp dport 40020-40200 dnat to :40020
	var errs []error
	for _, r := range rules {
		dportRange := fmt.Sprintf("%d-%d", r.StartPort, r.EndPort)
		toDest := fmt.Sprintf(":%d", r.ListenPort)
		cmd := exec.Command(nftPath, "add", "rule", "inet", "nat", "prerouting",
			r.Protocol, "dport", dportRange, "dnat", "to", toDest)
		if out, err := cmd.CombinedOutput(); err != nil {
			errs = append(errs, fmt.Errorf("nft DNAT add failed for tag=%s range=%s: %w, out: %s",
				r.Tag, r.PortRange, err, string(out)))
		} else {
			logger.Info("port hopping DNAT rule applied (nft)",
				"tag", r.Tag, "range", r.PortRange, "listen_port", r.ListenPort)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%d/%d nft DNAT rules failed: %v", len(errs), len(rules), errs[0])
	}
	return nil
}

func parseIntStr(s string) (int, error) {
	s = strings.TrimSpace(s)
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// ---------- iptables backend ----------

type iptablesManager struct {
	binPath string
}

func (m *iptablesManager) Name() string { return "iptables" }

func (m *iptablesManager) EnsurePortOpen(port int, proto Protocol) error {
	// Check if rule already exists (idempotent)
	check := exec.Command(m.binPath, "-C", "INPUT", "-p", string(proto),
		"--dport", fmt.Sprintf("%d", port), "-j", "ACCEPT")
	if check.Run() == nil {
		return nil // Rule already exists
	}

	// Add the rule
	add := exec.Command(m.binPath, "-A", "INPUT", "-p", string(proto),
		"--dport", fmt.Sprintf("%d", port), "-j", "ACCEPT")
	if out, err := add.CombinedOutput(); err != nil {
		return fmt.Errorf("iptables -A failed: %w, output: %s", err, string(out))
	}
	return nil
}

// ---------- ufw backend ----------

type ufwManager struct {
	binPath string
}

func (m *ufwManager) Name() string { return "ufw" }

func (m *ufwManager) EnsurePortOpen(port int, proto Protocol) error {
	// ufw allow is idempotent (doesn't error on duplicate)
	cmd := exec.Command(m.binPath, "allow", fmt.Sprintf("%d/%s", port, proto))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ufw allow failed: %w, output: %s", err, string(out))
	}
	return nil
}

// ---------- firewalld backend ----------

type firewalldManager struct {
	binPath string
}

func (m *firewalldManager) Name() string { return "firewalld" }

func (m *firewalldManager) EnsurePortOpen(port int, proto Protocol) error {
	// firewall-cmd --permanent --add-port=PORT/proto (idempotent)
	portSpec := fmt.Sprintf("%d/%s", port, proto)
	perm := exec.Command(m.binPath, "--permanent", "--add-port", portSpec)
	if out, err := perm.CombinedOutput(); err != nil {
		// "ALREADY_ENABLED" is not an error, just info
		if !strings.Contains(string(out), "ALREADY_ENABLED") {
			return fmt.Errorf("firewall-cmd --permanent failed: %w, output: %s", err, string(out))
		}
	}
	// Reload to apply
	reload := exec.Command(m.binPath, "--reload")
	if out, err := reload.CombinedOutput(); err != nil {
		return fmt.Errorf("firewall-cmd --reload failed: %w, output: %s", err, string(out))
	}
	return nil
}

// SyncPorts ensures all specified ports are open in the firewall.
// If no firewall manager is provided, it's a no-op.
// Returns the number of ports successfully opened.
func SyncPorts(mgr Manager, rules []PortRule, logger *slog.Logger) int {
	if mgr == nil || len(rules) == 0 {
		return 0
	}

	opened := 0
	for _, rule := range rules {
		if err := mgr.EnsurePortOpen(rule.Port, rule.Protocol); err != nil {
			logger.Warn("failed to open port",
				"port", rule.Port, "proto", rule.Protocol,
				"firewall", mgr.Name(), "error", err)
		} else {
			opened++
		}
	}

	if opened > 0 {
		logger.Info("firewall ports synced",
			"firewall", mgr.Name(),
			"ports_opened", opened,
			"total_requested", len(rules))
	}
	return opened
}

type DropRule struct {
	PortRange string
	Protocol  Protocol
}

var defaultDropRules = []DropRule{
	{PortRange: "8081:8090", Protocol: TCP},
	{PortRange: "9082", Protocol: TCP},
	{PortRange: "5433", Protocol: TCP},
	{PortRange: "6380", Protocol: TCP},
	{PortRange: "4223", Protocol: TCP},
	{PortRange: "8446:8600", Protocol: TCP},
	{PortRange: "20530:20699", Protocol: TCP},
	{PortRange: "8445", Protocol: TCP},
}

var dockerACCEPT = []struct {
	Source    string
	Interface string
}{
	{Source: "172.16.0.0/12", Interface: ""},
	{Source: "10.0.0.0/8", Interface: ""},
	{Source: "192.168.0.0/16", Interface: ""},
}

func ApplyDefaultRules(logger *slog.Logger) error {
	path, err := exec.LookPath("iptables")
	if err != nil {
		logger.Debug("iptables not found, skipping default rules")
		return nil
	}

	ruleExists := func(args ...string) bool {
		checkArgs := append([]string{"-C", "INPUT"}, args...)
		return exec.Command(path, checkArgs...).Run() == nil
	}

	appendRule := func(args ...string) error {
		if ruleExists(args...) {
			return nil
		}
		addArgs := append([]string{"-A", "INPUT"}, args...)
		if out, err := exec.Command(path, addArgs...).CombinedOutput(); err != nil {
			return fmt.Errorf("iptables %v failed: %w, output: %s", addArgs, err, string(out))
		}
		return nil
	}

	insertRule := func(pos int, args ...string) error {
		if ruleExists(args...) {
			return nil
		}
		addArgs := append([]string{"-I", "INPUT", fmt.Sprintf("%d", pos)}, args...)
		if out, err := exec.Command(path, addArgs...).CombinedOutput(); err != nil {
			return fmt.Errorf("iptables %v failed: %w, output: %s", addArgs, err, string(out))
		}
		return nil
	}

	allowLoopback := []string{"-i", "lo", "-j", "ACCEPT"}
	if err := insertRule(1, allowLoopback...); err != nil {
		logger.Warn("failed to add loopback ACCEPT rule", "error", err)
	}

	allowEstablished := []string{"-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"}
	if err := insertRule(2, allowEstablished...); err != nil {
		logger.Warn("failed to add ESTABLISHED ACCEPT rule", "error", err)
	}

	pos := 3
	for _, d := range dockerACCEPT {
		if err := insertRule(pos, "-s", d.Source, "-j", "ACCEPT"); err != nil {
			logger.Warn("failed to add Docker ACCEPT rule", "source", d.Source, "error", err)
		} else {
			pos++
		}
	}

	allowedPorts := []int{22, 80, 443}
	for _, p := range allowedPorts {
		if err := appendRule("-p", "tcp", "--dport", fmt.Sprintf("%d", p), "-j", "ACCEPT"); err != nil {
			logger.Warn("failed to add public ACCEPT rule", "port", p, "error", err)
		}
	}

	var errs []error
	for _, r := range defaultDropRules {
		ports := r.PortRange
		var args []string
		args = append(args, "-p", string(r.Protocol))
		args = append(args, "--dport", ports)
		args = append(args, "-j", "DROP")
		if err := appendRule(args...); err != nil {
			errs = append(errs, fmt.Errorf("port %s: %w", ports, err))
		}
	}

	if len(errs) > 0 {
		logger.Warn("some default DROP rules failed to apply", "errors", errs)
		return fmt.Errorf("%d/%d rules failed", len(errs), len(defaultDropRules))
	}

	logger.Info("default iptables rules applied",
		"drop_rules", len(defaultDropRules),
		"allowed_tcp", allowedPorts)
	return nil
}
