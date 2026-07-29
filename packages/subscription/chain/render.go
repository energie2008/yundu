package chain

import (
	"fmt"

	"github.com/airport-panel/subscription/nodespec"
)

// KernelType 标识目标内核类型
type KernelType string

const (
	KernelXray    KernelType = "xray"
	KernelSingBox KernelType = "sing-box"
)

// ChainRenderResult 链式代理渲染结果
type ChainRenderResult struct {
	Outbounds []map[string]interface{} `json:"outbounds"`
	Routing   map[string]interface{}   `json:"routing,omitempty"`
}

// RenderChainForKernel P1-1: 链式代理统一渲染入口。
// 根据 kernel 类型分发到 xray 或 sing-box 渲染器，消除双渲染器分叉风险。
// 此函数是 chain 渲染的唯一入口，替代直接调用 exposure.BuildXrayChainOutbounds/BuildSingboxChainOutbounds。
func RenderChainForKernel(kernel KernelType, c *ChainSpec) (*ChainRenderResult, error) {
	if c == nil {
		return nil, fmt.Errorf("nil chain spec")
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}

	switch kernel {
	case KernelXray:
		return renderXrayChain(c)
	case KernelSingBox:
		return renderSingboxChain(c)
	default:
		return nil, fmt.Errorf("unsupported kernel type: %s", kernel)
	}
}

// RenderOutboundFromNodeSpec P1-1: 从 NodeSpec 构建单跳 outbound（套娃出站构建器）。
// 统一入口，根据 kernel 类型分发到 xray 或 sing-box 渲染器。
func RenderOutboundFromNodeSpec(kernel KernelType, ns *nodespec.NodeSpec, tag, detourTag string) (map[string]interface{}, error) {
	switch kernel {
	case KernelXray:
		return renderXrayOutboundFromNodeSpec(ns, tag, detourTag)
	case KernelSingBox:
		return renderSingboxOutboundFromNodeSpec(ns, tag, detourTag)
	default:
		return nil, fmt.Errorf("unsupported kernel type: %s", kernel)
	}
}
