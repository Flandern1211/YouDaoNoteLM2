// Package dispatcher 实现 Worker Dispatcher。
package dispatcher

import (
	"context"
	"time"
)

// Role 运行角色。
type Role string

const (
	RoleAll    Role = "all"    // API + Worker（单进程）
	RoleAPI    Role = "api"    // 仅 API
	RoleWorker Role = "worker" // 仅 Worker
)

// RunState Run 状态。
type RunState string

const (
	RunStateQueued          RunState = "queued"
	RunStateRunning         RunState = "running"
	RunStateCancelRequested RunState = "cancel_requested"
)

// DispatcherConfig Dispatcher 配置。
type DispatcherConfig struct {
	Role            Role
	PollInterval    time.Duration
	WorkerID        string
	MaxConcurrent   int
}

// DefaultConfig 默认配置。
func DefaultConfig() DispatcherConfig {
	return DispatcherConfig{
		Role:          RoleAll,
		PollInterval:  5 * time.Second,
		WorkerID:      "worker-1",
		MaxConcurrent: 1,
	}
}

// QueuedRun 队列中的 Run。
type QueuedRun struct {
	ID          string
	RunID       string
	State       RunState
	Sequence    uint64
	CreatedAt   time.Time
}

// DispatcherStore Dispatcher 存储接口。
type DispatcherStore interface {
	// GetQueuedRuns 获取待处理的 Run 列表。
	GetQueuedRuns(ctx context.Context, limit int) ([]QueuedRun, error)
	// ClaimRun 领取 Run（原子操作）。
	ClaimRun(ctx context.Context, runID string, workerID string) error
	// CheckCancel 检查 Run 是否被取消。
	CheckCancel(ctx context.Context, runID string) (bool, error)
}

// Executor 执行器接口。
type Executor interface {
	// Execute 执行 Run。
	Execute(ctx context.Context, runID string, workerID string) error
}

// Dispatcher 调度器。
type Dispatcher struct {
	config   DispatcherConfig
	store    DispatcherStore
	executor Executor
	stopCh   chan struct{}
}

// NewDispatcher 创建 Dispatcher。
func NewDispatcher(config DispatcherConfig, store DispatcherStore, executor Executor) *Dispatcher {
	return &Dispatcher{
		config:   config,
		store:    store,
		executor: executor,
		stopCh:   make(chan struct{}),
	}
}

// Start 启动 Dispatcher。
func (d *Dispatcher) Start(ctx context.Context) {
	if d.config.Role == RoleAPI {
		return // API 角色不运行 Dispatcher
	}
	go d.run(ctx)
}

// Stop 停止 Dispatcher。
func (d *Dispatcher) Stop() {
	close(d.stopCh)
}

// run 运行调度循环。
func (d *Dispatcher) run(ctx context.Context) {
	ticker := time.NewTicker(d.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.stopCh:
			return
		case <-ticker.C:
			d.poll(ctx)
		}
	}
}

// poll 轮询并处理 queued Run。
func (d *Dispatcher) poll(ctx context.Context) {
	runs, err := d.store.GetQueuedRuns(ctx, d.config.MaxConcurrent)
	if err != nil {
		return
	}

	for _, run := range runs {
		d.processRun(ctx, run)
	}
}

// processRun 处理单个 Run。
func (d *Dispatcher) processRun(ctx context.Context, run QueuedRun) {
	// 检查是否被取消
	cancelled, _ := d.store.CheckCancel(ctx, run.RunID)
	if cancelled {
		return // 跳过已取消的 Run
	}

	// 领取 Run
	if err := d.store.ClaimRun(ctx, run.RunID, d.config.WorkerID); err != nil {
		return // 领取失败（可能已被其他 Worker 领取）
	}

	// 执行 Run
	d.executor.Execute(ctx, run.RunID, d.config.WorkerID)
}
