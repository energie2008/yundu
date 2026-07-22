package cert

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// ECHKeyPair 包含 xray tls ech 生成的密钥对
//
// - ConfigPEM: 公开配置（分发到客户端 / 出现在订阅 URL 的 ech= 参数）
// - KeyPEM:    服务端私钥（仅在服务端配置中使用）
type ECHKeyPair struct {
	ConfigPEM string
	KeyPEM    string
}

// echConfigBlockRe 匹配 -----BEGIN ECH CONFIGS----- ... -----END ECH CONFIGS-----
// xray 26.x 使用 CONFIGS/KEYS（复数），与 draft-ietf-tls-esni 一致
var echConfigBlockRe = regexp.MustCompile(`(?s)-----BEGIN ECH CONFIGS-----.*?-----END ECH CONFIGS-----`)

// echKeyBlockRe 匹配 -----BEGIN ECH KEYS----- ... -----END ECH KEYS-----
var echKeyBlockRe = regexp.MustCompile(`(?s)-----BEGIN ECH KEYS-----.*?-----END ECH KEYS-----`)

// ErrECHBinaryNotFound 表示找不到 xray 二进制
var ErrECHBinaryNotFound = errors.New("xray binary not found in PATH; install xray 1.8.24+ to generate ECH key pair")

// ECHGenerator 抽象 ECH 密钥对生成（便于测试 mock）
type ECHGenerator interface {
	Generate(ctx context.Context) (*ECHKeyPair, error)
}

// XrayECHGenerator 调用本地 xray 二进制生成 ECH 密钥对
type XrayECHGenerator struct {
	BinaryPath string
	Timeout    time.Duration
}

// NewXrayECHGenerator 创建默认的 xray ECH 生成器
//
// binaryPath 为空时按 PATH 查找 xray；超时默认 10s
func NewXrayECHGenerator(binaryPath string) *XrayECHGenerator {
	if binaryPath == "" {
		binaryPath = "xray"
	}
	return &XrayECHGenerator{
		BinaryPath: binaryPath,
		Timeout:    10 * time.Second,
	}
}

// Generate 调用 `xray tls ech --pem` 并解析输出
//
// xray 26.x 支持该命令，输出格式（PEM 格式）：
//
//	-----BEGIN ECH CONFIGS-----
//	...
//	-----END ECH CONFIGS-----
//	-----BEGIN ECH KEYS-----
//	...
//	-----END ECH KEYS-----
//
// 同时支持 serverName 参数：xray tls ech --pem --serverName example.com
func (g *XrayECHGenerator) Generate(ctx context.Context) (*ECHKeyPair, error) {
	// 检查 xray 是否存在
	if _, err := exec.LookPath(g.BinaryPath); err != nil {
		return nil, ErrECHBinaryNotFound
	}

	cctx, cancel := context.WithTimeout(ctx, g.Timeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, g.BinaryPath, "tls", "ech", "--pem")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("xray tls ech --pem failed: %w; stderr=%s", err, stderr.String())
	}

	output := stdout.String()
	if output == "" {
		return nil, fmt.Errorf("xray tls ech --pem returned empty output; stderr=%s", stderr.String())
	}

	configPEM := extractFirstMatch(echConfigBlockRe, output)
	keyPEM := extractFirstMatch(echKeyBlockRe, output)

	if configPEM == "" {
		return nil, errors.New("ECH CONFIGS PEM block not found in xray output")
	}
	if keyPEM == "" {
		return nil, errors.New("ECH KEYS PEM block not found in xray output")
	}

	return &ECHKeyPair{
		ConfigPEM: strings.TrimSpace(configPEM),
		KeyPEM:    strings.TrimSpace(keyPEM),
	}, nil
}

// extractFirstMatch 提取正则第一个匹配（含换行）
func extractFirstMatch(re *regexp.Regexp, s string) string {
	m := re.FindString(s)
	return m
}
