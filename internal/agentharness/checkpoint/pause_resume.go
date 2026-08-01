package checkpoint

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// RunState Run 状态。
type RunState string

const (
	RunStateQueued         RunState = "queued"
	RunStateRunning        RunState = "running"
	RunStatePauseRequested RunState = "pause_requested"
	RunStatePausing        RunState = "pausing"
	RunStatePaused         RunState = "paused"
	RunStateSuspended      RunState = "suspended"
	RunStateCancelRequested RunState = "cancel_requested"
)

// PauseStore Pause/Resume 所需存储接口。
type PauseStore interface {
	GetRun(ctx context.Context, runID string) (RunState, error)
	TransitionRun(ctx context.Context, runID string, from, to RunState) error
	SetDesiredState(ctx context.Context, runID string, desired string) error
	CreateRecord(ctx context.Context, record PauseResumeRecord) error
}

// PauseService 暂停服务。
type PauseService struct {
	store          PauseStore
	checkpointStore CheckpointStore
}

// NewPauseService 创建 PauseService。
func NewPauseService(store PauseStore, cpStore CheckpointStore) *PauseService {
	return &PauseService{
		store:          store,
		checkpointStore: cpStore,
	}
}

// RequestPause 请求暂停。
func (s *PauseService) RequestPause(ctx context.Context, runID string) error {
	state, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("获取 Run 失败: %w", err)
	}

	if state != RunStateRunning {
		return fmt.Errorf("只能暂停 running 状态的 Run，当前状态: %s", state)
	}

	// 设置 desired_state=paused
	if err := s.store.SetDesiredState(ctx, runID, "paused"); err != nil {
		return err
	}

	// 转换到 pause_requested
	return s.store.TransitionRun(ctx, runID, RunStateRunning, RunStatePauseRequested)
}

// ConfirmPause 确认暂停（Worker 调用）。
func (s *PauseService) ConfirmPause(ctx context.Context, runID string, cp Checkpoint) error {
	state, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return err
	}

	if state != RunStatePauseRequested && state != RunStatePausing {
		return fmt.Errorf("无法确认暂停，当前状态: %s", state)
	}

	// 转换到 pausing
	if state == RunStatePauseRequested {
		if err := s.store.TransitionRun(ctx, runID, RunStatePauseRequested, RunStatePausing); err != nil {
			return err
		}
	}

	// 写入检查点
	if err := s.checkpointStore.Set(ctx, cp); err != nil {
		// 检查点失败，进入 suspended
		s.store.TransitionRun(ctx, runID, RunStatePausing, RunStateSuspended)
		return fmt.Errorf("写入检查点失败: %w", err)
	}

	// 转换到 paused
	return s.store.TransitionRun(ctx, runID, RunStatePausing, RunStatePaused)
}

// ResumeService 恢复服务。
type ResumeService struct {
	store          PauseStore
	checkpointStore CheckpointStore
}

// NewResumeService 创建 ResumeService。
func NewResumeService(store PauseStore, cpStore CheckpointStore) *ResumeService {
	return &ResumeService{
		store:          store,
		checkpointStore: cpStore,
	}
}

// Resume 恢复执行。
func (s *ResumeService) Resume(ctx context.Context, runID string) error {
	state, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return err
	}

	// 只接受 paused 状态
	if state != RunStatePaused {
		return fmt.Errorf("只能恢复 paused 状态的 Run，当前状态: %s", state)
	}

	// 验证检查点存在且有效
	_, err = s.checkpointStore.Get(ctx, runID)
	if err != nil {
		return fmt.Errorf("检查点不可用: %w", err)
	}

	// 创建 Resume 记录
	record := PauseResumeRecord{
		ID:          uuid.NewString(),
		RunID:       runID,
		CommandType: "resume",
		Status:      "accepted",
		CreatedAt:   time.Now(),
	}
	if err := s.store.CreateRecord(ctx, record); err != nil {
		return err
	}

	// 转换到 queued（等待 Claim 创建新 Attempt）
	return s.store.TransitionRun(ctx, runID, RunStatePaused, RunStateQueued)
}
