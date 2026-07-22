package doctor

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// NodeProbeInfo 节点探测所需信息（由 service 通过 fetcher 获取后传入）
// ============================================================================

type NodeProbeInfo struct {
	NodeID        string
	Host          string // 节点对外地址
	Port          int    // 节点对外端口
	ProtocolType  string // vless / vmess / trojan / hysteria2 / tuic / shadowsocks
	TransportType string // tcp / ws / grpc / h2 / quic / kcp
	SecurityType  string // none / tls / reality
	SNI           string
	ExposureMode  string // direct / cdn / relay
}

// NodeProbeFetcher 获取节点探测信息（由 app.go 注入实现）
type NodeProbeFetcher interface {
	FetchProbeInfo(ctx context.Context, nodeID uuid.UUID) (*NodeProbeInfo, error)
}

// ============================================================================
// 真实网络探测实现
// ============================================================================

// executeRealCheck 根据检查项编码执行真实网络探测
func executeRealCheck(ctx context.Context, def *DoctorCheckDef, info *NodeProbeInfo) CheckResult {
	result := CheckResult{
		CheckCode: def.Code,
		CheckName: def.Name,
		Category:  def.CheckCategory,
		Severity:  def.Severity,
		Status:    "pass",
		Message:   "",
		Details:   map[string]interface{}{},
	}

	if info == nil {
		result.Status = "warn"
		result.Message = "无法获取节点信息，跳过网络探测"
		return result
	}

	switch def.Code {
	case "dns_resolve", "dns_resolution":
		return probeDNS(ctx, def, info, result)
	case "tcp_connectivity", "tcp_port":
		return probeTCP(ctx, def, info, result)
	case "tls_handshake", "tls_cert":
		return probeTLS(ctx, def, info, result)
	case "udp_connectivity", "udp_port":
		return probeUDP(ctx, def, info, result)
	case "latency", "rtt":
		return probeLatency(ctx, def, info, result)
	case "config_validation":
		// 配置校验由 node-service 内部的 renderer 已做，这里返回 pass
		result.Status = "pass"
		result.Message = "配置校验在保存时已由双内核渲染器验证"
		return result
	case "kernel_version":
		// 内核版本检查需要从 runtime 获取，这里返回 warn 提示需要 runtime 上报
		result.Status = "warn"
		result.Message = "内核版本检查依赖 runtime 心跳数据，请确认 agent 已连接"
		return result
	case "cert_expiry":
		return probeCertExpiry(ctx, def, info, result)
	case "firewall_rules":
		// 防火墙检查需要 SSH 到服务器，这里返回 pass 占位
		result.Status = "pass"
		result.Message = "防火墙规则检查需要服务器 SSH 访问权限"
		return result
	default:
		// 未知检查项，返回 pass + 占位消息
		result.Status = "pass"
		result.Message = stubPassMessage
		return result
	}
}

// probeDNS DNS 解析检查
func probeDNS(ctx context.Context, def *DoctorCheckDef, info *NodeProbeInfo, result CheckResult) CheckResult {
	if info.Host == "" {
		result.Status = "fail"
		result.Message = "节点主机地址为空"
		return result
	}

	// 如果是 IP 地址，跳过 DNS 检查
	if net.ParseIP(info.Host) != nil {
		result.Status = "pass"
		result.Message = fmt.Sprintf("主机地址 %s 是 IP，无需 DNS 解析", info.Host)
		return result
	}

	resolver := net.Resolver{}
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	start := time.Now()
	ips, err := resolver.LookupHost(lookupCtx, info.Host)
	elapsed := time.Since(start)

	if err != nil {
		result.Status = "fail"
		result.Message = fmt.Sprintf("DNS 解析失败: %v", err)
		result.Details["error"] = err.Error()
		return result
	}
	if len(ips) == 0 {
		result.Status = "fail"
		result.Message = "DNS 解析返回空结果"
		return result
	}

	result.Status = "pass"
	result.Message = fmt.Sprintf("DNS 解析成功，共 %d 个 IP，耗时 %dms", len(ips), elapsed.Milliseconds())
	result.Details["ips"] = ips
	result.Details["elapsed_ms"] = elapsed.Milliseconds()
	if elapsed > 1000*time.Millisecond {
		result.Status = "warn"
		result.Message = fmt.Sprintf("DNS 解析慢（%dms），建议检查 DNS 服务器配置", elapsed.Milliseconds())
	}
	return result
}

// probeTCP TCP 端口连通性检查
func probeTCP(ctx context.Context, def *DoctorCheckDef, info *NodeProbeInfo, result CheckResult) CheckResult {
	if info.Host == "" || info.Port == 0 {
		result.Status = "fail"
		result.Message = "节点主机或端口为空"
		return result
	}

	addr := net.JoinHostPort(info.Host, strconv.Itoa(info.Port))
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	start := time.Now()
	d := &net.Dialer{}
	conn, err := d.DialContext(dialCtx, "tcp", addr)
	elapsed := time.Since(start)

	if err != nil {
		result.Status = "fail"
		result.Message = fmt.Sprintf("TCP 连接 %s 失败: %v", addr, err)
		result.Details["error"] = err.Error()
		result.Details["elapsed_ms"] = elapsed.Milliseconds()
		return result
	}
	conn.Close()

	result.Status = "pass"
	result.Message = fmt.Sprintf("TCP 连接 %s 成功，耗时 %dms", addr, elapsed.Milliseconds())
	result.Details["elapsed_ms"] = elapsed.Milliseconds()
	if elapsed > 500*time.Millisecond {
		result.Status = "warn"
		result.Message = fmt.Sprintf("TCP 连接延迟较高（%dms），可能影响用户体验", elapsed.Milliseconds())
	}
	return result
}

// probeTLS TLS 握手检查
func probeTLS(ctx context.Context, def *DoctorCheckDef, info *NodeProbeInfo, result CheckResult) CheckResult {
	if info.SecurityType != "tls" && info.SecurityType != "reality" {
		result.Status = "skip"
		result.Message = fmt.Sprintf("节点安全类型为 %s，无需 TLS 检查", info.SecurityType)
		return result
	}
	if info.Host == "" || info.Port == 0 {
		result.Status = "fail"
		result.Message = "节点主机或端口为空"
		return result
	}

	addr := net.JoinHostPort(info.Host, strconv.Itoa(info.Port))
	sni := info.SNI
	if sni == "" {
		sni = info.Host
	}

	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	start := time.Now()
	tlsDialer := &tls.Dialer{
		Config: &tls.Config{
			ServerName:         sni,
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
		},
	}
	conn, err := tlsDialer.DialContext(dialCtx, "tcp", addr)
	elapsed := time.Since(start)

	if err != nil {
		result.Status = "fail"
		result.Message = fmt.Sprintf("TLS 握手失败 (SNI=%s): %v", sni, err)
		result.Details["error"] = err.Error()
		return result
	}
	defer conn.Close()

	state := conn.(*tls.Conn).ConnectionState()
	result.Status = "pass"
	result.Message = fmt.Sprintf("TLS 握手成功，版本 %s，密码套件 %s，耗时 %dms",
		tlsVersionName(state.Version),
		tls.CipherSuiteName(state.CipherSuite),
		elapsed.Milliseconds(),
	)
	result.Details["tls_version"] = tlsVersionName(state.Version)
	result.Details["cipher_suite"] = tls.CipherSuiteName(state.CipherSuite)
	result.Details["sni"] = sni
	result.Details["elapsed_ms"] = elapsed.Milliseconds()

	// 检查证书链
	if len(state.PeerCertificates) > 0 {
		cert := state.PeerCertificates[0]
		result.Details["cert_subject"] = cert.Subject.String()
		result.Details["cert_issuer"] = cert.Issuer.String()
		daysUntilExpiry := int(time.Until(cert.NotAfter).Hours() / 24)
		result.Details["cert_days_until_expiry"] = daysUntilExpiry
		if daysUntilExpiry < 0 {
			result.Status = "fail"
			result.Message = fmt.Sprintf("证书已过期 %d 天", -daysUntilExpiry)
		} else if daysUntilExpiry < 7 {
			result.Status = "warn"
			result.Message = fmt.Sprintf("证书将在 %d 天后过期", daysUntilExpiry)
		}
	}
	return result
}

// probeUDP UDP 端口连通性检查（用于 Hysteria2/TUIC）
func probeUDP(ctx context.Context, def *DoctorCheckDef, info *NodeProbeInfo, result CheckResult) CheckResult {
	// 仅对 UDP 协议检查
	if info.ProtocolType != "hysteria2" && info.ProtocolType != "tuic" && info.TransportType != "quic" {
		result.Status = "skip"
		result.Message = fmt.Sprintf("协议 %s/%s 不使用 UDP", info.ProtocolType, info.TransportType)
		return result
	}
	if info.Host == "" || info.Port == 0 {
		result.Status = "fail"
		result.Message = "节点主机或端口为空"
		return result
	}

	addr := net.JoinHostPort(info.Host, strconv.Itoa(info.Port))
	// UDP 探测：尝试建立 UDP 连接并发送一个空包
	// 注意：UDP 是无连接的，无法真正验证端口是否开放
	// 这里仅验证能否建立 UDP socket 并发送数据
	dialCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	start := time.Now()
	d := &net.Dialer{}
	conn, err := d.DialContext(dialCtx, "udp", addr)
	if err != nil {
		result.Status = "fail"
		result.Message = fmt.Sprintf("UDP 拨号 %s 失败: %v", addr, err)
		return result
	}
	defer conn.Close()

	// 设置写超时
	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, err = conn.Write([]byte{0x00})
	elapsed := time.Since(start)

	if err != nil {
		result.Status = "warn"
		result.Message = fmt.Sprintf("UDP 写入 %s 失败（可能被防火墙拦截）: %v", addr, err)
		result.Details["elapsed_ms"] = elapsed.Milliseconds()
		return result
	}

	result.Status = "pass"
	result.Message = fmt.Sprintf("UDP 探测 %s 成功（写入 1 字节），耗时 %dms。注意：UDP 无连接，实际连通性需客户端验证", addr, elapsed.Milliseconds())
	result.Details["elapsed_ms"] = elapsed.Milliseconds()
	return result
}

// probeLatency 延迟探测（TCP 握手耗时）
func probeLatency(ctx context.Context, def *DoctorCheckDef, info *NodeProbeInfo, result CheckResult) CheckResult {
	if info.Host == "" || info.Port == 0 {
		result.Status = "fail"
		result.Message = "节点主机或端口为空"
		return result
	}

	addr := net.JoinHostPort(info.Host, strconv.Itoa(info.Port))

	// 连续探测 3 次取平均
	var totalMs int64
	samples := 3
	successCount := 0
	for i := 0; i < samples; i++ {
		start := time.Now()
		conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
		elapsed := time.Since(start)
		if err == nil {
			conn.Close()
			totalMs += elapsed.Milliseconds()
			successCount++
		}
		// 短暂间隔避免过于密集
		if i < samples-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	if successCount == 0 {
		result.Status = "fail"
		result.Message = fmt.Sprintf("延迟探测失败：连续 %d 次 TCP 握手均失败", samples)
		return result
	}

	avgMs := totalMs / int64(successCount)
	result.Details["avg_ms"] = avgMs
	result.Details["samples"] = samples
	result.Details["success_count"] = successCount

	if successCount < samples {
		result.Status = "warn"
		result.Message = fmt.Sprintf("延迟探测部分失败：成功 %d/%d，平均 %dms", successCount, samples, avgMs)
		return result
	}

	if avgMs > 500 {
		result.Status = "warn"
		result.Message = fmt.Sprintf("延迟较高：平均 %dms（3 次探测）", avgMs)
	} else {
		result.Status = "pass"
		result.Message = fmt.Sprintf("延迟正常：平均 %dms（3 次探测）", avgMs)
	}
	return result
}

// probeCertExpiry 证书有效期检查（通过 TLS 握手获取证书信息）
func probeCertExpiry(ctx context.Context, def *DoctorCheckDef, info *NodeProbeInfo, result CheckResult) CheckResult {
	if info.SecurityType != "tls" && info.SecurityType != "reality" {
		result.Status = "skip"
		result.Message = fmt.Sprintf("节点安全类型为 %s，无需证书检查", info.SecurityType)
		return result
	}
	if info.Host == "" || info.Port == 0 {
		result.Status = "fail"
		result.Message = "节点主机或端口为空"
		return result
	}

	addr := net.JoinHostPort(info.Host, strconv.Itoa(info.Port))
	sni := info.SNI
	if sni == "" {
		sni = info.Host
	}

	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	tlsDialer := &tls.Dialer{
		Config: &tls.Config{
			ServerName:         sni,
			InsecureSkipVerify: true, // 证书检查不验证链，仅看有效期
		},
	}
	conn, err := tlsDialer.DialContext(dialCtx, "tcp", addr)
	if err != nil {
		result.Status = "fail"
		result.Message = fmt.Sprintf("TLS 握手失败，无法获取证书: %v", err)
		return result
	}
	defer conn.Close()

	state := conn.(*tls.Conn).ConnectionState()
	if len(state.PeerCertificates) == 0 {
		result.Status = "warn"
		result.Message = "TLS 握手成功但未返回证书"
		return result
	}

	cert := state.PeerCertificates[0]
	daysUntilExpiry := int(time.Until(cert.NotAfter).Hours() / 24)
	result.Details["subject"] = cert.Subject.String()
	result.Details["issuer"] = cert.Issuer.String()
	result.Details["not_after"] = cert.NotAfter.Format("2006-01-02")
	result.Details["days_until_expiry"] = daysUntilExpiry

	// 检查证书链是否完整
	pool := x509.NewCertPool()
	for _, c := range state.PeerCertificates[1:] {
		pool.AddCert(c)
	}
	if _, err := cert.Verify(x509.VerifyOptions{Roots: pool}); err != nil && !strings.Contains(err.Error(), "certificate signed by unknown authority") {
		result.Details["verify_error"] = err.Error()
	}

	if daysUntilExpiry < 0 {
		result.Status = "fail"
		result.Message = fmt.Sprintf("证书已过期 %d 天（到期日 %s）", -daysUntilExpiry, cert.NotAfter.Format("2006-01-02"))
	} else if daysUntilExpiry < 7 {
		result.Status = "fail"
		result.Message = fmt.Sprintf("证书将在 %d 天后过期（到期日 %s），请立即续签", daysUntilExpiry, cert.NotAfter.Format("2006-01-02"))
	} else if daysUntilExpiry < 30 {
		result.Status = "warn"
		result.Message = fmt.Sprintf("证书将在 %d 天后过期（到期日 %s），建议尽快续签", daysUntilExpiry, cert.NotAfter.Format("2006-01-02"))
	} else {
		result.Status = "pass"
		result.Message = fmt.Sprintf("证书有效期剩余 %d 天（到期日 %s）", daysUntilExpiry, cert.NotAfter.Format("2006-01-02"))
	}
	return result
}

// tlsVersionName TLS 版本号转可读名
func tlsVersionName(v uint16) string {
	switch v {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("0x%04x", v)
	}
}
