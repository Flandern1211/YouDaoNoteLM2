package core

import "context"

// RunStore 端口，由 run 包使用，store 包实现。
// 不能泄露 GORM model。
type RunStore interface {
	// CreateQueued 创建一个 queued 状态的 Run。
	CreateQueued(ctx context.Context, run Run) error

	// Get 获取 Run。
	Get(ctx context.Context, id RunID) (Run, error)

	// Claim 原子地：锁定或 CAS 更新 queued Run、递增 fencing token、创建新的 Attempt、设置 current_attempt_id 和 started_at。
	Claim(ctx context.Context, id RunID, workerID string) (Run, Attempt, error)

	// Transition 只负责状态和结构化错误字段。
	Transition(ctx context.Context, req TransitionRequest) (Run, error)

	// CreateStep 创建 Step。
	CreateStep(ctx context.Context, step Step) error

	// FinishStep 完成 Step。
	FinishStep(ctx context.Context, req FinishStepRequest) (Step, error)
}

// FinishStepRequest 完成 Step 请求。
type FinishStepRequest struct {
	StepID          StepID
	RunID           RunID
	AttemptID       AttemptID
	FencingToken    FencingToken
	State           StepState
	ErrorClass      *ErrorClass
	ErrorCode       *string
	ResultArtifactRef *string
}

// AdmissionStore 扩展 RunStore，支持 Admission 原子接纳。
type AdmissionStore interface {
	RunStore

	// Accept 在一个事务中完成：创建入口 Message、queued Run 和首条 run.accepted 事件。
	// 返回 AcceptedRun 与可能的幂等重放结果。
	Accept(ctx context.Context, req AcceptRequest) (AcceptedRun, error)
}
