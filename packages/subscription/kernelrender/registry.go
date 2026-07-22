package kernelrender

import "github.com/airport-panel/subscription/nodespec"

// DownstreamTagSuffix 是上下行分离节点（XHTTP split mode）自动衍生的下行 inbound
// tag 的固定后缀。生成下行 inbound tag 的代码（xray.go/singbox.go）和消费端
// （node-service 的 isDownstreamInbound）必须调用此常量，禁止两处各写一遍字面量。
//
// 重要约束（退役计划）：
// P0 阶段 node-service 基于此常量做 tag 后缀判定是【临时兜底】。
// P1 上线 downstream_exposure_mode 字段后，tag 后缀仅作为下行 inbound 的展示性命名，
// 不再参与安全逻辑判定。
const DownstreamTagSuffix = "-dl"

// init 注册默认渲染器到全局 Registry
func init() {
	Register(NewXrayRenderer())
	Register(NewSingBoxRenderer())
}

// PresetKernelCompat 返回预设模板声明的内核兼容性
// 这是一个辅助函数，让上层 validator 可以查询预设声明的内核支持情况
func PresetKernelCompat(preset *nodespec.PresetTemplate) (xraySupported, singboxSupported bool) {
	if preset == nil {
		return true, true // 无预设时默认都支持
	}
	switch preset.KernelCompat {
	case nodespec.CompatBoth:
		return true, true
	case nodespec.CompatXrayOnly:
		return true, false
	case nodespec.CompatSingboxOnly:
		return false, true
	case nodespec.CompatExperimental:
		return true, true // 实验性，按支持处理
	default:
		return true, true
	}
}
