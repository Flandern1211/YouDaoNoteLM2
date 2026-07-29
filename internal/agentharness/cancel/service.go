package cancel

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"YoudaoNoteLm/internal/agentharness/core"
)

// CancelStore Cancel 所需的存储接口。
type CancelStore interface {
	// CreateCommand 创建命令（幂等）。
	CreateCommand(ctx context.Context, cmd Command) error
	// GetCommand 获取命令。
	GetCommand(ctx context.Context, id string) (Command, error)
	// GetCommandByRunAndKey 按 RunID 和 IdempotencyKey 获取命令。
	GetCommandByRunAndKey(ctx context.Context, runID string, key string) (Command, error)
	// UpdateCommand 更新命令状态。
	UpdateCommand(ctx context.Context, cmd Command) error
	// GetRun 获取 Run。
	GetRun(ctx context.Context, runID string) (core.Run, error)
	// TransitionRun 转换 Run 状态。
	TransitionRun(ctx context.Context, runID string, from, to core.RunState, version core.StateVersion) error
}

// CancelService 取消服务。
type CancelService struct {
	store CancelStore
}

// NewCancelService 创建 CancelService。
func NewCancelService(store CancelStore) *CancelService {
	return &CancelService{store: store}
}

// Cancel 取消 Run。
func (s *CancelService) Cancel(ctx context.Context, req CancelRequest) (CancelResponse, error) {
	// 1. 幂等检查：相同 idempotency_key 返回既有结果
	existing, err := s.store.GetCommandByRunAndKey(ctx, req.RunID, req.IdempotencyKey)
	if err == nil && existing.ID != "" {
		return CancelResponse{
			CommandID: existing.ID,
			Status:    existing.Status,
			Message:   "cancel command already exists",
		}, nil
	}

	// 2. 获取 Run
	run, err := s.store.GetRun(ctx, req.RunID)
	if err != nil {
		return CancelResponse{}, fmt.Errorf("获取 Run 失败: %w", err)
	}

	// 3. 检查 Run 状态
	if core.IsTerminalRunState(run.State) {
		return CancelResponse{
			Status:  CommandStatusRejected,
			Message: "run already in terminal state",
		}, nil
	}

	if run.State == core.RunStateFinalizing {
		// 执行 Outcome 已冻结，Cancel 只作审计
		cmd := Command{
			ID:             uuid.NewString(),
			RunID:          req.RunID,
			CommandType:    CommandTypeCancel,
			ActorID:        req.ActorID,
			IdempotencyKey: req.IdempotencyKey,
			StateVersion:   uint64(run.StateVersion),
			Status:         CommandStatusSuperseded,
			ReasonCode:     req.ReasonCode,
			CreatedAt:      time.Now(),
		}
		if err := s.store.CreateCommand(ctx, cmd); err != nil {
			return CancelResponse{}, fmt.Errorf("创建命令失败: %w", err)
		}

		return CancelResponse{
			CommandID: cmd.ID,
			Status:    CommandStatusSuperseded,
			Message:   "cancel_too_late: run already finalizing",
		}, nil
	}

	// 4. 创建 Cancel Command
	cmd := Command{
		ID:             uuid.NewString(),
		RunID:          req.RunID,
		CommandType:    CommandTypeCancel,
		ActorID:        req.ActorID,
		IdempotencyKey: req.IdempotencyKey,
		StateVersion:   uint64(run.StateVersion),
		Status:         CommandStatusAccepted,
		ReasonCode:     req.ReasonCode,
		CreatedAt:      time.Now(),
	}
	if err := s.store.CreateCommand(ctx, cmd); err != nil {
		return CancelResponse{}, fmt.Errorf("创建命令失败: %w", err)
	}

	// 5. 转换 Run 状态到 cancel_requested
	if err := s.store.TransitionRun(ctx, req.RunID, run.State, core.RunStateCancelRequested, run.StateVersion); err != nil {
		// 状态转换失败（可能已被其他操作转换），更新命令状态
		cmd.Status = CommandStatusSuperseded
		s.store.UpdateCommand(ctx, cmd)
		return CancelResponse{
			CommandID: cmd.ID,
			Status:    CommandStatusSuperseded,
			Message:   "state transition failed",
		}, nil
	}

	return CancelResponse{
		CommandID: cmd.ID,
		Status:    CommandStatusAccepted,
		Message:   "cancel accepted",
	}, nil
}

// GetCancelCommand 获取取消命令。
func (s *CancelService) GetCancelCommand(ctx context.Context, runID string) (Command, error) {
	// 查询该 Run 的最新 Cancel 命令
	// 简化实现：实际需要查询最新的一条
	return Command{}, nil
}

// ShouldCancel 检查是否应该取消。
func (s *CancelService) ShouldCancel(ctx context.Context, runID string) bool {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return false
	}
	return run.DesiredState == core.DesiredStateCancelled
}
