package cancel

import (
	"context"
	"testing"

	"YoudaoNoteLm/internal/agentharness/core"
)

// mockCancelStore 实现 CancelStore 接口。
type mockCancelStore struct {
	commands map[string]Command
	runs     map[string]core.Run
}

func newMockCancelStore() *mockCancelStore {
	return &mockCancelStore{
		commands: make(map[string]Command),
		runs:     make(map[string]core.Run),
	}
}

func (s *mockCancelStore) CreateCommand(ctx context.Context, cmd Command) error {
	s.commands[cmd.ID] = cmd
	return nil
}

func (s *mockCancelStore) GetCommand(ctx context.Context, id string) (Command, error) {
	cmd, exists := s.commands[id]
	if !exists {
		return Command{}, nil
	}
	return cmd, nil
}

func (s *mockCancelStore) GetCommandByRunAndKey(ctx context.Context, runID string, key string) (Command, error) {
	for _, cmd := range s.commands {
		if cmd.RunID == runID && cmd.IdempotencyKey == key {
			return cmd, nil
		}
	}
	return Command{}, nil
}

func (s *mockCancelStore) UpdateCommand(ctx context.Context, cmd Command) error {
	s.commands[cmd.ID] = cmd
	return nil
}

func (s *mockCancelStore) GetRun(ctx context.Context, runID string) (core.Run, error) {
	run, exists := s.runs[runID]
	if !exists {
		return core.Run{}, core.ErrRunNotFound
	}
	return run, nil
}

func (s *mockCancelStore) TransitionRun(ctx context.Context, runID string, from, to core.RunState, version core.StateVersion) error {
	run, exists := s.runs[runID]
	if !exists {
		return core.ErrRunNotFound
	}
	if run.State != from {
		return core.ErrAuthorityStale
	}
	run.State = to
	run.StateVersion = version + 1
	s.runs[runID] = run
	return nil
}

func TestCancelService_Cancel_Running(t *testing.T) {
	store := newMockCancelStore()
	svc := NewCancelService(store)

	// 创建 running 状态的 Run
	store.runs["run-1"] = core.Run{
		ID:           "run-1",
		State:        core.RunStateRunning,
		StateVersion: 1,
	}

	resp, err := svc.Cancel(context.Background(), CancelRequest{
		RunID:          "run-1",
		ActorID:        1,
		IdempotencyKey: "cancel-key-1",
	})
	if err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	if resp.Status != CommandStatusAccepted {
		t.Errorf("expected status 'accepted', got '%s'", resp.Status)
	}

	// 验证 Run 状态已改变
	run := store.runs["run-1"]
	if run.State != core.RunStateCancelRequested {
		t.Errorf("expected state 'cancel_requested', got '%s'", run.State)
	}
}

func TestCancelService_Cancel_Queued(t *testing.T) {
	store := newMockCancelStore()
	svc := NewCancelService(store)

	// 创建 queued 状态的 Run
	store.runs["run-1"] = core.Run{
		ID:           "run-1",
		State:        core.RunStateQueued,
		StateVersion: 1,
	}

	resp, err := svc.Cancel(context.Background(), CancelRequest{
		RunID:          "run-1",
		ActorID:        1,
		IdempotencyKey: "cancel-key-1",
	})
	if err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	if resp.Status != CommandStatusAccepted {
		t.Errorf("expected status 'accepted', got '%s'", resp.Status)
	}
}

func TestCancelService_Cancel_Terminal(t *testing.T) {
	store := newMockCancelStore()
	svc := NewCancelService(store)

	// 创建终态的 Run
	store.runs["run-1"] = core.Run{
		ID:           "run-1",
		State:        core.RunStateSucceeded,
		StateVersion: 1,
	}

	resp, err := svc.Cancel(context.Background(), CancelRequest{
		RunID:          "run-1",
		ActorID:        1,
		IdempotencyKey: "cancel-key-1",
	})
	if err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	if resp.Status != CommandStatusRejected {
		t.Errorf("expected status 'rejected', got '%s'", resp.Status)
	}
}

func TestCancelService_Cancel_Finalizing(t *testing.T) {
	store := newMockCancelStore()
	svc := NewCancelService(store)

	// 创建 finalizing 状态的 Run
	store.runs["run-1"] = core.Run{
		ID:           "run-1",
		State:        core.RunStateFinalizing,
		StateVersion: 1,
	}

	resp, err := svc.Cancel(context.Background(), CancelRequest{
		RunID:          "run-1",
		ActorID:        1,
		IdempotencyKey: "cancel-key-1",
	})
	if err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	if resp.Status != CommandStatusSuperseded {
		t.Errorf("expected status 'superseded', got '%s'", resp.Status)
	}
}

func TestCancelService_Cancel_Idempotent(t *testing.T) {
	store := newMockCancelStore()
	svc := NewCancelService(store)

	// 创建 running 状态的 Run
	store.runs["run-1"] = core.Run{
		ID:           "run-1",
		State:        core.RunStateRunning,
		StateVersion: 1,
	}

	// 第一次取消
	resp1, err := svc.Cancel(context.Background(), CancelRequest{
		RunID:          "run-1",
		ActorID:        1,
		IdempotencyKey: "cancel-key-1",
	})
	if err != nil {
		t.Fatalf("first Cancel failed: %v", err)
	}

	// 第二次取消（相同幂等键）
	resp2, err := svc.Cancel(context.Background(), CancelRequest{
		RunID:          "run-1",
		ActorID:        1,
		IdempotencyKey: "cancel-key-1",
	})
	if err != nil {
		t.Fatalf("second Cancel failed: %v", err)
	}

	if resp1.CommandID != resp2.CommandID {
		t.Errorf("expected same command ID, got %s and %s", resp1.CommandID, resp2.CommandID)
	}
}
