// Package validator 实现双内核配置校验，包含真实 dry-run 和语义等价性校验。
//
// 设计对齐：yundu项目核心节点设计/YunDu 双核渲染器统一化 + 校验层完整实现.md
//
// 核心能力（XBoard 式架构没有的差异化能力）：
//  1. 分别用 Xray/Sing-box 渲染器生成配置
//  2. 调用真实二进制 dry-run（xray -test / sing-box check）做语法校验
//  3. 双核语义等价性校验（防止配置漂移）
//  4. Enhancement 专项校验（uTLS/ECH/Mux 适用性）
package validator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/airport-panel/subscription/kernelrender"
	"github.com/airport-panel/subscription/nodespec"
)

// ValidationErrorLevel 校验错误级别
type ValidationErrorLevel string

const (
	LevelError   ValidationErrorLevel = "error"
	LevelWarning ValidationErrorLevel = "warning"
	LevelInfo    ValidationErrorLevel = "info"
)

// ValidationError 单条校验错误
type ValidationError struct {
	Field   string               `json:"field,omitempty"`
	Kernel  string               `json:"kernel,omitempty"`
	Level   ValidationErrorLevel `json:"level"`
	Message string               `json:"message"`
}

// ValidationResult 校验结果
type ValidationResult struct {
	Passed        bool                   `json:"passed"`
	Errors        []ValidationError      `json:"errors"`
	XrayConfig    map[string]interface{} `json:"xray_config,omitempty"`
	SingBoxConfig map[string]interface{} `json:"sing_box_config,omitempty"`
}

// DualKernelValidator 双内核校验器
type DualKernelValidator struct {
	xrayBinaryPath    string
	singboxBinaryPath string
	dryRunTimeout     time.Duration
	tempDir           string
	skipDryRun        bool // 跳过真实 dry-run（用于无二进制的开发环境）
}

// NewDualKernelValidator 构造校验器
// xrayBin/singboxBin 为空时自动跳过对应 dry-run
func NewDualKernelValidator(xrayBin, singboxBin string) *DualKernelValidator {
	return &DualKernelValidator{
		xrayBinaryPath:    xrayBin,
		singboxBinaryPath: singboxBin,
		dryRunTimeout:     10 * time.Second,
		tempDir:           os.TempDir(),
		skipDryRun:        xrayBin == "" && singboxBin == "",
	}
}

// WithDryRunTimeout 设置 dry-run 超时
func (v *DualKernelValidator) WithDryRunTimeout(d time.Duration) *DualKernelValidator {
	v.dryRunTimeout = d
	return v
}

// WithSkipDryRun 设置跳过真实 dry-run（仅做语义校验）
func (v *DualKernelValidator) WithSkipDryRun(skip bool) *DualKernelValidator {
	v.skipDryRun = skip
	return v
}

// ValidateBoth 是校验主入口：语义校验 → 双核生成 → 语法dry-run → 语义等价性
func (v *DualKernelValidator) ValidateBoth(ctx context.Context, spec *nodespec.NodeSpec) *ValidationResult {
	result := &ValidationResult{Errors: []ValidationError{}}

	if spec == nil {
		result.Errors = append(result.Errors, ValidationError{
			Level:   LevelError,
			Message: "nodespec is nil",
		})
		return result
	}

	// 第0步：前置校验 — CDN/Tunnel/SaaS 节点必须设置 ServerPort
	// 使用 WS/gRPC/xHTTP/HTTPUpgrade + TLS/REALITY 传输的节点需要 nginx 反向代理，
	// 若 Port=443 且 ServerPort=0，xray 会与 nginx 端口冲突，节点无法启动。
	portErrs := validateServerPort(spec)
	result.Errors = append(result.Errors, portErrs...)

	// 第1步：Enhancement 专项校验
	enhErrs := RunEnhancementValidators(spec, nil)
	result.Errors = append(result.Errors, enhErrs...)

	// 第2步：分别渲染（复用统一 Registry，任何一侧渲染失败不阻断另一侧）
	var xrayCfg, sbCfg map[string]interface{}
	var xrayErr, sbErr error

	xrayCfg, xrayErr = kernelrender.RenderForKernel(kernelrender.KernelXray, spec)
	if xrayErr != nil {
		if warn, isWarn := xrayErr.(*kernelrender.UnsupportedFeatureWarning); isWarn {
			result.Errors = append(result.Errors, ValidationError{
				Level: LevelWarning, Kernel: "xray", Message: warn.Error(),
			})
		} else if e, isErr := xrayErr.(*kernelrender.UnsupportedFeatureError); isErr {
			result.Errors = append(result.Errors, ValidationError{
				Level: LevelInfo, Kernel: "xray",
				Message: fmt.Sprintf("此协议不生成Xray配置（内核不支持: %s）", e.Feature),
			})
		} else {
			result.Errors = append(result.Errors, ValidationError{
				Level: LevelError, Kernel: "xray", Message: xrayErr.Error(),
			})
		}
	}

	sbCfg, sbErr = kernelrender.RenderForKernel(kernelrender.KernelSingBox, spec)
	if sbErr != nil {
		if e, isErr := sbErr.(*kernelrender.UnsupportedFeatureError); isErr {
			result.Errors = append(result.Errors, ValidationError{
				Level: LevelInfo, Kernel: "sing_box",
				Message: fmt.Sprintf("此协议不生成Sing-box配置（内核不支持: %s）", e.Feature),
			})
		} else {
			result.Errors = append(result.Errors, ValidationError{
				Level: LevelError, Kernel: "sing_box", Message: sbErr.Error(),
			})
		}
	}
	result.XrayConfig = xrayCfg
	result.SingBoxConfig = sbCfg

	// 第3步：真实二进制 dry-run 语法校验
	if !v.skipDryRun {
		if xrayCfg != nil && v.xrayBinaryPath != "" {
			if err := v.runXrayDryRun(ctx, xrayCfg); err != nil {
				result.Errors = append(result.Errors, ValidationError{
					Level: LevelError, Kernel: "xray", Message: err.Error(),
				})
			}
		}
		if sbCfg != nil && v.singboxBinaryPath != "" {
			if err := v.runSingBoxDryRun(ctx, sbCfg); err != nil {
				result.Errors = append(result.Errors, ValidationError{
					Level: LevelError, Kernel: "sing_box", Message: err.Error(),
				})
			}
		}
	}

	// 第4步：双核语义等价性校验（防止配置漂移，这是 XBoard 式架构完全没有的能力）
	if xrayCfg != nil && sbCfg != nil {
		result.Errors = append(result.Errors, v.validateSemanticEquivalence(spec, xrayCfg, sbCfg)...)
	}

	result.Passed = !hasErrorLevel(result.Errors)
	return result
}

// runXrayDryRun 真实调用 xray -test 校验语法
func (v *DualKernelValidator) runXrayDryRun(ctx context.Context, cfg map[string]interface{}) error {
	tmpFile, cleanup, err := v.writeTempConfig("xray", cfg)
	if err != nil {
		return fmt.Errorf("写入临时配置失败: %w", err)
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(ctx, v.dryRunTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, v.xrayBinaryPath, "run", "-test", "-config", tmpFile)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("xray -test 超时(>%s)，可能配置存在死循环或二进制异常", v.dryRunTimeout)
	}
	if err != nil {
		return fmt.Errorf("xray -test 失败: %s", truncate(string(out), 500))
	}
	return nil
}

// runSingBoxDryRun 真实调用 sing-box check 校验语法
func (v *DualKernelValidator) runSingBoxDryRun(ctx context.Context, cfg map[string]interface{}) error {
	tmpFile, cleanup, err := v.writeTempConfig("singbox", cfg)
	if err != nil {
		return fmt.Errorf("写入临时配置失败: %w", err)
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(ctx, v.dryRunTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, v.singboxBinaryPath, "check", "-c", tmpFile)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("sing-box check 超时(>%s)", v.dryRunTimeout)
	}
	if err != nil {
		return fmt.Errorf("sing-box check 失败: %s", truncate(string(out), 500))
	}
	return nil
}

// writeTempConfig 将配置写入临时文件
func (v *DualKernelValidator) writeTempConfig(prefix string, cfg map[string]interface{}) (path string, cleanup func(), err error) {
	f, err := os.CreateTemp(v.tempDir, fmt.Sprintf("yundu-%s-*.json", prefix))
	if err != nil {
		return "", nil, err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	if err := enc.Encode(cfg); err != nil {
		os.Remove(f.Name())
		return "", nil, err
	}
	fullPath := f.Name()
	return fullPath, func() { os.Remove(fullPath) }, nil
}

// validateSemanticEquivalence 核心创新点：确保双核配置描述的是同一件事
// 防止配置漂移（比如 Xray 端口是 443，Sing-box 写成 444）
func (v *DualKernelValidator) validateSemanticEquivalence(spec *nodespec.NodeSpec,
	xrayCfg, sbCfg map[string]interface{}) []ValidationError {

	var errs []ValidationError

	// 1. 端口一致性
	xrayPort := extractInboundField(xrayCfg, "port")
	sbPort := extractInboundField(sbCfg, "listen_port")
	if fmt.Sprint(xrayPort) != fmt.Sprint(sbPort) {
		errs = append(errs, ValidationError{
			Level:   LevelError,
			Message: fmt.Sprintf("端口不一致: xray=%v sing-box=%v", xrayPort, sbPort),
		})
	}

	// 2. 用户数量一致性
	xrayUsers := extractUserCount(xrayCfg, "settings", "clients")
	sbUsers := extractUserCount(sbCfg, "users")
	if xrayUsers >= 0 && sbUsers >= 0 && xrayUsers != sbUsers {
		errs = append(errs, ValidationError{
			Level:   LevelError,
			Message: fmt.Sprintf("用户数量双核不一致: xray=%d sing-box=%d，可能存在配置漂移", xrayUsers, sbUsers),
		})
	}

	// 3. REALITY 私钥一致性
	if spec.Security == nodespec.SecurityReality {
		xrayPK := extractNestedString(xrayCfg, "streamSettings", "realitySettings", "privateKey")
		sbPK := extractNestedString(sbCfg, "tls", "reality", "private_key")
		if xrayPK != "" && sbPK != "" && xrayPK != sbPK {
			errs = append(errs, ValidationError{
				Level:   LevelError,
				Message: "REALITY私钥双核不一致",
			})
		}
	}

	// 4. ECH 场景：Xray 侧预期没有该字段，这不算漂移，属于预期缺失，跳过比对但记录 info
	if spec.TLS != nil && spec.TLS.ECH != nil && spec.TLS.ECH.Enabled {
		errs = append(errs, ValidationError{
			Level:   LevelInfo,
			Message: "ECH仅在Sing-box配置中生效，双核一致性校验对此字段跳过比对",
		})
	}

	// 5. 协议类型一致性
	xrayProto := extractInboundField(xrayCfg, "protocol")
	sbType := extractInboundField(sbCfg, "type")
	if fmt.Sprint(xrayProto) != fmt.Sprint(sbType) {
		errs = append(errs, ValidationError{
			Level:   LevelWarning,
			Message: fmt.Sprintf("协议字段名双核不同: xray=%v sing-box=%v（这是预期差异，仅记录）", xrayProto, sbType),
		})
	}

	return errs
}

// hasErrorLevel 判断是否有 error 级别的错误
func hasErrorLevel(errs []ValidationError) bool {
	for _, e := range errs {
		if e.Level == LevelError {
			return true
		}
	}
	return false
}

// truncate 截断字符串
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}

// ===== 提取辅助函数：从 inbounds[0] 取字段，双核结构统一按此路径读取 =====

func extractInboundField(cfg map[string]interface{}, key string) interface{} {
	inbounds, ok := cfg["inbounds"].([]interface{})
	if !ok || len(inbounds) == 0 {
		return nil
	}
	ib, ok := inbounds[0].(map[string]interface{})
	if !ok {
		return nil
	}
	return ib[key]
}

func extractUserCount(cfg map[string]interface{}, path ...string) int {
	inbounds, ok := cfg["inbounds"].([]interface{})
	if !ok || len(inbounds) == 0 {
		return -1
	}
	cur, ok := inbounds[0].(map[string]interface{})
	if !ok {
		return -1
	}
	for i, p := range path {
		if i == len(path)-1 {
			if arr, ok := cur[p].([]map[string]interface{}); ok {
				return len(arr)
			}
			if arr, ok := cur[p].([]interface{}); ok {
				return len(arr)
			}
			return -1
		}
		next, ok := cur[p].(map[string]interface{})
		if !ok {
			return -1
		}
		cur = next
	}
	return -1
}

func extractNestedString(cfg map[string]interface{}, path ...string) string {
	inbounds, ok := cfg["inbounds"].([]interface{})
	if !ok || len(inbounds) == 0 {
		return ""
	}
	cur, ok := inbounds[0].(map[string]interface{})
	if !ok {
		return ""
	}
	for i, p := range path {
		if i == len(path)-1 {
			if s, ok := cur[p].(string); ok {
				return s
			}
			return ""
		}
		next, ok := cur[p].(map[string]interface{})
		if !ok {
			return ""
		}
		cur = next
	}
	return ""
}

// validateServerPort 校验 CDN/Tunnel/SaaS 节点的 ServerPort 配置。
//
// 使用 WS/gRPC/xHTTP/HTTPUpgrade + TLS/REALITY 传输的节点需要 nginx 反向代理
// （nginx 占用 443 端口终止 TLS，再将流量转发到 xray 的 ServerPort）。
// 若 Port=443 且 ServerPort=0，xray 会尝试监听 0.0.0.0:443，与 nginx 端口冲突，
// 导致节点无法启动。此校验在渲染前拦截此类错误配置。
//
// 直接 TCP+REALITY 节点（ServerPort=0, Port=443）不触发此校验，
// 因为 REALITY 自行处理 TLS，不需要 nginx。
// UDP 直连节点（Hysteria2/TUIC）同样不触发。
func validateServerPort(spec *nodespec.NodeSpec) []ValidationError {
	if spec.ServerPort > 0 {
		return nil // ServerPort 已设置，无问题
	}

	// 判断是否为需要 nginx 反向代理的传输类型
	needsNginx := false
	switch spec.Transport.Type {
	case nodespec.TransportWS, nodespec.TransportGRPC,
		nodespec.TransportXHTTP, nodespec.TransportHTTPUpgrade:
		if spec.Security == nodespec.SecurityTLS || spec.Security == nodespec.SecurityReality {
			needsNginx = true
		}
	}

	if !needsNginx {
		return nil
	}

	// Port=443 且 ServerPort=0 → xray 会与 nginx 端口冲突
	if spec.Port == 443 {
		return []ValidationError{{
			Level: LevelError,
			Field: "server_port",
			Message: fmt.Sprintf(
				"节点使用 %s+%s 传输且 Port=443，必须设置 server_port（xray 本地监听端口），"+
					"否则 xray 与 nginx 端口冲突。CDN/Tunnel/SaaS 节点请设置 server_port 为高位端口（如 10003）",
				spec.Security, spec.Transport.Type,
			),
		}}
	}

	return nil
}
