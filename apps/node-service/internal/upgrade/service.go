package upgrade

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/airport-panel/node-service/internal/model"
	"github.com/google/uuid"
)

// AuditWriter 预留的审计日志接口（app.go 传 nil 时调用前判 nil）
type AuditWriter interface {
	Audit(ctx context.Context, action, resource string, before, after interface{})
}

// TaskStore 抽象 UpgradeTaskRepo 的数据访问（便于测试注入 mock）
type TaskStore interface {
	Create(ctx context.Context, t *RuntimeUpgradeTask) error
	GetByID(ctx context.Context, id uuid.UUID) (*RuntimeUpgradeTask, error)
	ListByServer(ctx context.Context, serverID uuid.UUID) ([]*RuntimeUpgradeTask, error)
	ListByBatch(ctx context.Context, batchID uuid.UUID) ([]*RuntimeUpgradeTask, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status UpgradeStatus, errMsg *string) error
	HasRunningTask(ctx context.Context, serverID uuid.UUID) (bool, error)
}

// RuntimeFetcher 用于获取 runtime 当前版本（from_version），由 app.go 注入 repo.RuntimeRepo
type RuntimeFetcher interface {
	GetByID(ctx context.Context, id uuid.UUID) (*model.Runtime, error)
}

// UpgradeService 封装 runtime 升级任务的业务逻辑
type UpgradeService struct {
	store   TaskStore
	runtime RuntimeFetcher
	audit   AuditWriter
	logger  *slog.Logger
}

func NewUpgradeService(store TaskStore, runtime RuntimeFetcher, audit AuditWriter, logger *slog.Logger) *UpgradeService {
	if logger == nil {
		logger = slog.Default()
	}
	return &UpgradeService{
		store:   store,
		runtime: runtime,
		audit:   audit,
		logger:  logger,
	}
}

// resolveFromVersion 取 runtime 当前版本作为 from_version；runtime 不存在返回 ErrRuntimeNotFound
func (s *UpgradeService) resolveFromVersion(ctx context.Context, runtimeID uuid.UUID) (string, error) {
	if s.runtime == nil {
		return "", nil
	}
	rt, err := s.runtime.GetByID(ctx, runtimeID)
	if err != nil {
		return "", err
	}
	if rt == nil {
		return "", ErrRuntimeNotFound
	}
	if rt.RuntimeVersion != nil {
		return *rt.RuntimeVersion, nil
	}
	return "", nil
}

// CreateUpgrade 发起单台升级任务
func (s *UpgradeService) CreateUpgrade(ctx context.Context, serverID uuid.UUID, req *CreateRequest) (*RuntimeUpgradeTask, error) {
	if req.ToVersion == "" {
		return nil, ErrVersionSame
	}
	running, err := s.store.HasRunningTask(ctx, serverID)
	if err != nil {
		return nil, err
	}
	if running {
		return nil, ErrUpgradeAlreadyRunning
	}

	fromVersion, err := s.resolveFromVersion(ctx, req.RuntimeID)
	if err != nil {
		return nil, err
	}
	if fromVersion != "" && fromVersion == req.ToVersion {
		return nil, ErrVersionSame
	}

	task := &RuntimeUpgradeTask{
		ID:             uuid.New(),
		ServerID:       serverID,
		RuntimeID:      req.RuntimeID,
		FromVersion:    fromVersion,
		ToVersion:      req.ToVersion,
		Status:         StatusPending,
		Scope:           ScopeSingle,
		DownloadURL:    req.DownloadURL,
		ExpectedSha256: req.ExpectedSha256,
	}
	if err := s.store.Create(ctx, task); err != nil {
		return nil, err
	}
	if s.audit != nil {
		s.audit.Audit(ctx, "create", "runtime_upgrade_task", nil, task)
	}
	return task, nil
}

// CreateBatchUpgrade 批量升级（同一 batch_id）
func (s *UpgradeService) CreateBatchUpgrade(ctx context.Context, req *BatchCreateRequest) ([]*RuntimeUpgradeTask, error) {
	batchID := uuid.New()
	tasks := make([]*RuntimeUpgradeTask, 0, len(req.Items))

	for _, item := range req.Items {
		running, err := s.store.HasRunningTask(ctx, item.ServerID)
		if err != nil {
			return nil, err
		}
		if running {
			return nil, fmt.Errorf("%w: server_id=%s", ErrUpgradeAlreadyRunning, item.ServerID)
		}
		fromVersion, err := s.resolveFromVersion(ctx, item.RuntimeID)
		if err != nil {
			return nil, err
		}
		if fromVersion != "" && fromVersion == item.ToVersion {
			return nil, fmt.Errorf("%w: server_id=%s", ErrVersionSame, item.ServerID)
		}
		bid := batchID
		task := &RuntimeUpgradeTask{
			ID:             uuid.New(),
			ServerID:       item.ServerID,
			RuntimeID:      item.RuntimeID,
			FromVersion:    fromVersion,
			ToVersion:      item.ToVersion,
			Status:         StatusPending,
			Scope:           ScopeBatch,
			BatchID:        &bid,
			DownloadURL:    item.DownloadURL,
			ExpectedSha256: item.ExpectedSha256,
		}
		if err := s.store.Create(ctx, task); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	if s.audit != nil {
		s.audit.Audit(ctx, "create_batch", "runtime_upgrade_task", nil, tasks)
	}
	return tasks, nil
}

// CreateCanaryUpgrade 灰度升级：按百分比从 targets 中选取节点
func (s *UpgradeService) CreateCanaryUpgrade(ctx context.Context, req *CanaryUpgradeRequest) ([]*RuntimeUpgradeTask, error) {
	if req.CanaryPercent < 1 || req.CanaryPercent > 100 {
		return nil, ErrInvalidCanaryPercent
	}
	if len(req.Targets) == 0 {
		return nil, ErrNoCanaryTargets
	}

	// 选取数量 = ceil(N * percent / 100)，最多为 N
	selectCount := int(math.Ceil(float64(len(req.Targets)) * float64(req.CanaryPercent) / 100))
	if selectCount > len(req.Targets) {
		selectCount = len(req.Targets)
	}
	if selectCount < 1 {
		selectCount = 1
	}
	selected := req.Targets[:selectCount]

	tasks := make([]*RuntimeUpgradeTask, 0, selectCount)
	for _, target := range selected {
		running, err := s.store.HasRunningTask(ctx, target.ServerID)
		if err != nil {
			return nil, err
		}
		if running {
			return nil, fmt.Errorf("%w: server_id=%s", ErrUpgradeAlreadyRunning, target.ServerID)
		}
		fromVersion, err := s.resolveFromVersion(ctx, target.RuntimeID)
		if err != nil {
			return nil, err
		}
		if fromVersion != "" && fromVersion == req.ToVersion {
			return nil, fmt.Errorf("%w: server_id=%s", ErrVersionSame, target.ServerID)
		}
		pct := req.CanaryPercent
		task := &RuntimeUpgradeTask{
			ID:             uuid.New(),
			ServerID:       target.ServerID,
			RuntimeID:      target.RuntimeID,
			FromVersion:    fromVersion,
			ToVersion:      req.ToVersion,
			Status:         StatusPending,
			Scope:           ScopeCanary,
			CanaryPercent:  &pct,
			DownloadURL:    req.DownloadURL,
			ExpectedSha256: req.ExpectedSha256,
		}
		if err := s.store.Create(ctx, task); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	if s.audit != nil {
		s.audit.Audit(ctx, "create_canary", "runtime_upgrade_task", nil, tasks)
	}
	return tasks, nil
}

// MarkSucceeded 标记任务成功
func (s *UpgradeService) MarkSucceeded(ctx context.Context, taskID uuid.UUID) (*RuntimeUpgradeTask, error) {
	task, err := s.getTaskOrErr(ctx, taskID)
	if err != nil {
		return nil, err
	}
	before := *task
	if err := s.store.UpdateStatus(ctx, taskID, StatusSucceeded, nil); err != nil {
		return nil, err
	}
	task.Status = StatusSucceeded
	now := time.Now()
	task.CompletedAt = &now
	if s.audit != nil {
		s.audit.Audit(ctx, "succeeded", "runtime_upgrade_task", before, task)
	}
	return task, nil
}

// MarkFailed 标记任务失败并记录错误信息
func (s *UpgradeService) MarkFailed(ctx context.Context, taskID uuid.UUID, errMsg string) (*RuntimeUpgradeTask, error) {
	task, err := s.getTaskOrErr(ctx, taskID)
	if err != nil {
		return nil, err
	}
	before := *task
	msg := errMsg
	if err := s.store.UpdateStatus(ctx, taskID, StatusFailed, &msg); err != nil {
		return nil, err
	}
	task.Status = StatusFailed
	task.ErrorMessage = &msg
	now := time.Now()
	task.CompletedAt = &now
	if s.audit != nil {
		s.audit.Audit(ctx, "failed", "runtime_upgrade_task", before, task)
	}
	return task, nil
}

// Rollback 回滚到 from_version：标记 status=rolled_back
func (s *UpgradeService) Rollback(ctx context.Context, taskID uuid.UUID) (*RuntimeUpgradeTask, error) {
	task, err := s.getTaskOrErr(ctx, taskID)
	if err != nil {
		return nil, err
	}
	// 仅 succeeded / failed 的任务可回滚
	if task.Status != StatusSucceeded && task.Status != StatusFailed {
		return nil, ErrTaskNotRollbackable
	}
	before := *task
	if err := s.store.UpdateStatus(ctx, taskID, StatusRolledBack, nil); err != nil {
		return nil, err
	}
	task.Status = StatusRolledBack
	now := time.Now()
	task.CompletedAt = &now
	if s.audit != nil {
		s.audit.Audit(ctx, "rollback", "runtime_upgrade_task", before, task)
	}
	return task, nil
}

// getTaskOrErr 取任务，不存在返回 ErrUpgradeTaskNotFound
func (s *UpgradeService) getTaskOrErr(ctx context.Context, taskID uuid.UUID) (*RuntimeUpgradeTask, error) {
	task, err := s.store.GetByID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, ErrUpgradeTaskNotFound
	}
	return task, nil
}

// GetByID 查看任务详情
func (s *UpgradeService) GetByID(ctx context.Context, taskID uuid.UUID) (*RuntimeUpgradeTask, error) {
	return s.getTaskOrErr(ctx, taskID)
}

// ListByServer 查看某服务器的升级历史
func (s *UpgradeService) ListByServer(ctx context.Context, serverID uuid.UUID) ([]*RuntimeUpgradeTask, error) {
	return s.store.ListByServer(ctx, serverID)
}

// ListByBatch 查看某批次下的全部任务
func (s *UpgradeService) ListByBatch(ctx context.Context, batchID uuid.UUID) ([]*RuntimeUpgradeTask, error) {
	return s.store.ListByBatch(ctx, batchID)
}
