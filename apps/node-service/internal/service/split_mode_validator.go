package service

import (
	"fmt"

	"github.com/airport-panel/node-service/internal/model"
)

// validateSplitModeConsistency P1-4: 校验 is_split_mode 与 DownstreamExposureMode 的一致性。
//
// 约束（重要）：
//   - is_split_mode 是前端表单开关（DTO），控制"下行暴露方式"下拉框是否显示
//   - downstream_exposure_mode 是安全判定唯一状态源
//   - 两者必须保持一致：is_split_mode=true ↔ downstream_exposure_mode 有值
//
// 本函数检测不一致并返回错误描述（不直接修正，修正由调用点决定）。
// 调用点在节点保存时自动修正为以 downstream_exposure_mode 为准。
func validateSplitModeConsistency(node *model.Node) error {
	if node == nil {
		return nil
	}

	hasDownstreamMode := node.DownstreamExposureMode != nil && *node.DownstreamExposureMode != ""

	// 也检查 config_json 中的 downstream_exposure_mode（防止 DB 列与 JSON 不一致）
	if !hasDownstreamMode && node.ConfigJSON != nil {
		if v, ok := node.ConfigJSON["downstream_exposure_mode"].(string); ok && v != "" {
			hasDownstreamMode = true
		}
	}

	// 检查 config_json.is_split_mode 与 DB 列的一致性
	configSplitMode := false
	if node.ConfigJSON != nil {
		if v, ok := node.ConfigJSON["is_split_mode"].(bool); ok {
			configSplitMode = v
		}
	}

	// 场景1: is_split_mode=true 但 downstream_exposure_mode 为空 → 状态双轨
	if node.IsSplitMode && !hasDownstreamMode {
		return fmt.Errorf("is_split_mode=true but downstream_exposure_mode is empty; " +
			"downstream_exposure_mode is the sole security authority")
	}

	// 场景2: is_split_mode=false 但 downstream_exposure_mode 有值 → 状态双轨
	if !node.IsSplitMode && hasDownstreamMode {
		return fmt.Errorf("is_split_mode=false but downstream_exposure_mode has value %q; " +
			"is_split_mode should be true when downstream_exposure_mode is set",
			*node.DownstreamExposureMode)
	}

	// 场景3: DB 列 is_split_mode 与 config_json.is_split_mode 不一致
	if node.IsSplitMode != configSplitMode {
		return fmt.Errorf("DB is_split_mode=%v but config_json.is_split_mode=%v; " +
			"these must be kept in sync", node.IsSplitMode, configSplitMode)
	}

	return nil
}

// autoCorrectSplitModeConsistency P1-4: 自动修正 is_split_mode 一致性。
// 以 downstream_exposure_mode 为唯一状态源，强制同步 is_split_mode。
func autoCorrectSplitModeConsistency(node *model.Node) {
	if node == nil {
		return
	}

	hasDownstreamMode := node.DownstreamExposureMode != nil && *node.DownstreamExposureMode != ""

	// 以 downstream_exposure_mode 为准
	node.IsSplitMode = hasDownstreamMode
	if node.ConfigJSON == nil {
		node.ConfigJSON = make(map[string]interface{})
	}
	node.ConfigJSON["is_split_mode"] = hasDownstreamMode

	// 如果 downstream_exposure_mode 为空，清除 DB 列
	if !hasDownstreamMode {
		node.DownstreamExposureMode = nil
		delete(node.ConfigJSON, "downstream_exposure_mode")
	}
}
