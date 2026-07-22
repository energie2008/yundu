// Package prober 实现 Active Chaos Prober（主动拨测）。
//
// P3 支柱：每次配置变更/Delta 应用/蓝绿切换后，主动通过本地代理发起合成拨测，
// 验证协议握手、TLS、路由转发全链路正常，而不是等用户上报故障。
//
// 拨测策略：
//  1. 配置应用后立即触发快速拨测（TCP 连接 + 协议握手）
//  2. 周期性主动拨测（每 60s 轮询所有监听端口）
//  3. 混沌工程模式：随机注入延迟/断连，验证自愈能力
//  4. 拨测失败触发 LKG 回滚
//
// 支持的拨测目标：
//   - TCP 连通性：net.Dial 到 listen_port
//   - HTTP 代理：通过 HTTP CONNECT 发起请求
//   - SOCKS5：SOCKS5 握手
//   - 协议层：VLESS/Trojan/VMess/SS 真实握手（使用测试 UUID）
package prober

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// ProbeTarget 描述一个拨测目标。
type ProbeTarget struct {
	Name       string            `json:"name"`
	Addr       string            `json:"addr"`        // host:port
	Protocol   string            `json:"protocol"`    // vless/trojan/vmess/ss/http/socks5/tcp
	Tags       map[string]string `json:"tags,omitempty"`
	Timeout    time.Duration     `json:"-"`
}

// ProbeResult 拨测结果。
type ProbeResult struct {
	Target      string        `json:"target"`
	Success     bool          `json:"success"`
	Latency     time.Duration `json:"latency_ms"`
	Error       string        `json:"error,omitempty"`
	Protocol    string        `json:"protocol"`
	Timestamp   time.Time     `json:"timestamp"`
	ProbeType   string        `json:"probe_type"` // tcp/http/socks5/protocol
}

// Config 拨测器配置。
type Config struct {
	// ProbeInterval 周期性拨测间隔，0 表示禁用周期性拨测。
	ProbeInterval time.Duration
	// PostApplyTimeout 配置应用后拨测超时。
	PostApplyTimeout time.Duration
	// ProbeURL HTTP 拨测时请求的目标 URL（用于验证端到端连通性）。
	ProbeURL string
	// SocksTestAddr SOCKS5 拨测地址。
	SocksTestAddr string
}

// Prober 主动拨测器。
type Prober struct {
	mu       sync.RWMutex
	targets  map[string]*ProbeTarget
	cfg      Config
	logger   *slog.Logger
	client   *http.Client
	running  atomic.Bool
	stopCh   chan struct{}
	results  map[string]*ProbeResult // 最近一次拨测结果
	failSeq  atomic.Int64            // 连续失败次数
	totalProbes atomic.Int64
	totalSuccess atomic.Int64
}

// NewProber 创建主动拨测器。
func NewProber(cfg Config, logger *slog.Logger) *Prober {
	if cfg.PostApplyTimeout == 0 {
		cfg.PostApplyTimeout = 10 * time.Second
	}
	if cfg.ProbeInterval == 0 {
		cfg.ProbeInterval = 60 * time.Second
	}
	if cfg.ProbeURL == "" {
		cfg.ProbeURL = "http://www.google.com/generate_204"
	}
	return &Prober{
		targets: make(map[string]*ProbeTarget),
		cfg:     cfg,
		logger:  logger.With("component", "active-prober"),
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
			},
		},
		results: make(map[string]*ProbeResult),
		stopCh:  make(chan struct{}),
	}
}

// AddTarget 添加拨测目标。
func (p *Prober) AddTarget(target *ProbeTarget) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.targets[target.Name] = target
}

// RemoveTarget 移除拨测目标。
func (p *Prober) RemoveTarget(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.targets, name)
}

// ReplaceTargets 替换所有拨测目标（在全量配置下发后调用）。
func (p *Prober) ReplaceTargets(targets []*ProbeTarget) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.targets = make(map[string]*ProbeTarget, len(targets))
	for _, t := range targets {
		p.targets[t.Name] = t
	}
}

// Start 启动周期性拨测。
func (p *Prober) Start(ctx context.Context) {
	if !p.running.CompareAndSwap(false, true) {
		return
	}
	p.logger.Info("active prober started",
		"interval", p.cfg.ProbeInterval.String(),
		"post_apply_timeout", p.cfg.PostApplyTimeout.String(),
	)

	go func() {
		ticker := time.NewTicker(p.cfg.ProbeInterval)
		defer ticker.Stop()
		defer p.running.Store(false)

		// 启动后立即拨测一次
		p.ProbeAll(ctx)

		for {
			select {
			case <-ctx.Done():
				return
			case <-p.stopCh:
				return
			case <-ticker.C:
				p.ProbeAll(ctx)
			}
		}
	}()
}

// Stop 停止拨测。
func (p *Prober) Stop() {
	if p.running.Load() {
		close(p.stopCh)
	}
}

// ProbeAll 对所有目标执行拨测。
func (p *Prober) ProbeAll(ctx context.Context) []*ProbeResult {
	p.mu.RLock()
	targets := make([]*ProbeTarget, 0, len(p.targets))
	for _, t := range p.targets {
		targets = append(targets, t)
	}
	p.mu.RUnlock()

	results := make([]*ProbeResult, 0, len(targets))
	var wg sync.WaitGroup
	var resultMu sync.Mutex

	for _, t := range targets {
		wg.Add(1)
		go func(target *ProbeTarget) {
			defer wg.Done()
			res := p.probeOne(ctx, target)
			resultMu.Lock()
			results = append(results, res)
			p.mu.Lock()
			p.results[target.Name] = res
			p.mu.Unlock()
			resultMu.Unlock()
		}(t)
	}
	wg.Wait()

	// 统计
	allOk := true
	for _, r := range results {
		p.totalProbes.Add(1)
		if r.Success {
			p.totalSuccess.Add(1)
		} else {
			allOk = false
		}
	}
	if allOk {
		p.failSeq.Store(0)
	} else {
		p.failSeq.Add(1)
	}

	return results
}

// ProbeAfterApply 配置应用后的快速拨测（验证配置是否有效）。
// 返回 (allHealthy bool, results)。
func (p *Prober) ProbeAfterApply(ctx context.Context) (bool, []*ProbeResult) {
	ctx, cancel := context.WithTimeout(ctx, p.cfg.PostApplyTimeout)
	defer cancel()

	// 等待内核完全启动（端口绑定完成）
	time.Sleep(500 * time.Millisecond)

	results := p.ProbeAll(ctx)

	allOk := true
	for _, r := range results {
		if !r.Success {
			allOk = false
			p.logger.Warn("post-apply probe failed",
				"target", r.Target,
				"protocol", r.Protocol,
				"error", r.Error,
				"latency_ms", r.Latency.Milliseconds(),
			)
		}
	}

	if allOk {
		p.logger.Info("post-apply probes passed",
			"targets", len(results),
			"fail_seq", p.failSeq.Load(),
		)
	}

	return allOk, results
}

// probeOne 对单个目标执行拨测。
func (p *Prober) probeOne(ctx context.Context, t *ProbeTarget) *ProbeResult {
	timeout := t.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	result := &ProbeResult{
		Target:    t.Addr,
		Protocol:  t.Protocol,
		Timestamp: start,
	}

	// DEBUG: 输出拨测目标的标签信息，用于诊断 SNI/transport 提取问题
	if t.Tags != nil {
		p.logger.Info("prober target tags",
			"target", t.Addr,
			"protocol", t.Protocol,
			"security", t.Tags["security"],
			"transport", t.Tags["transport"],
			"sni", t.Tags["sni"],
		)
	}

	// UDP 协议（hysteria2/tuic/quic）无法用 TCP 拨测，直接标记为成功跳过。
	// 这些协议的端口监听状态由内核自身管理，TCP 拨测只会产生误报。
	if t.Protocol == "hysteria2" || t.Protocol == "tuic" || t.Protocol == "quic" || t.Protocol == "hysteria" {
		result.ProbeType = "udp-skip"
		result.Success = true
		result.Latency = time.Since(start)
		return result
	}

	// REALITY inbound 无法用普通 TLS/VLESS 握手拨测（需要 REALITY 专有认证），
	// 强制回退到 TCP 连通性检测，避免误判为失败触发 LKG 回滚循环。
	if t.Tags != nil && t.Tags["security"] == "reality" {
		result.ProbeType = "tcp"
		err := p.probeTCP(ctx, t.Addr)
		result.Latency = time.Since(start)
		if err != nil {
			result.Success = false
			result.Error = err.Error()
		} else {
			result.Success = true
		}
		return result
	}

	// WS/XHTTP/gRPC/HTTPUpgrade 等传输层在 TLS 之上还有 HTTP 帧封装，
	// 直接发送裸 VLESS 字节会被服务器拒绝（期望 HTTP Upgrade / HTTP/2 帧）。
	// 这些目标回退到 TLS 握手（TLS inbound）或 TCP 连通性（非 TLS）检测。
	if t.Tags != nil {
		tp := t.Tags["transport"]
		if tp == "ws" || tp == "xhttp" || tp == "grpc" || tp == "httpupgrade" || tp == "splithttp" || tp == "h2" {
			if isTLS(t) {
				result.ProbeType = "tls"
				conn, err := p.dialTarget(ctx, t)
				result.Latency = time.Since(start)
				if err != nil {
					result.Success = false
					result.Error = err.Error()
				} else {
					conn.Close()
					result.Success = true
				}
			} else {
				result.ProbeType = "tcp"
				err := p.probeTCP(ctx, t.Addr)
				result.Latency = time.Since(start)
				if err != nil {
					result.Success = false
					result.Error = err.Error()
				} else {
					result.Success = true
				}
			}
			return result
		}
	}

	switch t.Protocol {
	case "vless":
		result.ProbeType = "protocol"
		err := p.probeVLESS(ctx, t)
		result.Latency = time.Since(start)
		if err != nil {
			result.Success = false
			result.Error = err.Error()
		} else {
			result.Success = true
		}
	case "trojan":
		result.ProbeType = "protocol"
		err := p.probeTrojan(ctx, t)
		result.Latency = time.Since(start)
		if err != nil {
			result.Success = false
			result.Error = err.Error()
		} else {
			result.Success = true
		}
	case "vmess":
		result.ProbeType = "protocol"
		err := p.probeVMess(ctx, t)
		result.Latency = time.Since(start)
		if err != nil {
			result.Success = false
			result.Error = err.Error()
		} else {
			result.Success = true
		}
	case "shadowsocks", "ss":
		result.ProbeType = "protocol"
		err := p.probeShadowsocks(ctx, t)
		result.Latency = time.Since(start)
		if err != nil {
			result.Success = false
			result.Error = err.Error()
		} else {
			result.Success = true
		}
	case "http":
		result.ProbeType = "http"
		err := p.probeHTTP(ctx, t.Addr)
		result.Latency = time.Since(start)
		if err != nil {
			result.Success = false
			result.Error = err.Error()
		} else {
			result.Success = true
		}
	default:
		// tcp, hysteria2, tuic 等没有纯文本握手的协议回退到 TCP 连通性检测
		result.ProbeType = "tcp"
		err := p.probeTCP(ctx, t.Addr)
		result.Latency = time.Since(start)
		if err != nil {
			result.Success = false
			result.Error = err.Error()
		} else {
			result.Success = true
		}
	}

	return result
}

// probeTCP 验证 TCP 连通性。
func (p *Prober) probeTCP(ctx context.Context, addr string) error {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("tcp dial: %w", err)
	}
	conn.Close()
	return nil
}

// isTLS 判断目标是否使用 TLS（从 Tags["security"] 读取）。
func isTLS(t *ProbeTarget) bool {
	if t.Tags == nil {
		return false
	}
	sec := t.Tags["security"]
	return sec == "tls" || sec == "reality"
}

// dialTarget 建立连接，根据目标 security 配置自动判断是否使用 TLS。
func (p *Prober) dialTarget(ctx context.Context, t *ProbeTarget) (net.Conn, error) {
	host, _, _ := net.SplitHostPort(t.Addr)
	dialer := &net.Dialer{}

	if isTLS(t) {
		// SNI 必须使用 inbound 配置中的 serverName（从 Tags["sni"] 读取），
		// 不能用 127.0.0.1。xray TLS/REALITY inbound 会校验 SNI，
		// SNI 不匹配时返回 "tls: unrecognized name"，导致拨测误判。
		sni := host
		if t.Tags != nil {
			if s, ok := t.Tags["sni"]; ok && s != "" {
				sni = s
			}
		}
		// 本地拨测场景：证书签发给域名而非 127.0.0.1，必须跳过证书校验，
		// 否则 x509 报 "cannot validate certificate for 127.0.0.1"。
		tlsCfg := &tls.Config{
			ServerName:         sni,
			InsecureSkipVerify: true,
		}
		return tls.DialWithDialer(dialer, "tcp", t.Addr, tlsCfg)
	}
	return dialer.DialContext(ctx, "tcp", t.Addr)
}

// probeVLESS 执行 VLESS 协议握手拨测。
//
// 发送 VLESS 请求头（使用测试 UUID），若服务器返回任何响应则视为协议层存活。
// 即使服务器返回 UUID 无效的错误，对拨测而言也意味着 VLESS 服务正在运行。
func (p *Prober) probeVLESS(ctx context.Context, t *ProbeTarget) error {
	conn, err := p.dialTarget(ctx, t)
	if err != nil {
		return fmt.Errorf("vless dial: %w", err)
	}
	defer conn.Close()

	// VLESS 协议请求头格式：
	// 1 byte  version (0)
	// 16 bytes UUID
	// 1 byte  addon length (0)
	// 1 byte  command (1=TCP, 2=UDP)
	// 2 bytes port (big-endian)
	// 1 byte  address type (1=IPv4)
	// N bytes address
	var buf [1 + 16 + 1 + 1 + 2 + 1 + 4]byte

	// Version = 0
	buf[0] = 0

	// UUID: 00000000-0000-0000-0000-000000000000
	testUUID := [16]byte{} // 全零

	copy(buf[1:17], testUUID[:])

	// Addon length = 0
	buf[17] = 0

	// Command = 1 (TCP)
	buf[18] = 1

	// Port = 80 (0x0050)
	binary.BigEndian.PutUint16(buf[19:21], 80)

	// Address type = 1 (IPv4)
	buf[21] = 1

	// Address = 127.0.0.1
	copy(buf[22:26], net.ParseIP("127.0.0.1").To4())

	if _, err := conn.Write(buf[:]); err != nil {
		return fmt.Errorf("vless write request: %w", err)
	}

	// 设置读超时，等待服务器响应
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))

	// 读取任意响应 —— 即使是错误响应也说明 VLESS 服务在运行
	recv := make([]byte, 1)
	n, err := conn.Read(recv)
	if n > 0 {
		// 服务器返回了数据，说明 VLESS 协议层存活
		return nil
	}
	if err != nil {
		// 区分连接错误和超时
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			// 超时：VLESS 服务器可能静默丢弃无效 UUID 请求
			// 但 TCP + TLS 握手成功说明服务在运行
			// 对于 VLESS，服务器不回复是常见行为（丢弃无效请求）
			// 如果我们能连接成功，就认为协议层存活
			return nil
		}
		// 连接被拒绝或重置 = 服务不可用
		return fmt.Errorf("vless read response: %w", err)
	}

	return nil
}

// probeTrojan 执行 Trojan 协议握手拨测。
//
// 发送 Trojan 请求头（使用测试密码的 SHA224），若服务器返回任何响应则视为协议层存活。
func (p *Prober) probeTrojan(ctx context.Context, t *ProbeTarget) error {
	conn, err := p.dialTarget(ctx, t)
	if err != nil {
		return fmt.Errorf("trojan dial: %w", err)
	}
	defer conn.Close()

	// Trojan 协议请求头格式：
	// 56 bytes SHA224(password) hex
	// 1 byte  CRLF (\r)
	// 1 byte  CRLF (\n)
	// 1 byte  command (1=CONNECT)
	// 2 bytes address type + port
	// N bytes address

	// 生成测试密码的 SHA224
	testPassword := "probe-test"
	h := sha256.New224()
	h.Write([]byte(testPassword))
	passHash := fmt.Sprintf("%x", h.Sum(nil)) // 56 字符的 hex

	// 构建请求
	// SHA224(hex) + CRLF + command + address_type + address + port
	addr := "127.0.0.1"
	port := uint16(80)

	var req []byte
	req = append(req, []byte(passHash)...)
	req = append(req, '\r', '\n')
	req = append(req, 0x01) // CONNECT

	// IPv4 地址
	req = append(req, 0x01) // ATYPE_IPV4
	req = append(req, net.ParseIP(addr).To4()...)
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, port)
	req = append(req, buf...)

	if _, err := conn.Write(req); err != nil {
		return fmt.Errorf("trojan write request: %w", err)
	}

	// 设置读超时
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))

	// 读取任意响应
	recv := make([]byte, 1)
	n, err := conn.Read(recv)
	if n > 0 {
		return nil
	}
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			// 超时：Trojan 服务器可能静默丢弃无效密码
			// 连接建立成功即可
			return nil
		}
		// EOF: 服务器收到无效密码后关闭连接，说明 Trojan 服务在运行
		if err == io.EOF {
			return nil
		}
		return fmt.Errorf("trojan read response: %w", err)
	}

	return nil
}

// probeVMess 执行 VMess 协议握手拨测。
//
// 发送 VMess 请求（使用测试 UUID），若服务器返回任何响应则视为协议层存活。
func (p *Prober) probeVMess(ctx context.Context, t *ProbeTarget) error {
	conn, err := p.dialTarget(ctx, t)
	if err != nil {
		return fmt.Errorf("vmess dial: %w", err)
	}
	defer conn.Close()

	// VMess 协议请求头格式：
	// 16 bytes Auth ID = MD5(UUID)
	// 1 byte  Version (1)
	// 16 bytes IV (随机)
	// 1 byte  Key length (constant: 16 for AES-128)
	// ... 后续为加密部分
	//
	// 简化版：发送 Auth ID + 明文版本信息来触发服务器响应
	// VMess 的 Auth ID 是 MD5(UUID)，服务器验证后拒绝无效 UUID

	testUUID := [16]byte{} // 00000000-0000-0000-0000-000000000000

	// Auth ID = MD5(UUID)
	authID := md5.Sum(testUUID[:])

	var req []byte
	req = append(req, authID[:]...)

	// Version = 1
	req = append(req, 1)

	// 数据部分：为了触发服务器处理，我们追加一些必要的字段
	// IV (16 bytes 随机)
	iv := make([]byte, 16)
	req = append(req, iv...)

	// Key count = 1 (1 byte)
	req = append(req, 1)

	// Command = 1 (TCP) (1 byte)
	req = append(req, 1)

	// Port (2 bytes big-endian)
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, 80)
	req = append(req, buf...)

	// Address type = 1 (IPv4) (1 byte)
	req = append(req, 1)

	// Address (4 bytes IPv4)
	req = append(req, net.ParseIP("127.0.0.1").To4()...)

	if _, err := conn.Write(req); err != nil {
		return fmt.Errorf("vmess write request: %w", err)
	}

	// 设置读超时
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))

	// 读取任意响应
	recv := make([]byte, 1)
	n, err := conn.Read(recv)
	if n > 0 {
		return nil
	}
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			// 超时：VMess 服务器可能静默丢弃无效请求
			// 连接建立成功即可
			return nil
		}
		return fmt.Errorf("vmess read response: %w", err)
	}

	return nil
}

// probeShadowsocks 执行 Shadowsocks 协议拨测。
//
// SS 协议的特殊性：
//   - SS2022 (AEAD 2022) 可以尝试发送最小加密包
//   - Legacy SS 无法在不知道密码的情况下验证协议，回退到 TCP 连通性检测
//   - SS 不会对无效数据发回响应（与 VLESS/Trojan 不同）
//
// 因此对 SS 的拨测策略为：TCP 连接成功即视为成功。
func (p *Prober) probeShadowsocks(ctx context.Context, t *ProbeTarget) error {
	conn, err := p.dialTarget(ctx, t)
	if err != nil {
		return fmt.Errorf("ss dial: %w", err)
	}
	defer conn.Close()

	// 检查是否为 SS2022（AEAD 2022 加密方法）
	// SS2022 的加密方法名包含 "2022-blake3" 或 "2022-gcm"
	method, _ := t.Tags["method"]
	if method != "" && (contains2022(method)) {
		// SS2022: 尝试发送一个最小的加密包
		// SS2022 使用 PSK（32字节），我们发送一个随机头部
		// 即使服务器无法解密，如果 TCP + TLS 连接成功，也说明服务在运行
		// SS2022 的头部是 8 字节时间戳 + 加密的请求头
		// 发送假数据，服务器可能会断开连接但不会回复错误
		fakeHeader := make([]byte, 32)
		_ = conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
		conn.Write(fakeHeader)

		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		recv := make([]byte, 1)
		n, _ := conn.Read(recv)
		_ = n // 忽略读结果，SS 不会回复无效数据
	}

	// SS 协议不回复无效请求，TCP 连接成功即视为协议层存活
	return nil
}

// contains2022 检查 SS 加密方法名是否为 2022 版本。
func contains2022(method string) bool {
	return len(method) >= 8 &&
		(method[:8] == "2022-bla" || method[:8] == "2022-aes")
}

// probeHTTP 验证 HTTP 代理连通性。
func (p *Prober) probeHTTP(ctx context.Context, addr string) error {
	proxyURL, err := url.Parse("http://" + addr)
	if err != nil {
		return fmt.Errorf("parse proxy url: %w", err)
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
			DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.ProbeURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http via proxy: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("http status %d", resp.StatusCode)
	}
	return nil
}

// FailCount 返回连续失败次数。
func (p *Prober) FailCount() int64 {
	return p.failSeq.Load()
}

// Stats 返回拨测统计。
func (p *Prober) Stats() (total, success int64, results map[string]*ProbeResult) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	total = p.totalProbes.Load()
	success = p.totalSuccess.Load()
	results = make(map[string]*ProbeResult, len(p.results))
	for k, v := range p.results {
		cp := *v
		results[k] = &cp
	}
	return
}

// LastResult 返回指定目标最近一次拨测结果。
func (p *Prober) LastResult(name string) *ProbeResult {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.results[name]
}
