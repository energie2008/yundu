// Package delta 提供 Delta Sync 增量同步的数据类型和处理逻辑。
//
// Delta Sync 是 YunDu 云原生代理平台四大支柱之一：
// 用户增删/套餐变更时，控制面不再推送全量 Manifest（80KB-1MB），
// 而是推送轻量级 Delta 消息（<1KB），Agent 收到后通过 RuntimePlugin.UpdateUsers
// 零断流热更，端到端延迟 <1s。
//
// 传输通道：
//   - WebSocket: JSON 序列化的 DeltaSync 消息
//   - HTTP: POST /api/v1/agent/delta 端点
//   - gRPC: 通过 ConfigPush 的 full_replace=false 标志触发增量（P2 兼容路径）
//
// 一致性保障：
//   - config_version 单调递增，Agent 检测版本跳跃时触发全量同步
//   - Delta 失败自动回退全量同步
//   - Agent 重启时拉取全量配置建立基线
package delta

import "time"

// UserChange 描述单个用户的增删改操作。
type UserChange struct {
	Email      string            `json:"email"`
	UUID       string            `json:"uuid,omitempty"`
	InboundTag string            `json:"inbound_tag,omitempty"`
	Level      int               `json:"level,omitempty"`
	Password   string            `json:"password,omitempty"`
	Extra      map[string]string `json:"extra,omitempty"`
}

// Sync 是控制面推送给 Agent 的增量同步消息。
type Sync struct {
	// NodeID 是目标节点 UUID。
	NodeID string `json:"node_id,omitempty"`
	// ServerCode 是目标服务器 code（如 "vps206"）。
	ServerCode string `json:"server_code"`
	// Kernel 是目标内核类型："xray" / "sing-box"。
	Kernel string `json:"kernel"`
	// AddUsers 是需要新增/修改的用户列表。
	AddUsers []UserChange `json:"add_users,omitempty"`
	// DelUsers 是需要删除的用户 email 列表。
	DelUsers []string `json:"del_users,omitempty"`
	// ConfigVersion 是目标配置版本号。
	ConfigVersion int64 `json:"config_version"`
	// ConfigHash 是应用后的预期配置 hash（一致性校验）。
	ConfigHash string `json:"config_hash,omitempty"`
	// Timestamp 是消息发送时间。
	Timestamp time.Time `json:"timestamp"`
}

// Ack 是 Agent 对 Delta Sync 的确认响应。
type Ack struct {
	ConfigVersion   int64  `json:"config_version"`
	Success         bool   `json:"success"`
	Error           string `json:"error,omitempty"`
	ApplyDurationMs int64  `json:"apply_duration_ms"`
}

// IsEmpty 判断 Delta 是否为空（无操作）。
func (d *Sync) IsEmpty() bool {
	return len(d.AddUsers) == 0 && len(d.DelUsers) == 0
}

// Merge 合并多个 Delta 为一个（同一 server_code 的批量合并去重）。
func Merge(deltas ...*Sync) *Sync {
	if len(deltas) == 0 {
		return nil
	}
	merged := &Sync{
		NodeID:        deltas[0].NodeID,
		ServerCode:    deltas[0].ServerCode,
		Kernel:        deltas[0].Kernel,
		ConfigVersion: deltas[0].ConfigVersion,
		Timestamp:     deltas[0].Timestamp,
	}

	addMap := make(map[string]*UserChange)
	delSet := make(map[string]bool)

	for _, d := range deltas {
		if d.ServerCode != merged.ServerCode {
			continue
		}
		if d.ConfigVersion > merged.ConfigVersion {
			merged.ConfigVersion = d.ConfigVersion
			merged.ConfigHash = d.ConfigHash
		}
		for _, u := range d.AddUsers {
			addMap[u.Email] = &u
		}
		for _, email := range d.DelUsers {
			delSet[email] = true
			delete(addMap, email) // 删除优先于添加
		}
	}

	for _, u := range addMap {
		merged.AddUsers = append(merged.AddUsers, *u)
	}
	for email := range delSet {
		merged.DelUsers = append(merged.DelUsers, email)
	}

	return merged
}
