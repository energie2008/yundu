package model

import "github.com/google/uuid"

// DeviceReportRequest Agent 上报在线设备 IP 的请求体。
//
// Node-Agent 周期性（或事件触发）将本节点观测到的某用户在线 IP 列表上报至面板，
// 面板据此在 Redis Hash 中聚合跨节点设备态，用于全局设备数限制判定。
type DeviceReportRequest struct {
	// UserID 用户 ID
	UserID uuid.UUID `json:"user_id" binding:"required"`
	// NodeID 上报节点 ID
	NodeID uuid.UUID `json:"node_id" binding:"required"`
	// IPs 该用户在本节点的在线 IP 列表（可带端口后缀，服务端会归一化）
	IPs []string `json:"ips"`
}

// AliveListResponse 批量查询用户当前在线设备数的响应体。
//
// Users 映射 user_id(字符串) -> 当前在线设备数。
// 设备数 = 跨节点去重后的有效 IP 数量（已剔除超过 TTL 的过期记录）。
type AliveListResponse struct {
	Users map[string]int `json:"users"` // user_id -> device_count
}

// DevicesListResponse 批量查询用户设备列表的响应体。
//
// Users 映射 user_id(字符串) -> 去重后的在线 IP 列表。
type DevicesListResponse struct {
	Users map[string][]string `json:"users"` // user_id -> []ip
}

// ClearNodeResponse 清除节点设备记录的响应体。
type ClearNodeResponse struct {
	Status  string `json:"status"`
	Cleared int    `json:"cleared"` // 清理的 user key 数量
}
