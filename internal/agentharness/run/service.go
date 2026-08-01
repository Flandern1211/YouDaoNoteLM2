// Package run 定义 RunStore 的使用方接口，仅依赖 core 包。
package run

import (
	"context"

	"YoudaoNoteLm/internal/agentharness/core"
)

// Service 提供 Run 的高级操作。
type Service struct {
	store core.RunStore
}

// NewService 创建 Service。
func NewService(store core.RunStore) *Service {
	return &Service{store: store}
}

// CreateQueued 创建一个 queued 状态的 Run。
func (s *Service) CreateQueued(ctx context.Context, run core.Run) error {
	if run.State != core.RunStateQueued {
		return core.ErrInvalidTransition
	}
	return s.store.CreateQueued(ctx, run)
}

// Get 获取 Run。
func (s *Service) Get(ctx context.Context, id core.RunID) (core.Run, error) {
	return s.store.Get(ctx, id)
}

// Claim 原子地 claim 一个 queued Run。
func (s *Service) Claim(ctx context.Context, id core.RunID, workerID string) (core.Run, core.Attempt, error) {
	return s.store.Claim(ctx, id, workerID)
}

// Transition 执行状态转换。
func (s *Service) Transition(ctx context.Context, req core.TransitionRequest) (core.Run, error) {
	// 验证转换合法性
	if err := core.ValidateTransition(req.CurrentState, req.TargetState); err != nil {
		return core.Run{}, err
	}
	return s.store.Transition(ctx, req)
}

// CreateStep 创建 Step。
func (s *Service) CreateStep(ctx context.Context, step core.Step) error {
	return s.store.CreateStep(ctx, step)
}

// FinishStep 完成 Step。
func (s *Service) FinishStep(ctx context.Context, req core.FinishStepRequest) (core.Step, error) {
	return s.store.FinishStep(ctx, req)
}
