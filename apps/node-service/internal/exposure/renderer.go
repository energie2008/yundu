package exposure

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/airport-panel/node-service/internal/cert"
)

// RenderNginxServerBlock 渲染标准 nginx server block。
// 含 listen、server_name、ssl_certificate、location（含 WS upgrade 头）、proxy_pass 到 origin_host:origin_port
func RenderNginxServerBlock(e *EdgeExposure, profile *cert.TLSProfile) (string, error) {
	if e == nil {
		return "", ErrConfigRenderFailed
	}
	var b strings.Builder

	listenPort := e.PublicPort
	if listenPort == 0 {
		listenPort = 443
	}
	serverName := ""
	if e.PublicHostname != nil {
		serverName = *e.PublicHostname
	} else if profile != nil && profile.ServerName != nil {
		serverName = *profile.ServerName
	}

	// server block start
	fmt.Fprintf(&b, "server {\n")
	fmt.Fprintf(&b, "    listen %d ssl http2;\n", listenPort)
	fmt.Fprintf(&b, "    listen [::]:%d ssl http2;\n", listenPort)
	if serverName != "" {
		fmt.Fprintf(&b, "    server_name %s;\n", serverName)
	}

	// ssl_certificate: 从 profile 关联的证书路径取（这里仅写占位路径，实际由部署写入）
	if profile != nil {
		if profile.ServerName != nil {
			fmt.Fprintf(&b, "    ssl_server_name %s;\n", *profile.ServerName)
		}
		if len(profile.ALPN) > 0 {
			fmt.Fprintf(&b, "    ssl_alpn %s;\n", quoteJoin(profile.ALPN, " "))
		}
		fmt.Fprintf(&b, "    ssl_protocols %s %s;\n", profile.MinVersion, profile.MaxVersion)
	}
	fmt.Fprintf(&b, "    ssl_certificate /etc/nginx/ssl/%s.crt;\n", e.Code)
	fmt.Fprintf(&b, "    ssl_certificate_key /etc/nginx/ssl/%s.key;\n", e.Code)

	// location with WS upgrade headers, proxy_pass to origin_host:origin_port
	origin := fmt.Sprintf("%s:%d", e.OriginHost, e.OriginPort)
	wsPath := "/"
	if e.NginxWSPath != nil && *e.NginxWSPath != "" {
		wsPath = *e.NginxWSPath
	}
	hostHeader := serverName
	if e.NginxHostHeader != nil && *e.NginxHostHeader != "" {
		hostHeader = *e.NginxHostHeader
	}

	fmt.Fprintf(&b, "    location %s {\n", wsPath)
	fmt.Fprintf(&b, "        proxy_pass http://%s;\n", origin)
	fmt.Fprintf(&b, "        proxy_http_version 1.1;\n")
	fmt.Fprintf(&b, "        proxy_set_header Upgrade $http_upgrade;\n")
	fmt.Fprintf(&b, "        proxy_set_header Connection \"upgrade\";\n")
	fmt.Fprintf(&b, "        proxy_set_header Host %s;\n", hostHeader)
	fmt.Fprintf(&b, "        proxy_set_header X-Real-IP $remote_addr;\n")
	fmt.Fprintf(&b, "        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n")
	fmt.Fprintf(&b, "        proxy_read_timeout 300s;\n")
	fmt.Fprintf(&b, "        proxy_send_timeout 300s;\n")
	fmt.Fprintf(&b, "    }\n")

	if e.NginxExtraConf != nil && *e.NginxExtraConf != "" {
		fmt.Fprintf(&b, "    # extra config\n")
		fmt.Fprintf(&b, "    %s\n", *e.NginxExtraConf)
	}

	fmt.Fprintf(&b, "}\n")
	return b.String(), nil
}

// RenderCloudflaredYAML 渲染 cloudflared config.yml。
// 含 tunnel token、ingress 规则指向 origin
func RenderCloudflaredYAML(e *EdgeExposure) (string, error) {
	if e == nil {
		return "", ErrConfigRenderFailed
	}
	var b strings.Builder
	b.WriteString("tunnel:\n")
	if e.CFTunnelID != nil {
		fmt.Fprintf(&b, "  id: %s\n", *e.CFTunnelID)
	}
	if e.CFTunnelName != nil {
		fmt.Fprintf(&b, "  name: %s\n", *e.CFTunnelName)
	}
	if e.CFTunnelTokenEncrypted != nil && *e.CFTunnelTokenEncrypted != "" {
		// 注意：生产中应解密后注入，这里仅占位
		fmt.Fprintf(&b, "  token: <decrypted:%s>\n", e.Code)
	}

	b.WriteString("credentials-file: /etc/cloudflared/credentials.json\n")
	b.WriteString("origincert: /etc/cloudflared/cert.pem\n\n")

	b.WriteString("ingress:\n")
	hostname := ""
	if e.PublicHostname != nil {
		hostname = *e.PublicHostname
	}
	// cloudflared token 模式：明文 HTTP 回源，xray 必须 security=none（TLS 剥离方案）
	// service 用 http://127.0.0.1:<port> 而非 http://localhost:<port>（IPv6 解析问题）
	origin := fmt.Sprintf("http://%s:%d", e.OriginHost, e.OriginPort)
	fmt.Fprintf(&b, "  - hostname: %s\n", hostname)
	fmt.Fprintf(&b, "    service: %s\n", origin)
	b.WriteString("  - service: http_status:404\n")
	return b.String(), nil
}

// Explanation 返回给运维人员的可读说明
func Explanation(e *EdgeExposure) string {
	if e == nil {
		return ""
	}
	publicHost := ""
	if e.PublicHostname != nil {
		publicHost = *e.PublicHostname
	}
	commonName := "<unspecified>"
	// commonName 取自证书，本批 service 不深入读取证书，故用占位说明
	return fmt.Sprintf(
		"客户端连接的是 %s:%d，Xray 实际监听的是 %s:%d，证书覆盖 %s",
		publicHost, e.PublicPort, e.OriginHost, e.OriginPort, commonName,
	)
}

// HashNginxConf 计算配置内容的 SHA-256（用于 nginx_generated_configs 去重）
func HashNginxConf(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// quoteJoin 将字符串切片用 sep 连接，每个元素加引号（用于 nginx ssl_alpn 指令）
func quoteJoin(parts []string, sep string) string {
	quoted := make([]string, len(parts))
	for i, p := range parts {
		quoted[i] = fmt.Sprintf("%q", p)
	}
	return strings.Join(quoted, sep)
}
