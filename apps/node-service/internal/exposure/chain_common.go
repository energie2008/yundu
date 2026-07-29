package exposure

import (
	"github.com/airport-panel/subscription/chain"
	"github.com/airport-panel/subscription/nodespec"
)

// P1-1: chain 渲染已迁移到 packages/subscription/chain 包。
// 此文件保留薄包装（thin wrapper）向后兼容，实际逻辑在 chain 包中。
// 后续可删除此文件，所有调用方直接使用 chain.RenderChainForKernel()。

// ChainOutboundFields 双内核共用的中间表示（IR）。
// P1-1: 已迁移到 chain.ChainOutboundFields，此为别名。
type ChainOutboundFields = chain.ChainOutboundFields

// ExtractChainOutboundFields 从 NodeSpec 提取套娃 outbound 通用字段。
// P1-1: 已迁移到 chain.ExtractChainOutboundFields，此为代理。
func ExtractChainOutboundFields(ns *nodespec.NodeSpec) (*chain.ChainOutboundFields, error) {
	return chain.ExtractChainOutboundFields(ns)
}

// normalizeChainProtocol 已迁移到 chain 包（私有函数），保留此函数向后兼容。
func normalizeChainProtocol(p string) string {
	switch p {
	case "socks5", "socks5h":
		return "socks"
	case "shadowsocks":
		return "ss"
	default:
		return p
	}
}
