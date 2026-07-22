package service

// capability_degrade.go 实现 P2-2：Capability Degrade 三策略
// (Deny / Downgrade / ForceKernel)。
//
// 当节点协议+传输+安全+特性组合在目标内核上为 blocked 时：
//   - Deny:        直接拒绝发布（返回 422，与 P0 行为一致）
//   - Downgrade:   若 kernel_capabilities.downgrade_to 非空，则原地改写 NodeSpec
//                  （如 XHTTP → HTTPUpgrade），并记录 CapabilityLost 事件
//   - ForceKernel: 若另一内核 native 支持，则切换内核；否则回退到 Downgrade/Deny
//
// 降级事件写入 capability_lost_events 表（migration 000046），
// 并在 slog 中以 INFO 级别记录，便于运维排查。

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/airport-panel/node-service/internal/repo"
	"github.com/google/uuid"
	"github.com/airport-panel/subscription/nodespec"
)

// DegradeStrategy 能力降级策略（P2-2）
type DegradeStrategy string

const (
	// StrategyDeny 拒绝：blocked 直接返回错误（默认，与 P0 一致）
	StrategyDeny DegradeStrategy = "deny"
	// StrategyDowngrade 降级：应用 downgrade_to 映射，改写 NodeSpec
	StrategyDowngrade DegradeStrategy = "downgrade"
	// StrategyForceKernel 强切内核：blocked 时尝试切换到支持该组合的另一内核
	StrategyForceKernel DegradeStrategy = "force_kernel"
)

// CapabilityLostEvent 能力降级事件（P2-2 审计记录）
type CapabilityLostEvent struct {
	RuntimeID        uuid.UUID           `json:"runtime_id,omitempty"`
	NodeID           uuid.UUID           `json:"node_id,omitempty"`
	NodeCode         string              `json:"node_code"`
	Kernel           string              `json:"kernel"`
	Protocol         string              `json:"protocol"`
	Transport        string              `json:"transport"`
	Security         string              `json:"security"`
	Feature          string              `json:"feature"`
	OriginalSupport  string              `json:"original_support"` // blocked / degradable
	DegradeStrategy  DegradeStrategy     `json:"degrade_strategy"`
	DowngradeTo      map[string]interface{} `json:"downgrade_to,omitempty"`
	Message          string              `json:"message,omitempty"`
	ConfigVersionNo  string              `json:"config_version_no,omitempty"`
}

// degradeRepo 降级事件持久化接口（由 repo.CapabilityRepo 实现）
type degradeRepo interface {
	InsertCapabilityLostEvent(ctx context.Context, e *CapabilityLostEvent) error
}

// ApplyDegrade 在 NodeSpec 上应用能力降级策略（P2-2）。
//
// 参数：
//   - ctx: 上下文
//   - strategy: 降级策略（deny/downgrade/force_kernel）
//   - kernel: 目标内核（xray/sing-box）
//   - spec: 待降级的 NodeSpec（会被原地改写）
//   - capRepo: 能力矩阵仓库
//   - nodeID/nodeCode: 用于审计事件
//
// 返回：
//   - events: 实际发生的降级事件列表（可能为空）
//   - newKernel: 若策略为 force_kernel 且发生内核切换，返回新内核名；否则返回原 kernel
//   - err: 策略为 deny 时遇到 blocked 返回 ErrPreflightValidation
func ApplyDegrade(
	ctx context.Context,
	strategy DegradeStrategy,
	kernel string,
	spec *nodespec.NodeSpec,
	capRepo *repo.CapabilityRepo,
	nodeID uuid.UUID,
	nodeCode string,
) ([]CapabilityLostEvent, string, error) {
	if capRepo == nil || spec == nil {
		return nil, kernel, nil
	}

	protocol := string(spec.Protocol)
	transport := string(spec.Transport.Type)
	security := string(spec.Security)
	feature := "*"

	var events []CapabilityLostEvent
	currentKernel := kernel

	// 1. 基础组合校验
	cap, err := capRepo.CheckSupport(ctx, currentKernel, protocol, transport, security, feature)
	if err == nil && cap != nil && cap.SupportLevel == "blocked" {
		event, newKernel, derr := handleBlocked(ctx, strategy, currentKernel, spec, cap, capRepo, nodeID, nodeCode, protocol, transport, security, feature)
		if derr != nil {
			return events, currentKernel, derr
		}
		if event != nil {
			events = append(events, *event)
		}
		if newKernel != currentKernel {
			currentKernel = newKernel
		}
	}

	// 2. XHTTP mode 特性校验
	if spec.Transport.Type == nodespec.TransportXHTTP && spec.Transport.XHTTP != nil {
		mode := spec.Transport.XHTTP.Mode
		if mode != "" {
			modeCap, merr := capRepo.CheckSupport(ctx, currentKernel, protocol, transport, security, mode)
			if merr == nil && modeCap != nil && modeCap.SupportLevel == "blocked" {
				event, newKernel, derr := handleBlocked(ctx, strategy, currentKernel, spec, modeCap, capRepo, nodeID, nodeCode, protocol, transport, security, mode)
				if derr != nil {
					return events, currentKernel, derr
				}
				if event != nil {
					events = append(events, *event)
				}
				if newKernel != currentKernel {
					currentKernel = newKernel
				}
			}
		}
	}

	// 3. REALITY vision flow 特性校验
	if spec.Security == nodespec.SecurityReality && spec.Protocol == nodespec.ProtocolVLESS {
		flow := extractFlowLocal(spec)
		if flow == string(nodespec.FlowXTLSRprxVision) {
			visionCap, verr := capRepo.CheckSupport(ctx, currentKernel, protocol, transport, security, "vision")
			if verr == nil && visionCap != nil && visionCap.SupportLevel == "blocked" {
				event, newKernel, derr := handleBlocked(ctx, strategy, currentKernel, spec, visionCap, capRepo, nodeID, nodeCode, protocol, transport, security, "vision")
				if derr != nil {
					return events, currentKernel, derr
				}
				if event != nil {
					events = append(events, *event)
				}
				if newKernel != currentKernel {
					currentKernel = newKernel
				}
			}
		}
	}

	// 4. ECH 特性校验
	if isECHEnabledLocal(spec) {
		echCap, eerr := capRepo.CheckSupport(ctx, currentKernel, "*", transport, security, "ech")
		if eerr == nil && echCap != nil && echCap.SupportLevel == "blocked" {
			event, newKernel, derr := handleBlocked(ctx, strategy, currentKernel, spec, echCap, capRepo, nodeID, nodeCode, "*", transport, security, "ech")
			if derr != nil {
				return events, currentKernel, derr
			}
			if event != nil {
				events = append(events, *event)
			}
			if newKernel != currentKernel {
				currentKernel = newKernel
			}
		}
	}

	return events, currentKernel, nil
}

// handleBlocked 处理单个 blocked 能力条目，根据策略决定降级/拒绝/强切内核。
func handleBlocked(
	ctx context.Context,
	strategy DegradeStrategy,
	kernel string,
	spec *nodespec.NodeSpec,
	cap *repo.KernelCapability,
	capRepo *repo.CapabilityRepo,
	nodeID uuid.UUID,
	nodeCode string,
	protocol, transport, security, feature string,
) (*CapabilityLostEvent, string, error) {
	switch strategy {
	case StrategyDeny:
		msg := cap.Message
		if msg == "" {
			msg = fmt.Sprintf("%s 不支持 %s/%s/%s/%s", kernel, protocol, transport, security, feature)
		}
		return nil, kernel, fmt.Errorf("%w: %s", ErrPreflightValidation, msg)

	case StrategyDowngrade:
		if len(cap.DowngradeTo) == 0 {
			// 无降级目标，回退到 Deny
			msg := cap.Message
			if msg == "" {
				msg = fmt.Sprintf("%s 不支持 %s/%s/%s/%s 且无降级目标", kernel, protocol, transport, security, feature)
			}
			return nil, kernel, fmt.Errorf("%w: %s", ErrPreflightValidation, msg)
		}
		if err := applyDowngradeToSpec(spec, cap.DowngradeTo); err != nil {
			return nil, kernel, fmt.Errorf("%w: 降级失败: %v", ErrPreflightValidation, err)
		}
		ev := &CapabilityLostEvent{
			NodeID:          nodeID,
			NodeCode:        nodeCode,
			Kernel:          kernel,
			Protocol:        protocol,
			Transport:       transport,
			Security:        security,
			Feature:         feature,
			OriginalSupport: cap.SupportLevel,
			DegradeStrategy: strategy,
			DowngradeTo:     cap.DowngradeTo,
			Message:         cap.Message,
		}
		slog.Info("capability degraded (downgrade)",
			"node_code", nodeCode, "kernel", kernel,
			"protocol", protocol, "transport", transport, "feature", feature,
			"downgrade_to", cap.DowngradeTo)
		return ev, kernel, nil

	case StrategyForceKernel:
		// 尝试切换到另一内核
		otherKernel := "sing-box"
		if kernel == "sing-box" {
			otherKernel = "xray"
		}
		otherCap, oerr := capRepo.CheckSupport(ctx, otherKernel, protocol, transport, security, feature)
		if oerr == nil && otherCap != nil && otherCap.SupportLevel == "native" {
			// 另一内核原生支持，切换
			ev := &CapabilityLostEvent{
				NodeID:          nodeID,
				NodeCode:        nodeCode,
				Kernel:          kernel,
				Protocol:        protocol,
				Transport:       transport,
				Security:        security,
				Feature:         feature,
				OriginalSupport: cap.SupportLevel,
				DegradeStrategy: strategy,
				DowngradeTo:     map[string]interface{}{"kernel": otherKernel},
				Message:         fmt.Sprintf("强制切换内核 %s → %s", kernel, otherKernel),
			}
			slog.Info("capability degraded (force_kernel)",
				"node_code", nodeCode, "old_kernel", kernel, "new_kernel", otherKernel,
				"protocol", protocol, "transport", transport)
			return ev, otherKernel, nil
		}
		// 另一内核也不支持，回退到 Downgrade 或 Deny
		if len(cap.DowngradeTo) > 0 {
			if err := applyDowngradeToSpec(spec, cap.DowngradeTo); err != nil {
				return nil, kernel, fmt.Errorf("%w: 降级失败: %v", ErrPreflightValidation, err)
			}
			ev := &CapabilityLostEvent{
				NodeID:          nodeID,
				NodeCode:        nodeCode,
				Kernel:          kernel,
				Protocol:        protocol,
				Transport:       transport,
				Security:        security,
				Feature:         feature,
				OriginalSupport: cap.SupportLevel,
				DegradeStrategy: StrategyDowngrade,
				DowngradeTo:     cap.DowngradeTo,
				Message:         fmt.Sprintf("force_kernel 回退为 downgrade: %s", cap.Message),
			}
			return ev, kernel, nil
		}
		msg := cap.Message
		if msg == "" {
			msg = fmt.Sprintf("%s 和 %s 均不支持 %s/%s/%s/%s", kernel, otherKernel, protocol, transport, security, feature)
		}
		return nil, kernel, fmt.Errorf("%w: %s", ErrPreflightValidation, msg)
	}

	// 未知策略，保守拒绝
	return nil, kernel, fmt.Errorf("%w: 未知降级策略 %s", ErrPreflightValidation, strategy)
}

// applyDowngradeToSpec 根据 downgrade_to JSONB 映射原地改写 NodeSpec。
//
// 支持的映射格式（来自 migration 000046）：
//   - {"transport": "httpupgrade"}: 切换传输层，迁移 path/host 到新配置
//   - {"feature": "mux", "action": "drop"}: 去除 Mux 配置
//   - {"feature": "split", "action": "drop"}: 去除 XHTTP split（清空 DownloadSettings）
func applyDowngradeToSpec(spec *nodespec.NodeSpec, downgradeTo map[string]interface{}) error {
	if len(downgradeTo) == 0 {
		return nil
	}

	// 传输层降级（如 xhttp → httpupgrade）
	if targetTransport, ok := downgradeTo["transport"].(string); ok && targetTransport != "" {
		oldTransport := string(spec.Transport.Type)
		if oldTransport == targetTransport {
			return nil
		}
		switch nodespec.Transport(targetTransport) {
		case nodespec.TransportHTTPUpgrade:
			// 从 XHTTP 迁移到 HTTPUpgrade，保留 path/host
			var path, host string
			if spec.Transport.XHTTP != nil {
				path = spec.Transport.XHTTP.Path
				host = spec.Transport.XHTTP.Host
			}
			spec.Transport.Type = nodespec.TransportHTTPUpgrade
			spec.Transport.HTTPUpgrade = &nodespec.HTTPUpgradeConfig{
				Path:    path,
				Host:    host,
				Headers: spec.Transport.Headers,
			}
			spec.Transport.XHTTP = nil
		default:
			return fmt.Errorf("不支持的降级目标传输: %s", targetTransport)
		}
		return nil
	}

	// 特性去除
	if feature, ok := downgradeTo["feature"].(string); ok {
		action, _ := downgradeTo["action"].(string)
		if action != "drop" {
			return fmt.Errorf("不支持的降级 action: %s", action)
		}
		switch feature {
		case "mux":
			spec.Transport.Mux = nil
		case "split":
			if spec.Transport.XHTTP != nil {
				spec.Transport.XHTTP.DownloadSettings = nil
			}
		default:
			return fmt.Errorf("不支持的降级 feature: %s", feature)
		}
		return nil
	}

	return fmt.Errorf("无法识别的 downgrade_to 映射: %v", downgradeTo)
}

// MarshalDowngradeTo 辅助函数：将 downgrade_to map 序列化为 JSON（用于 DB 写入）
func MarshalDowngradeTo(m map[string]interface{}) []byte {
	if len(m) == 0 {
		return nil
	}
	b, _ := json.Marshal(m)
	return b
}
