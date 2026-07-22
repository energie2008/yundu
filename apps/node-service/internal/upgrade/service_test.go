package upgrade

import (
	"context"
	"testing"

	"github.com/airport-panel/node-service/internal/model"
	"github.com/google/uuid"
)

// fakeStore 内存实现 TaskStore
type fakeStore struct {
	tasks        map[uuid.UUID]*RuntimeUpgradeTask
	createErr    error
	runningSet   map[uuid.UUID]bool // 显式标记哪些 server 有运行中任务
	updateCalls  []updateCall
}

type updateCall struct {
	id     uuid.UUID
	status UpgradeStatus
	errMsg *string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		tasks:      map[uuid.UUID]*RuntimeUpgradeTask{},
		runningSet: map[uuid.UUID]bool{},
	}
}

func (f *fakeStore) Create(ctx context.Context, t *RuntimeUpgradeTask) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.tasks[t.ID] = t
	return nil
}
func (f *fakeStore) GetByID(ctx context.Context, id uuid.UUID) (*RuntimeUpgradeTask, error) {
	if t, ok := f.tasks[id]; ok {
		return t, nil
	}
	return nil, nil
}
func (f *fakeStore) ListByServer(ctx context.Context, serverID uuid.UUID) ([]*RuntimeUpgradeTask, error) {
	var out []*RuntimeUpgradeTask
	for _, t := range f.tasks {
		if t.ServerID == serverID {
			out = append(out, t)
		}
	}
	return out, nil
}
func (f *fakeStore) ListByBatch(ctx context.Context, batchID uuid.UUID) ([]*RuntimeUpgradeTask, error) {
	var out []*RuntimeUpgradeTask
	for _, t := range f.tasks {
		if t.BatchID != nil && *t.BatchID == batchID {
			out = append(out, t)
		}
	}
	return out, nil
}
func (f *fakeStore) UpdateStatus(ctx context.Context, id uuid.UUID, status UpgradeStatus, errMsg *string) error {
	f.updateCalls = append(f.updateCalls, updateCall{id, status, errMsg})
	if t, ok := f.tasks[id]; ok {
		t.Status = status
		if errMsg != nil {
			msg := *errMsg
			t.ErrorMessage = &msg
		}
	}
	return nil
}
func (f *fakeStore) HasRunningTask(ctx context.Context, serverID uuid.UUID) (bool, error) {
	return f.runningSet[serverID], nil
}

// fakeRuntimeFetcher 内存实现 RuntimeFetcher
type fakeRuntimeFetcher struct {
	runtimes map[uuid.UUID]*model.Runtime
}

func newFakeRuntimeFetcher() *fakeRuntimeFetcher {
	return &fakeRuntimeFetcher{runtimes: map[uuid.UUID]*model.Runtime{}}
}

func (f *fakeRuntimeFetcher) GetByID(ctx context.Context, id uuid.UUID) (*model.Runtime, error) {
	if r, ok := f.runtimes[id]; ok {
		return r, nil
	}
	return nil, nil
}

func strPtr(s string) *string { return &s }

func TestCreateUpgrade_Success(t *testing.T) {
	store := newFakeStore()
	rt := &model.Runtime{ID: uuid.New(), RuntimeVersion: strPtr("1.8.0")}
	fetcher := newFakeRuntimeFetcher()
	fetcher.runtimes[rt.ID] = rt

	svc := NewUpgradeService(store, fetcher, nil, nil)
	serverID := uuid.New()
	req := &CreateRequest{
		RuntimeID: rt.ID,
		ToVersion: "1.8.24",
	}
	task, err := svc.CreateUpgrade(context.Background(), serverID, req)
	if err != nil {
		t.Fatalf("CreateUpgrade error: %v", err)
	}
	if task.FromVersion != "1.8.0" {
		t.Errorf("FromVersion = %q, want 1.8.0", task.FromVersion)
	}
	if task.ToVersion != "1.8.24" {
		t.Errorf("ToVersion = %q, want 1.8.24", task.ToVersion)
	}
	if task.Status != StatusPending {
		t.Errorf("Status = %q, want pending", task.Status)
	}
	if task.Scope != ScopeSingle {
		t.Errorf("Scope = %q, want single", task.Scope)
	}
	if task.BatchID != nil {
		t.Errorf("BatchID should be nil for single upgrade")
	}
}

func TestCreateUpgrade_AlreadyRunning(t *testing.T) {
	store := newFakeStore()
	serverID := uuid.New()
	store.runningSet[serverID] = true
	svc := NewUpgradeService(store, newFakeRuntimeFetcher(), nil, nil)

	_, err := svc.CreateUpgrade(context.Background(), serverID, &CreateRequest{
		RuntimeID: uuid.New(),
		ToVersion: "1.8.24",
	})
	if err != ErrUpgradeAlreadyRunning {
		t.Errorf("expected ErrUpgradeAlreadyRunning, got %v", err)
	}
}

func TestCreateUpgrade_VersionSame(t *testing.T) {
	store := newFakeStore()
	rt := &model.Runtime{ID: uuid.New(), RuntimeVersion: strPtr("1.8.24")}
	fetcher := newFakeRuntimeFetcher()
	fetcher.runtimes[rt.ID] = rt
	svc := NewUpgradeService(store, fetcher, nil, nil)

	_, err := svc.CreateUpgrade(context.Background(), uuid.New(), &CreateRequest{
		RuntimeID: rt.ID,
		ToVersion: "1.8.24",
	})
	if err != ErrVersionSame {
		t.Errorf("expected ErrVersionSame, got %v", err)
	}
}

func TestCreateUpgrade_RuntimeNotFound(t *testing.T) {
	store := newFakeStore()
	svc := NewUpgradeService(store, newFakeRuntimeFetcher(), nil, nil)

	_, err := svc.CreateUpgrade(context.Background(), uuid.New(), &CreateRequest{
		RuntimeID: uuid.New(),
		ToVersion: "1.8.24",
	})
	if err != ErrRuntimeNotFound {
		t.Errorf("expected ErrRuntimeNotFound, got %v", err)
	}
}

func TestCreateBatchUpgrade_SharedBatchID(t *testing.T) {
	store := newFakeStore()
	rt1 := &model.Runtime{ID: uuid.New(), RuntimeVersion: strPtr("1.8.0")}
	rt2 := &model.Runtime{ID: uuid.New(), RuntimeVersion: strPtr("1.8.0")}
	fetcher := newFakeRuntimeFetcher()
	fetcher.runtimes[rt1.ID] = rt1
	fetcher.runtimes[rt2.ID] = rt2
	svc := NewUpgradeService(store, fetcher, nil, nil)

	tasks, err := svc.CreateBatchUpgrade(context.Background(), &BatchCreateRequest{
		Items: []BatchItem{
			{ServerID: uuid.New(), RuntimeID: rt1.ID, ToVersion: "1.8.24"},
			{ServerID: uuid.New(), RuntimeID: rt2.ID, ToVersion: "1.8.24"},
		},
	})
	if err != nil {
		t.Fatalf("CreateBatchUpgrade error: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0].BatchID == nil || tasks[1].BatchID == nil {
		t.Fatalf("batch tasks must have batch_id")
	}
	if *tasks[0].BatchID != *tasks[1].BatchID {
		t.Errorf("batch_id should be shared, got %s and %s", *tasks[0].BatchID, *tasks[1].BatchID)
	}
	for _, tk := range tasks {
		if tk.Scope != ScopeBatch {
			t.Errorf("Scope = %q, want batch", tk.Scope)
		}
		if tk.Status != StatusPending {
			t.Errorf("Status = %q, want pending", tk.Status)
		}
	}
}

func TestCreateCanaryUpgrade_SelectsByPercent(t *testing.T) {
	store := newFakeStore()
	fetcher := newFakeRuntimeFetcher()
	// 4 个目标，50% 应选 2 个
	targets := make([]CanaryTarget, 4)
	for i := range targets {
		rt := &model.Runtime{ID: uuid.New(), RuntimeVersion: strPtr("1.8.0")}
		fetcher.runtimes[rt.ID] = rt
		targets[i] = CanaryTarget{ServerID: uuid.New(), RuntimeID: rt.ID}
	}
	svc := NewUpgradeService(store, fetcher, nil, nil)

	tasks, err := svc.CreateCanaryUpgrade(context.Background(), &CanaryUpgradeRequest{
		ToVersion:     "1.8.24",
		CanaryPercent: 50,
		Targets:       targets,
	})
	if err != nil {
		t.Fatalf("CreateCanaryUpgrade error: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 canary tasks (50%% of 4), got %d", len(tasks))
	}
	for _, tk := range tasks {
		if tk.Scope != ScopeCanary {
			t.Errorf("Scope = %q, want canary", tk.Scope)
		}
		if tk.CanaryPercent == nil || *tk.CanaryPercent != 50 {
			t.Errorf("CanaryPercent = %v, want 50", tk.CanaryPercent)
		}
	}
}

func TestCreateCanaryUpgrade_InvalidPercent(t *testing.T) {
	svc := NewUpgradeService(newFakeStore(), newFakeRuntimeFetcher(), nil, nil)
	_, err := svc.CreateCanaryUpgrade(context.Background(), &CanaryUpgradeRequest{
		ToVersion:     "1.8.24",
		CanaryPercent: 0,
		Targets:       []CanaryTarget{{ServerID: uuid.New(), RuntimeID: uuid.New()}},
	})
	if err != ErrInvalidCanaryPercent {
		t.Errorf("expected ErrInvalidCanaryPercent, got %v", err)
	}
}

func TestRollback_Success(t *testing.T) {
	store := newFakeStore()
	svc := NewUpgradeService(store, newFakeRuntimeFetcher(), nil, nil)

	// 预置一个已成功的任务
	task := &RuntimeUpgradeTask{
		ID:        uuid.New(),
		ServerID:  uuid.New(),
		Status:    StatusSucceeded,
		FromVersion: "1.8.0",
		ToVersion: "1.8.24",
	}
	store.tasks[task.ID] = task

	updated, err := svc.Rollback(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("Rollback error: %v", err)
	}
	if updated.Status != StatusRolledBack {
		t.Errorf("Status = %q, want rolled_back", updated.Status)
	}
	// 验证 store 被调用更新为 rolled_back
	last := store.updateCalls[len(store.updateCalls)-1]
	if last.status != StatusRolledBack {
		t.Errorf("store.UpdateStatus called with %q, want rolled_back", last.status)
	}
}

func TestRollback_NotRollbackable(t *testing.T) {
	store := newFakeStore()
	svc := NewUpgradeService(store, newFakeRuntimeFetcher(), nil, nil)

	task := &RuntimeUpgradeTask{ID: uuid.New(), ServerID: uuid.New(), Status: StatusPending}
	store.tasks[task.ID] = task

	_, err := svc.Rollback(context.Background(), task.ID)
	if err != ErrTaskNotRollbackable {
		t.Errorf("expected ErrTaskNotRollbackable, got %v", err)
	}
}

func TestGetByID_NotFound(t *testing.T) {
	svc := NewUpgradeService(newFakeStore(), newFakeRuntimeFetcher(), nil, nil)
	_, err := svc.GetByID(context.Background(), uuid.New())
	if err != ErrUpgradeTaskNotFound {
		t.Errorf("expected ErrUpgradeTaskNotFound, got %v", err)
	}
}

func TestMarkSucceeded_And_MarkFailed(t *testing.T) {
	store := newFakeStore()
	svc := NewUpgradeService(store, newFakeRuntimeFetcher(), nil, nil)

	task := &RuntimeUpgradeTask{ID: uuid.New(), ServerID: uuid.New(), Status: StatusRunning}
	store.tasks[task.ID] = task

	if _, err := svc.MarkSucceeded(context.Background(), task.ID); err != nil {
		t.Fatalf("MarkSucceeded error: %v", err)
	}
	if task.Status != StatusSucceeded {
		t.Errorf("after MarkSucceeded Status = %q, want succeeded", task.Status)
	}
	if task.CompletedAt == nil {
		t.Errorf("CompletedAt should be set after MarkSucceeded")
	}

	if _, err := svc.MarkFailed(context.Background(), task.ID, "boom"); err != nil {
		t.Fatalf("MarkFailed error: %v", err)
	}
	if task.Status != StatusFailed {
		t.Errorf("after MarkFailed Status = %q, want failed", task.Status)
	}
	if task.ErrorMessage == nil || *task.ErrorMessage != "boom" {
		t.Errorf("ErrorMessage = %v, want boom", task.ErrorMessage)
	}
}
