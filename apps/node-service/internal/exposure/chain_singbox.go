package exposure

import (
	"github.com/airport-panel/subscription/chain"
	"github.com/airport-panel/subscription/nodespec"
)

// P1-1: chain sing-box 渲染已迁移到 packages/subscription/chain/render_singbox.go。
// 此文件保留薄包装（thin wrapper）向后兼容。

// BuildSingboxChainOutbounds 构建订阅侧多跳链式代理的 sing-box outbounds。
func BuildSingboxChainOutbounds(c *chain.ChainSpec) ([]map[string]interface{}, error) {
	result, err := chain.RenderChainForKernel(chain.KernelSingBox, c)
	if err != nil {
		return nil, err
	}
	return result.Outbounds, nil
}

// BuildSingboxChainRoute 构建订阅侧多跳链式代理的 sing-box route。
func BuildSingboxChainRoute(c *chain.ChainSpec) map[string]interface{} {
	result, err := chain.RenderChainForKernel(chain.KernelSingBox, c)
	if err != nil {
		return nil
	}
	return result.Routing
}

// BuildSingboxOutboundFromNodeSpec 从 NodeSpec 构建 sing-box outbound。
func BuildSingboxOutboundFromNodeSpec(ns *nodespec.NodeSpec, tag, detourTag string) (map[string]interface{}, error) {
	return chain.RenderOutboundFromNodeSpec(chain.KernelSingBox, ns, tag, detourTag)
}
