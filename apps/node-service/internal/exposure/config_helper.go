package exposure

import (
	"context"
	"strings"

	"github.com/airport-panel/node-service/internal/repo"
	"github.com/google/uuid"
)

func getStringFromConfig(m map[string]interface{}, key string) (string, bool) {
	if m == nil {
		return "", false
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s, true
		}
	}
	return "", false
}

// pickStringNested 从 config_json 中按"顶层 → 多个嵌套对象"的顺序读取字符串字段。
// 借鉴 xboard ServerService::buildNodeConfig 的字段路径标准：
//   - 顶层: cfg[topLevelKey]
//   - 嵌套: cfg[nestedKey][topLevelKey]（按 nestedKeys 顺序回退）
//
// 用于解决前端双写（reality.* + reality_settings.*）与后端读取路径不一致的问题。
// 返回第一个非空字符串值；全部未命中返回空字符串。
func pickStringNested(cfg map[string]interface{}, topLevelKey string, nestedKeys ...string) string {
	if cfg == nil {
		return ""
	}
	// 1. 顶层
	if v, ok := cfg[topLevelKey].(string); ok && v != "" {
		return v
	}
	// 2. 多个嵌套路径（按优先级回退）
	for _, nk := range nestedKeys {
		if m, ok := cfg[nk].(map[string]interface{}); ok {
			if v, ok := m[topLevelKey].(string); ok && v != "" {
				return v
			}
		}
	}
	return ""
}

// pickBoolNested 从 config_json 中按"顶层 → 多个嵌套对象"的顺序读取布尔字段。
// 兼容 JSON 解析后的多种类型：bool、float64(0/1)、string("true"/"1"/"yes")。
// 用于 allow_insecure 等 xboard 历史可能以 0/1 数字或字符串存储的字段。
func pickBoolNested(cfg map[string]interface{}, topLevelKey string, nestedKeys ...string) bool {
	if cfg == nil {
		return false
	}
	if asBoolNested(cfg[topLevelKey]) {
		return true
	}
	for _, nk := range nestedKeys {
		if m, ok := cfg[nk].(map[string]interface{}); ok {
			if asBoolNested(m[topLevelKey]) {
				return true
			}
		}
	}
	return false
}

// asBoolNested 将任意值转为 bool，兼容 bool/float64/string。
func asBoolNested(v interface{}) bool {
	switch x := v.(type) {
	case bool:
		return x
	case float64:
		return x != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "true", "1", "yes", "on":
			return true
		}
	}
	return false
}

// UserCredentialFetcher 抽象 user_node_credentials 表的查询能力
// node-service 的 repo.UserNodeCredentialRepo 隐式实现此接口
type UserCredentialFetcher interface {
	GetByNodeID(ctx context.Context, nodeID uuid.UUID) ([]*repo.UserNodeCredential, error)
}

// NodeCredentials 是预取的 per-node 凭证映射：nodeID -> 用户凭证列表
// nil 或条目缺失表示该节点无 per-user 凭证，回退到原有单用户配置
type NodeCredentials map[uuid.UUID][]*repo.UserNodeCredential

// FetchNodeCredentials 批量预取所有节点的 per-user 凭证。
// fetcher 为 nil 时返回 nil（向后兼容，调用方使用原有配置）
func FetchNodeCredentials(ctx context.Context, fetcher UserCredentialFetcher, nodeIDs []uuid.UUID) (NodeCredentials, error) {
	if fetcher == nil {
		return nil, nil
	}
	result := make(NodeCredentials)
	for _, id := range nodeIDs {
		creds, err := fetcher.GetByNodeID(ctx, id)
		if err != nil {
			return nil, err
		}
		if len(creds) > 0 {
			result[id] = creds
		}
	}
	return result, nil
}
