package nginx

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	yunduMarker     = "# YunDu managed"
	defaultNginxDir = "/etc/nginx"
)

const (
	yunduNginxVhostsDir = "/etc/yundu/nginx/vhosts"
	yunduNginxStreamDir = "/etc/yundu/nginx/stream"
	acmeChallengeDir    = "/var/www/html/.well-known/acme-challenge"
	yunduDefaultCertDir = "/etc/yundu/certs/default"
	yunduStreamConf     = yunduNginxStreamDir + "/yundu_autogen.conf"
	yunduVhostConf      = yunduNginxVhostsDir + "/yundu_autogen.conf"
)

func getNginxDir() string {
	if dir := os.Getenv("NGINX_DIR"); dir != "" {
		return dir
	}
	// 宝塔环境检测：/www/server/nginx/conf/nginx.conf 存在
	if _, err := os.Stat("/www/server/nginx/conf/nginx.conf"); err == nil {
		return "/www/server/nginx"
	}
	return defaultNginxDir
}

func atomicWrite(path string, content []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create dir %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".yundu-nginx-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	cleanup := func() {
		_ = os.Remove(tmpName)
	}

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

func fileHasMarker(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), yunduMarker)
}

func generateDefaultCert(logger *slog.Logger, certDir string) error {
	certPath := filepath.Join(certDir, "fullchain.pem")
	keyPath := filepath.Join(certDir, "privkey.pem")

	if _, err := os.Stat(certPath); err == nil {
		if _, err := os.Stat(keyPath); err == nil {
			logger.Debug("default cert already exists, skipping generation",
				"cert", certPath, "key", keyPath)
			return nil
		}
	}

	if err := os.MkdirAll(certDir, 0755); err != nil {
		return fmt.Errorf("create cert dir: %w", err)
	}

	logger.Info("generating default self-signed certificate",
		"cert", certPath, "key", keyPath)

	cmd := exec.Command(
		"openssl", "req", "-x509", "-newkey", "rsa:2048",
		"-sha256", "-nodes",
		"-keyout", keyPath,
		"-out", certPath,
		"-days", "3650",
		"-subj", "/CN=yundu-default",
	)

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("openssl generate cert failed: %w\n%s", err, out)
	}

	return nil
}

func EnsureNginxSkeleton(logger *slog.Logger) error {
	if runtime.GOOS != "linux" {
		logger.Debug("skipping nginx skeleton generation (not linux)")
		return nil
	}

	nginxBin, err := exec.LookPath("nginx")
	if err != nil {
		logger.Warn("nginx not found in PATH, skipping skeleton generation (agent may run without nginx for UDP-only nodes)")
		return nil
	}

	logger.Info("ensuring nginx skeleton configuration", "nginx_bin", nginxBin)

	nginxDir := getNginxDir()
	nginxConf := filepath.Join(nginxDir, "nginx.conf")

	dirs := []string{
		yunduNginxVhostsDir,
		yunduNginxStreamDir,
		acmeChallengeDir,
		yunduDefaultCertDir,
		filepath.Join(nginxDir, "conf.d"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create dir %s: %w", dir, err)
		}
		logger.Debug("ensured directory exists", "dir", dir)
	}

	if err := generateDefaultCert(logger, yunduDefaultCertDir); err != nil {
		logger.Warn("failed to generate default cert, continuing", "error", err)
	}

	existingConf, err := os.ReadFile(nginxConf)
	existingHasMarker := err == nil && strings.Contains(string(existingConf), yunduMarker)
	confExists := err == nil

	if !confExists || existingHasMarker {
		skeletonConf := `# YunDu managed skeleton — DO NOT EDIT MANUALLY
user www-data;
worker_processes auto;
pid /run/nginx.pid;
error_log /var/log/nginx/error.log warn;

load_module /usr/lib/nginx/modules/ngx_stream_module.so;

events {
    worker_connections 4096;
    multi_accept on;
}

http {
    include /etc/nginx/mime.types;
    default_type application/octet-stream;
    sendfile on;
    tcp_nopush on;
    keepalive_timeout 65;

    server {
        listen 80 default_server;
        listen [::]:80 default_server;
        server_name _;

        location ^~ /.well-known/acme-challenge/ {
            root /var/www/html;
            default_type "text/plain";
            try_files $uri =404;
        }

        location / {
            return 301 https://$host$request_uri;
        }
    }

    # P2 TLS分离架构改造 719：删除默认 8445 HTTP server block。
    # CDN 节点改为 stream TCP 透传后不再需要 nginx HTTP 回源入口。
    # 当没有任何节点生成 WSVhostEntry 时，8445 端口不再监听。
    # HTTP 回源入口完全由 yundu_autogen.conf（vhosts/*.conf）按需要生成。

    include /etc/nginx/conf.d/*.conf;
    include /etc/yundu/nginx/vhosts/*.conf;
}

stream {
    include /etc/yundu/nginx/stream/*.conf;
}
`
		if err := atomicWrite(nginxConf, []byte(skeletonConf), 0644); err != nil {
			return fmt.Errorf("write nginx.conf: %w", err)
		}
		logger.Info("nginx skeleton config written", "path", nginxConf)

		// Ensure initial stream snippet exists so that nginx -t passes before first dynamic config arrives.
		// RenderStreamSnippet on the panel will overwrite this with REALITY/CDN upstreams.
		initialStream := `# YunDu managed — initial skeleton (no upstreams yet; deployment will overwrite)
map $ssl_preread_server_name $internal_upstream {
    default "";
}
upstream yundu_placeholder { server 127.0.0.1:1; }
server {
    listen 443 reuseport;
    listen [::]:443 reuseport;
    proxy_pass $internal_upstream;
    ssl_preread on;
    proxy_connect_timeout 2s;
    proxy_timeout 30s;
}
`
		if _, err := os.Stat(yunduStreamConf); os.IsNotExist(err) {
			if err := atomicWrite(yunduStreamConf, []byte(initialStream), 0644); err != nil {
				logger.Warn("failed to write initial stream snippet, continuing", "error", err)
			} else {
				logger.Info("initial stream snippet written", "path", yunduStreamConf)
			}
		}
	} else {
		// 现有 nginx.conf 不含 YunDu marker（例如 VPS206 手工配置版、宝塔共存版）：
		// 不全量覆盖（会破坏用户自定义配置），改为在 stream/http 块内追加 include。
		// 这样：
		//   1. 多 VPS 走同一套标准路径（/etc/yundu/nginx/stream/*.conf 和
		//      /etc/yundu/nginx/vhosts/*.conf），符合架构文档 §7.3
		//   2. agent 完全自管，零 SSH 也能跨 VPS 标准化
		//   3. conf.d 留给用户/宝塔，不污染
		if err := injectNginxIncludes(logger, nginxConf); err != nil {
			logger.Warn("inject includes failed, falling back to conf.d paths",
				"path", nginxConf, "error", err)
			// 降级：把 stream snippet 路径切到 conf.d，sync.go 已经支持（DefaultBtPanelConfig）
			// 这里只打日志，sync.go 的 DefaultSkeletonSyncConfig 由后续调用方根据 inject 结果选择
		}
	}

	out, testErr := exec.Command(nginxBin, "-t").CombinedOutput()
	if testErr != nil {
		logger.Warn("nginx -t test after skeleton setup failed (may be first-time setup before certs exist)",
			"output", string(out), "error", testErr)
	} else {
		logger.Info("nginx config test passed after skeleton setup")
	}

	return nil
}

func DefaultSkeletonSyncConfig() *SyncConfig {
	return &SyncConfig{
		StreamSnippetPath: yunduStreamConf,
		HTTPSSnippetPath:  yunduVhostConf,
		NginxBinaryPath:   "nginx",
	}
}

// injectNginxIncludes 在现有 nginx.conf 的 stream { ... } 和 http { ... } 块内
// 智能追加标准 include 路径，实现"现有配置不破坏、新配置自动接管"。
//
// 行为：
//   - 检测 stream 块是否已 include yunduStreamDir/*.conf，没有则在闭合 } 前插入
//   - 检测 http 块是否已 include yunduNginxVhostsDir/*.conf，没有则在闭合 } 前插入
//   - 插入前备份 nginx.conf 到 .bak-yundu-inject-{timestamp}
//   - 写入后跑 nginx -t，失败回滚；成功 reload
//   - 幂等：重复调用不会重复 include
//
// 这是方案 A 的核心实现：让所有 VPS 走同一套标准路径，不依赖 conf.d，
// 不破坏用户已有 nginx.conf，真正实现零 SSH 标准化。
func injectNginxIncludes(logger *slog.Logger, nginxConfPath string) error {
	data, err := os.ReadFile(nginxConfPath)
	if err != nil {
		return fmt.Errorf("read nginx.conf: %w", err)
	}
	content := string(data)

	streamInclude := "include " + yunduNginxStreamDir + "/*.conf;"
	httpInclude := "include " + yunduNginxVhostsDir + "/*.conf;"

	newContent := content
	streamInjected := false
	httpInjected := false

	// 1. 注入 stream 块 include
	if !containsInclude(content, yunduNginxStreamDir) {
		idx, ok := findBlockEnd(content, "stream")
		if ok {
			newContent = newContent[:idx] + "    " + streamInclude + "\n" + newContent[idx:]
			streamInjected = true
			logger.Info("inject stream include", "path", yunduNginxStreamDir)
		} else {
			// 没有 stream 块，追加一个
			newContent = newContent + "\nstream {\n    " + streamInclude + "\n}\n"
			streamInjected = true
			logger.Info("append new stream block with include", "path", yunduNginxStreamDir)
		}
	} else {
		logger.Debug("stream include already present, skipping",
			"path", yunduNginxStreamDir)
	}

	// 2. 注入 http 块 include
	if !containsInclude(newContent, yunduNginxVhostsDir) {
		idx, ok := findBlockEnd(newContent, "http")
		if ok {
			newContent = newContent[:idx] + "    " + httpInclude + "\n" + newContent[idx:]
			httpInjected = true
			logger.Info("inject http include", "path", yunduNginxVhostsDir)
		} else {
			return fmt.Errorf("no http block found in nginx.conf (required)")
		}
	} else {
		logger.Debug("http include already present, skipping",
			"path", yunduNginxVhostsDir)
	}

	// 3. 无变更：直接返回
	if !streamInjected && !httpInjected {
		logger.Info("all standard includes already present, no injection needed",
			"nginx_conf", nginxConfPath)
		return nil
	}

	// 4. 备份原文件
	ts := time.Now().Unix()
	bakPath := fmt.Sprintf("%s.bak-yundu-inject-%d", nginxConfPath, ts)
	if err := os.WriteFile(bakPath, data, 0644); err != nil {
		return fmt.Errorf("backup nginx.conf: %w", err)
	}
	logger.Info("backed up original nginx.conf", "bak", bakPath)

	// 5. 写入新内容（先 tmp + rename 原子替换）
	if err := atomicWrite(nginxConfPath, []byte(newContent), 0644); err != nil {
		// 回滚
		os.WriteFile(nginxConfPath, data, 0644)
		return fmt.Errorf("write new nginx.conf: %w", err)
	}

	// 6. nginx -t 校验
	nginxBin, _ := exec.LookPath("nginx")
	if nginxBin == "" {
		nginxBin = "nginx"
	}
	testOut, testErr := exec.Command(nginxBin, "-t").CombinedOutput()
	if testErr != nil {
		// 检查是否是 443 端口冲突（用户原有 stream 块已有 listen 443，yundu_autogen.conf 也有）
		testOutStr := string(testOut)
		if strings.Contains(testOutStr, "duplicate") && strings.Contains(testOutStr, "443") {
			logger.Info("port 443 conflict detected, commenting out original stream listen 443",
				"error", testOutStr)
			// 注释掉原有 nginx.conf 中 stream 块的 listen 443 server（yundu_autogen.conf 会接管）
			newContent = commentOutStreamListen443(newContent)
			if err := atomicWrite(nginxConfPath, []byte(newContent), 0644); err != nil {
				os.WriteFile(nginxConfPath, data, 0644)
				return fmt.Errorf("write nginx.conf after 443 fix: %w", err)
			}
			// 重新测试
			testOut2, testErr2 := exec.Command(nginxBin, "-t").CombinedOutput()
			if testErr2 != nil {
				os.WriteFile(nginxConfPath, data, 0644)
				return fmt.Errorf("nginx -t still failed after 443 fix: %w\n%s", testErr2, string(testOut2))
			}
			logger.Info("nginx -t passed after commenting out original 443 listener")
		} else {
			// 其他错误：回滚
			os.WriteFile(nginxConfPath, data, 0644)
			return fmt.Errorf("nginx -t failed after inject: %w\n%s", testErr, testOutStr)
		}
	}

	// 7. reload
	if err := exec.Command(nginxBin, "-s", "reload").Run(); err != nil {
		// nginx -t 已通过，配置合法但 reload 失败，仍视为成功（下次 reload 会生效）
		logger.Warn("nginx reload failed after inject, config is valid",
			"error", err)
	} else {
		logger.Info("nginx reloaded after include inject")
	}

	return nil
}

// containsInclude 检测 nginx.conf 内容是否已包含指定目录的 include。
// 简单字符串匹配（已过滤 # 注释），不做完整 nginx config 解析。
func containsInclude(content, dir string) bool {
	target := "include " + dir + "/"
	for _, line := range strings.Split(content, "\n") {
		// 去除行尾注释
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = line[:idx]
		}
		if strings.Contains(strings.TrimSpace(line), target) {
			return true
		}
	}
	return false
}

// findBlockEnd 找到指定顶层块（"stream" 或 "http"）的最外层闭合 } 位置。
// 返回闭合 } 所在字节位置（用于在该位置前插入内容）。
//
// 块识别规则：
//   - 行首（去除前导空白后）以块名 + { 开头视为该块的开始
//   - 用花括号嵌套计数找到匹配的 }
//   - 字符串字面量内的 { } 不计数（不严谨但够用：nginx config 不常用字符串）
//
// 例：
//   input:  "stream { map ...; server { listen 443; } server { listen 80; } }"
//   block:  "stream"
//   output: idx=末尾 } 位置, ok=true
func findBlockEnd(content, blockName string) (int, bool) {
	lines := strings.Split(content, "\n")
	depth := 0
	started := false
	bytePos := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// 去除行注释
		if idx := strings.Index(trimmed, "#"); idx >= 0 {
			trimmed = strings.TrimSpace(trimmed[:idx])
		}

		if !started {
			// 找块起始：行首是 "blockName" + 可选空白 + {
			if strings.HasPrefix(trimmed, blockName) {
				rest := strings.TrimSpace(trimmed[len(blockName):])
				if rest == "" || strings.HasPrefix(rest, "{") {
					started = true
					// 统计该行所有花括号（处理起始行就有嵌套的情况）
					for i, ch := range line {
						if ch == '{' {
							depth++
						} else if ch == '}' {
							depth--
							if depth == 0 {
								return bytePos + i, true
							}
						}
					}
				}
			}
		} else {
			// 已开始，统计本行 { } 数量
			for i, ch := range line {
				if ch == '{' {
					depth++
				} else if ch == '}' {
					depth--
					if depth == 0 {
						return bytePos + i, true
					}
				}
			}
		}
		bytePos += len(line) + 1 // +1 for \n
	}
	return 0, false
}

// commentOutStreamListen443 注释掉 stream 块内监听 443 的 server 块。
// 当用户原有 nginx.conf 的 stream 块已有 listen 443 server，
// 而 yundu_autogen.conf 也监听 443 时，需要注释掉原有 server 块避免冲突。
func commentOutStreamListen443(content string) string {
	lines := strings.Split(content, "\n")
	streamDepth := 0 // 0 = 不在 stream 块；>0 = stream 块内深度
	var result []string

	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// 检测 stream 块开始
		if streamDepth == 0 && strings.HasPrefix(trimmed, "stream") {
			rest := strings.TrimSpace(trimmed[len("stream"):])
			if rest == "" || strings.HasPrefix(rest, "{") {
				streamDepth = 0
				for _, ch := range line {
					if ch == '{' {
						streamDepth++
					} else if ch == '}' {
						streamDepth--
					}
				}
				result = append(result, line)
				i++
				continue
			}
		}

		// 不在 stream 块内
		if streamDepth == 0 {
			result = append(result, line)
			i++
			continue
		}

		// 在 stream 块内，检测 server 块开始
		if strings.HasPrefix(trimmed, "server") && strings.Contains(trimmed, "{") {
			// 收集整个 server 块
			blockStart := i
			depth := 0
			for _, ch := range line {
				if ch == '{' {
					depth++
				} else if ch == '}' {
					depth--
				}
			}
			// 继续读取直到 server 块结束
			for depth > 0 && i+1 < len(lines) {
				i++
				for _, ch := range lines[i] {
					if ch == '{' {
						depth++
					} else if ch == '}' {
						depth--
					}
				}
			}
			// 检查整个 server 块是否含 listen 443
			blockText := strings.Join(lines[blockStart:i+1], "\n")
			if strings.Contains(blockText, "listen") && strings.Contains(blockText, "443") {
				for j := blockStart; j <= i; j++ {
					result = append(result, "# "+lines[j]+" # disabled by yundu (443 conflict)")
				}
				// server 块结束，更新 stream 深度（server 块的 } 已经被消费）
				streamDepth--
				i++
				continue
			}
			// 不含 443，正常添加
			for j := blockStart; j <= i; j++ {
				result = append(result, lines[j])
			}
			// 更新 stream 深度
			for _, ch := range blockText {
				if ch == '}' {
					streamDepth--
				}
			}
			i++
			continue
		}

		// 其他行：更新 stream 深度
		for _, ch := range line {
			if ch == '{' {
				streamDepth++
			} else if ch == '}' {
				streamDepth--
			}
		}
		result = append(result, line)
		i++
	}

	return strings.Join(result, "\n")
}
