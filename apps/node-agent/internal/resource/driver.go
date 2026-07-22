package resource

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// DefaultInterval 默认轮询周期。
const DefaultInterval = 30 * time.Second

// driver Resource Driver 的默认实现。
type driver struct {
	mu        sync.Mutex
	resources map[string]*entry
	logger    *slog.Logger
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

type entry struct {
	resource Resource
	interval time.Duration
	eventCh  chan Event
}

// NewDriver 创建默认 Driver。
func NewDriver(logger *slog.Logger) Driver {
	if logger == nil {
		logger = slog.Default()
	}
	return &driver{
		resources: make(map[string]*entry),
		logger:    logger.With("component", "resource-driver"),
	}
}

// Register 注册一个 Resource。
func (d *driver) Register(r Resource, opts ...Option) error {
	options := &Options{Interval: DefaultInterval}
	for _, opt := range opts {
		opt(options)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	kind := r.Kind()
	if _, exists := d.resources[kind]; exists {
		d.logger.Warn("resource already registered, replacing", "kind", kind)
	}
	d.resources[kind] = &entry{
		resource: r,
		interval: options.Interval,
		eventCh:  make(chan Event, 16),
	}
	d.logger.Info("resource registered", "kind", kind, "interval", options.Interval)
	return nil
}

// Start 启动所有已注册 Resource 的协调循环。
func (d *driver) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	d.cancel = cancel

	d.mu.Lock()
	defer d.mu.Unlock()

	for kind, e := range d.resources {
		d.wg.Add(1)
		go d.runLoop(ctx, e)
		d.logger.Info("resource loop started", "kind", kind)
	}
	return nil
}

// Stop 停止所有协调循环。
func (d *driver) Stop() {
	if d.cancel != nil {
		d.cancel()
	}
	d.wg.Wait()
	d.logger.Info("resource driver stopped")
}

// Trigger 手动触发某 Resource 的即时协调。
func (d *driver) Trigger(ctx context.Context, kind string, event Event) error {
	d.mu.Lock()
	e, ok := d.resources[kind]
	d.mu.Unlock()
	if !ok {
		return nil
	}
	select {
	case e.eventCh <- event:
		d.logger.Info("event triggered", "kind", kind, "event_type", event.Type)
		return nil
	default:
		d.logger.Warn("event channel full, dropping event", "kind", kind, "event_type", event.Type)
		return nil
	}
}

// runLoop 单个 Resource 的协调循环。
func (d *driver) runLoop(ctx context.Context, e *entry) {
	defer d.wg.Done()
	kind := e.resource.Kind()
	logger := d.logger.With("kind", kind)

	// 启动先跑一次
	if err := d.reconcileOnce(ctx, e); err != nil {
		logger.Warn("initial reconcile failed", "error", err)
	}

	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("resource loop stopped")
			return
		case event := <-e.eventCh:
			logger.Info("event received, triggering reconcile", "event_type", event.Type)
			if err := d.reconcileOnce(ctx, e); err != nil {
				logger.Error("event-driven reconcile failed", "error", err, "event_type", event.Type)
			}
		case <-ticker.C:
			if err := d.reconcileOnce(ctx, e); err != nil {
				logger.Error("periodic reconcile failed", "error", err)
			}
		}
	}
}

// reconcileOnce 执行一次完整的四段式协调。
func (d *driver) reconcileOnce(ctx context.Context, e *entry) error {
	kind := e.resource.Kind()
	logger := d.logger.With("kind", kind)

	// 1. Observe
	observed, err := e.resource.Observe(ctx)
	if err != nil {
		return err
	}

	// 2. FetchDesired
	desired, err := e.resource.FetchDesired(ctx)
	if err != nil {
		return err
	}

	// 3. Diff
	diff, err := e.resource.Diff(desired, observed)
	if err != nil {
		return err
	}

	if !diff.HasDrift {
		return nil
	}

	logger.Info("drift detected, applying",
		"level", string(diff.Level),
		"summary", diff.Summary,
		"changed_fields", diff.ChangedFields)

	// 4. Apply
	if err := e.resource.Apply(ctx, diff); err != nil {
		return err
	}

	// 5. Persist
	if err := e.resource.Persist(ctx, desired); err != nil {
		logger.Warn("persist failed", "error", err)
	}

	logger.Info("reconcile completed")
	return nil
}
