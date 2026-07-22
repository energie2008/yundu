package executor

import (
	"context"
	"time"
)

type RuntimeStatus struct {
	Running      bool      `json:"running"`
	PID          int       `json:"pid"`
	Uptime       int64     `json:"uptime_seconds"`
	Version      string    `json:"version"`
	ConfigHash   string    `json:"config_hash"`
	StartedAt    time.Time `json:"started_at"`
	RestartCount int64     `json:"restart_count"`
}

// AlterUserOp 表示用户变更操作类型（P1-5）。
type AlterUserOp string

const (
	AlterUserAdded    AlterUserOp = "added"
	AlterUserRemoved  AlterUserOp = "removed"
	AlterUserModified AlterUserOp = "modified"
)

// AlterUser 描述单个用户的增量变更（P1-5）。
type AlterUser struct {
	InboundTag string                 // 所属 inbound 的 tag
	Email      string                 // 用户 email（xray client 的唯一键）
	Op         AlterUserOp            // 变更操作
	Account    map[string]interface{} // 新的 client 对象（added/modified 时填充，removed 时为 nil）
}

type RuntimeExecutor interface {
	Validate(configContent string) error
	Apply(configPath string, content string) error
	DryRun(ctx context.Context, configPath string) error
	Reload(ctx context.Context, configPath string) error
	Status(ctx context.Context) (*RuntimeStatus, error)
	Stop(ctx context.Context) error
	Rollback() error
	// AlterInbound P1-5: 对运行中的 runtime 执行增量用户变更（不重启进程）。
	// users 为空切片时直接返回 nil（无操作）。
	// 实现方应保证：
	//   1. 优先尝试 gRPC/API 增量更新（PID 不变，无断连）
	//   2. 失败时回退到 SIGUSR1 graceful reload（PID 不变，全量重载）
	//   3. 再失败时返回 error，由调用方决定是否走全量 restart
	AlterInbound(ctx context.Context, users []AlterUser) error
}
