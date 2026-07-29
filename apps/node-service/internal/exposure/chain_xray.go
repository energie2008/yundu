package exposure

import (
	"github.com/airport-panel/subscription/chain"
	"github.com/airport-panel/subscription/nodespec"
)

// P1-1: chain xray 渲染已迁移到 packages/subscription/chain/render_xray.go。
// 此文件保留薄包装（thin wrapper）向后兼容。

// BuildXrayChainOutbounds 构建订阅侧多跳链式代理的完整 xray outbounds + routing。
func BuildXrayChainOutbounds(c *chain.ChainSpec) ([]map[string]interface{}, map[string]interface{}, error) {
	result, err := chain.RenderChainForKernel(chain.KernelXray, c)
	if err != nil {
		return nil, nil, err
	}
	return result.Outbounds, result.Routing, nil
}

// BuildXrayOutboundFromNodeSpec 从 NodeSpec 构建 xray outbound。
func BuildXrayOutboundFromNodeSpec(ns *nodespec.NodeSpec, tag, proxyTag string) (map[string]interface{}, error) {
	return chain.RenderOutboundFromNodeSpec(chain.KernelXray, ns, tag, proxyTag)
}

// DefaultXrayRouting 返回 xray 默认 routing 模板。
func DefaultXrayRouting() map[string]interface{} {
	return chain.DefaultXrayRouting()
}
