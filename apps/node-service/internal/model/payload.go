package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ConfigPayload 是 config_payloads 表的 ORM 模型，存储加密的 Payload Manifest。
// 每条记录对应一个 ConfigVersion 的加密配置包，Agent 通过 HTTP 拉取后
// 使用共享密钥 AES-GCM 解密得到明文配置。
type ConfigPayload struct {
	ID               uuid.UUID       `json:"id"`
	ConfigVersionID  uuid.UUID       `json:"config_version_id"`
	VersionNo        int64           `json:"version_no"`
	SHA256           string          `json:"sha256"`
	Kernel           string          `json:"kernel"`
	RollbackStrategy string          `json:"rollback_strategy"`
	PayloadEncrypted bool            `json:"payload_encrypted"`
	Content          json.RawMessage `json:"content"` // 加密后的 content（base64 字符串字面量）
	CreatedAt        time.Time       `json:"created_at"`
}

// DeploymentResult 是 deployment_results 表的 ORM 模型，存储 Agent 上报的 ACK/NACK。
// Agent 在 precheck / activate / healthcheck 各阶段完成后通过 HTTP 上报结果，
// 控制面据此推进部署 phase 或触发回滚。
type DeploymentResult struct {
	ID                 uuid.UUID `json:"id"`
	DeploymentTargetID uuid.UUID `json:"deployment_target_id"`
	ServerCode         string    `json:"server_code"`
	VersionNo          int64     `json:"version_no"`
	Status             string    `json:"status"` // ack / nack
	Phase              string    `json:"phase"`  // precheck / activate / healthcheck
	Error              string    `json:"error,omitempty"`
	ApplyDurationMs    int64     `json:"apply_duration_ms"`
	ReportedAt         time.Time `json:"reported_at"`
	CreatedAt          time.Time `json:"created_at"`
}
