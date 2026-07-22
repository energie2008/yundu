package model

import (
	"time"

	"github.com/google/uuid"
)

// SubscribeTemplate 是按名称索引的订阅模板（对齐 Xboard v2_subscribe_templates）。
//
// 与 SubscriptionTemplate（按 code+target_client 索引、面向多客户端差异化管理）不同，
// SubscribeTemplate 采用更简单的 name→content 映射，直接对齐 Xboard 的
// subscribe_template('singbox') / subscribe_template('clash') helper：
// 渲染器按内核/格式名称（clash / clashmeta / singbox / surge / surfboard）取模板内容，
// 再将节点列表注入模板的 proxies/outbounds 占位。
//
// 内置模板（is_builtin=true）由迁移预置，可在管理端编辑但不可删除。
type SubscribeTemplate struct {
	ID        uuid.UUID `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`       // 模板键名：clash / clashmeta / singbox / surge / surfboard
	Content   string    `json:"content" db:"content"` // 模板内容（YAML / JSON / conf）
	IsBuiltin bool      `json:"is_builtin" db:"is_builtin"`
	Enabled   bool      `json:"enabled" db:"enabled"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// SubscribeTemplateUpdateRequest 管理端更新模板请求体。
type SubscribeTemplateUpdateRequest struct {
	Content string `json:"content" binding:"required"`
	Enabled *bool  `json:"enabled,omitempty"`
}
