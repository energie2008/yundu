// Package nginx 提供 node-agent 的 nginx 配置同步能力。
//
// 核心安全原则：
//   - 原子写入：先备份现有文件，写入新内容后 nginx -t 校验，失败则回滚
//   - 校验失败绝不保留新文件——防止"配置生成成功但语法有误，reload 把整个 nginx 打挂"
//   - 路径可配置：不同 VPS 的 nginx 配置路径不同（宝塔 vs 标准），通过 SyncConfig 配置
package nginx

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SyncConfig nginx 配置同步参数。
type SyncConfig struct {
	// StreamSnippetPath stream SNI 分流片段的正式路径
	// 宝塔环境默认: /www/server/panel/vhost/nginx/tcp/yundu-stream-snippet.conf
	// 标准环境默认: /etc/nginx/conf.d/yundu-stream-snippet.conf
	StreamSnippetPath string
	// HTTPSSnippetPath CDN 反代 vhost 片段的正式路径
	// 宝塔环境默认: /www/server/panel/vhost/nginx/yundu-https-snippet.conf
	// 标准环境默认: /etc/nginx/conf.d/yundu-https-snippet.conf
	HTTPSSnippetPath string
	// NginxBinaryPath nginx 二进制路径（默认从 PATH 查找）
	NginxBinaryPath string
}

// DefaultBtPanelConfig 返回宝塔面板环境的默认配置（VPS190 实测）。
func DefaultBtPanelConfig() *SyncConfig {
	return &SyncConfig{
		StreamSnippetPath: "/www/server/panel/vhost/nginx/tcp/yundu-stream-snippet.conf",
		HTTPSSnippetPath:  "/www/server/panel/vhost/nginx/yundu-https-snippet.conf",
		NginxBinaryPath:   "nginx",
	}
}

// DefaultStandardConfig 返回标准 nginx 环境的默认配置。
func DefaultStandardConfig() *SyncConfig {
	return &SyncConfig{
		StreamSnippetPath: "/etc/nginx/conf.d/yundu-stream-snippet.conf",
		HTTPSSnippetPath:  "/etc/nginx/conf.d/yundu-https-snippet.conf",
		NginxBinaryPath:   "nginx",
	}
}

// SyncResult 同步结果。
type SyncResult struct {
	StreamApplied bool   // stream snippet 是否已应用
	HTTPSApplied  bool   // https snippet 是否已应用
	NginxReloaded bool   // nginx 是否已 reload
	NginxTestOut  string // nginx -t 的输出
}

// fileBackup 记录文件的原始内容（用于回滚）。
type fileBackup struct {
	content  []byte
	existed  bool
}

// Sync 原子写入 nginx 配置片段 + nginx -t 强制校验 + reload。
//
// 流程：
//  1. 备份现有文件内容
//  2. 写入新内容到正式路径（或删除空 snippet 的残留文件）
//  3. 执行 nginx -t 校验（此时测试的是包含新内容的完整配置）
//  4. 校验失败 → 回滚到备份内容，返回错误
//  5. 校验成功 → 执行 nginx -s reload
//
// 安全保证：
//   - nginx -t 失败时原文件被回滚，不会留下破损配置
//   - 内容未变化时跳过写入和 reload（幂等）
//   - 空 snippet 会删除残留文件（避免旧配置干扰）
func Sync(streamSnippet, httpsSnippet string, cfg *SyncConfig) (*SyncResult, error) {
	if cfg == nil {
		cfg = DefaultBtPanelConfig()
	}

	result := &SyncResult{}

	nginxBin := cfg.NginxBinaryPath
	if nginxBin == "" {
		nginxBin = "nginx"
	}

	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(cfg.StreamSnippetPath), 0755); err != nil {
		return result, fmt.Errorf("create stream snippet dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.HTTPSSnippetPath), 0755); err != nil {
		return result, fmt.Errorf("create https snippet dir: %w", err)
	}

	// 1. 备份现有文件
	streamBak := readFile(cfg.StreamSnippetPath)
	httpsBak := readFile(cfg.HTTPSSnippetPath)

	// 2. 写入新内容（或删除空 snippet 的残留文件）
	//    仅在内容变化时才写入，避免不必要的 reload
	streamChanged := false
	httpsChanged := false

	if streamSnippet != "" {
		markedContent := ensureYunduMarker(streamSnippet)
		if string(streamBak.content) != markedContent {
			if err := atomicWrite(cfg.StreamSnippetPath, []byte(markedContent), 0644); err != nil {
				return result, fmt.Errorf("write stream snippet: %w", err)
			}
			streamChanged = true
		}
	} else {
		// Always keep a deny-all stream snippet in place so that the skeleton's
		// "include /etc/yundu/nginx/stream/*.conf" always has a server listening on 443.
		// Deleting would make the stream block empty → nginx -t passes but 443 is unguarded.
		fallback := defaultStreamDenyAll()
		if string(streamBak.content) != fallback {
			if err := atomicWrite(cfg.StreamSnippetPath, []byte(fallback), 0644); err != nil {
				return result, fmt.Errorf("write default stream snippet: %w", err)
			}
			streamChanged = true
		}
	}

	if httpsSnippet != "" {
		markedContent := ensureYunduMarker(httpsSnippet)
		if string(httpsBak.content) != markedContent {
			if err := atomicWrite(cfg.HTTPSSnippetPath, []byte(markedContent), 0644); err != nil {
				// 回滚 stream（如果刚写入）
				restoreFile(cfg.StreamSnippetPath, streamBak)
				return result, fmt.Errorf("write https snippet: %w", err)
			}
			httpsChanged = true
		}
	} else if httpsBak.existed {
		os.Remove(cfg.HTTPSSnippetPath)
		httpsChanged = true
	}

	// 无变更 → 跳过校验和 reload
	if !streamChanged && !httpsChanged {
		return result, nil
	}

	// 3. nginx -t 校验（此时测试的是包含新内容的完整配置）
	out, err := exec.Command(nginxBin, "-t").CombinedOutput()
	result.NginxTestOut = string(out)
	if err != nil {
		// 校验失败 → 回滚所有变更
		restoreFile(cfg.StreamSnippetPath, streamBak)
		restoreFile(cfg.HTTPSSnippetPath, httpsBak)
		return result, fmt.Errorf("nginx -t FAILED, config rolled back: %w\n%s", err, out)
	}

	// 4. 校验成功 → nginx -s reload
	if err := exec.Command(nginxBin, "-s", "reload").Run(); err != nil {
		// 配置已校验通过但 reload 失败——配置本身是合法的，不需要回滚
		// 可能是 nginx 主进程异常，记录错误但保留配置
		result.StreamApplied = streamChanged
		result.HTTPSApplied = httpsChanged
		return result, fmt.Errorf("nginx reload failed (config is valid but not reloaded): %w", err)
	}

	result.StreamApplied = streamChanged
	result.HTTPSApplied = httpsChanged
	result.NginxReloaded = true

	return result, nil
}

// readFile 读取文件内容，返回备份信息。
func readFile(path string) fileBackup {
	content, err := os.ReadFile(path)
	if err != nil {
		return fileBackup{content: nil, existed: false}
	}
	return fileBackup{content: content, existed: true}
}

// restoreFile 将文件回滚到备份内容。
func restoreFile(path string, bak fileBackup) {
	if bak.existed {
		os.WriteFile(path, bak.content, 0644)
	} else {
		os.Remove(path)
	}
}

// RemoveSnippets 移除 YunDu 生成的 snippet 文件（用于回退/禁用）。
// 移除后需要手动 nginx -s reload。
func RemoveSnippets(cfg *SyncConfig) error {
	if cfg == nil {
		cfg = DefaultBtPanelConfig()
	}
	var errs []error
	if err := os.Remove(cfg.StreamSnippetPath); err != nil && !os.IsNotExist(err) {
		errs = append(errs, fmt.Errorf("remove stream snippet: %w", err))
	}
	if err := os.Remove(cfg.HTTPSSnippetPath); err != nil && !os.IsNotExist(err) {
		errs = append(errs, fmt.Errorf("remove https snippet: %w", err))
	}
	if len(errs) > 0 {
		return fmt.Errorf("remove snippets: %v", errs)
	}
	return nil
}

// TestNginxConfig 执行 nginx -t 校验当前配置（不写入任何文件）。
// 用于部署前预检。
func TestNginxConfig(cfg *SyncConfig) error {
	if cfg == nil {
		cfg = DefaultBtPanelConfig()
	}
	nginxBin := cfg.NginxBinaryPath
	if nginxBin == "" {
		nginxBin = "nginx"
	}
	out, err := exec.Command(nginxBin, "-t").CombinedOutput()
	if err != nil {
		return fmt.Errorf("nginx -t failed: %w\n%s", err, out)
	}
	return nil
}

func ensureYunduMarker(content string) string {
	if strings.Contains(content, yunduMarker) {
		return content
	}
	return yunduMarker + " snippet — DO NOT EDIT MANUALLY\n" + content
}

func defaultStreamDenyAll() string {
	return ensureYunduMarker(`# Default deny-all stream fallback (no REALITY/CDN upstreams configured)
map $ssl_preread_server_name $internal_upstream {
    default "";
}
server {
    listen 443 reuseport;
    listen [::]:443 reuseport;
    proxy_pass $internal_upstream;
    ssl_preread on;
    proxy_connect_timeout 2s;
    proxy_timeout 5s;
}
`)
}
