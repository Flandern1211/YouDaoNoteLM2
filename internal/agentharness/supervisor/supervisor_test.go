package supervisor

import (
	"context"
	"testing"

	"YoudaoNoteLm/internal/agentharness/core"
)

// mockStore 实现 core.RunStore 接口用于测试。
type mockStore struct {
	runs     map[core.RunID]core.Run
	attempts map[core.RunID][]core.Attempt
	steps    map[core.StepID]core.Step
}

func newMockStore() *mockStore {
	return &mockStore{
		runs:     make(map[core.RunID]core.Run),
		attempts: make(map[core.RunID][]core.Attempt),
		steps:    make(map[core.StepID]core.Step),
	}
}

func (s *mockStore) CreateQueued(_ context.Context, run core.Run) error {
	if _, exists := s.runs[run.ID]; exists {
		return core.ErrRunAlreadyExists
	}
	s.runs[run.ID] = run
	return nil
}

func (s *mockStore) Get(_ context.Context, id core.RunID) (core.Run, error) {
	run, exists := s.runs[id]
	if !exists {
		return core.Run{}, core.ErrRunNotFound
	}
	return run, nil
}

func (s *mockStore) Claim(_ context.Context, id core.RunID, workerID string) (core.Run, core.Attempt, error) {
	run, exists := s.runs[id]
	if !exists {
		return core.Run{}, core.Attempt{}, core.ErrRunNotFound
	}
	if run.State != core.RunStateQueued {
		return core.Run{}, core.Attempt{}, core.ErrNotQueued
	}

	attempt := core.Attempt{
		ID:            "attempt-1",
		RunID:         id,
		AttemptNumber: 1,
		WorkerID:      workerID,
		FencingToken:  1,
		State:         core.AttemptStateRunning,
	}
	s.attempts[id] = append(s.attempts[id], attempt)

	run.State = core.RunStateRunning
	run.StateVersion = 2
	run.Authority = &core.ExecutionAuthority{
		AttemptID:       attempt.ID,
		FencingToken:    attempt.FencingToken,
		RunStateVersion: run.StateVersion,
	}
	s.runs[id] = run

	return run, attempt, nil
}

func (s *mockStore) Transition(_ context.Context, req core.TransitionRequest) (core.Run, error) {
	run, exists := s.runs[req.RunID]
	if !exists {
		return core.Run{}, core.ErrRunNotFound
	}
	if run.State != req.CurrentState {
		return core.Run{}, core.ErrAuthorityStale
	}
	if run.StateVersion != req.StateVersion {
		return core.Run{}, core.ErrAuthorityStale
	}

	run.State = req.TargetState
	run.StateVersion++
	s.runs[req.RunID] = run
	return run, nil
}

func (s *mockStore) CreateStep(_ context.Context, step core.Step) error {
	if _, exists := s.steps[step.ID]; exists {
		return core.ErrStepAlreadyExists
	}
	s.steps[step.ID] = step
	return nil
}

func (s *mockStore) FinishStep(_ context.Context, req core.FinishStepRequest) (core.Step, error) {
	step, exists := s.steps[req.StepID]
	if !exists {
		return core.Step{}, core.ErrStepNotFound
	}
	step.State = req.State
	s.steps[req.StepID] = step
	return step, nil
}

func TestExecutionSupervisor_Execute_Success(t *testing.T) {
	store := newMockStore()
	finalization := &FakeFinalizationPort{}
	agent := &MockEinoAgent{
		Events: []AgentEvent{
			{Type: "text", Content: "Hello"},
		},
	}

	supervisor := NewExecutionSupervisor(store, finalization, agent)

	// 创建并 Claim Run
	run := core.Run{
		ID:        "run-1",
		AgentType: "chat",
		UserID:    1,
		Input: core.InputRef{
			Kind: "chat_message",
			Ref:  "hello",
		},
		State:    core.RunStateQueued,
		StateVersion: 1,
	}
	if err := store.CreateQueued(context.Background(), run); err != nil {
		t.Fatalf("CreateQueued failed: %v", err)
	}
	if _, _, err := store.Claim(context.Background(), "run-1", "worker-1"); err != nil {
		t.Fatalf("Claim failed: %v", err)
	}

	// 执行
	err := supervisor.Execute(context.Background(), "run-1", "worker-1")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// 验证 Finalization 被调用
	if finalization.Calls != 1 {
		t.Errorf("expected 1 finalize call, got %d", finalization.Calls)
	}
	if finalization.LastRequest == nil {
		t.Fatal("expected LastRequest to be set")
	}
	if finalization.LastRequest.Outcome.Status != core.OutcomeStatusSuccess {
		t.Errorf("expected success outcome, got %s", finalization.LastRequest.Outcome.Status)
	}
}

func TestExecutionSupervisor_Execute_AgentError(t *testing.T) {
	store := newMockStore()
	finalization := &FakeFinalizationPort{}
	agent := &MockEinoAgent{
		Error: context.DeadlineExceeded,
	}

	supervisor := NewExecutionSupervisor(store, finalization, agent)

	// 创建并 Claim Run
	run := core.Run{
		ID:        "run-1",
		AgentType: "chat",
		UserID:    1,
		Input: core.InputRef{
			Kind: "chat_message",
			Ref:  "hello",
		},
		State:    core.RunStateQueued,
		StateVersion: 1,
	}
	if err := store.CreateQueued(context.Background(), run); err != nil {
		t.Fatalf("CreateQueued failed: %v", err)
	}
	if _, _, err := store.Claim(context.Background(), "run-1", "worker-1"); err != nil {
		t.Fatalf("Claim failed: %v", err)
	}

	// 执行
	err := supervisor.Execute(context.Background(), "run-1", "worker-1")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// 验证 Finalization 被调用，且结果为 failed
	if finalization.Calls != 1 {
		t.Errorf("expected 1 finalize call, got %d", finalization.Calls)
	}
	if finalization.LastRequest.Outcome.Status != core.OutcomeStatusFailed {
		t.Errorf("expected failed outcome, got %s", finalization.LastRequest.Outcome.Status)
	}
}

func TestExecutionSupervisor_Execute_InvalidState(t *testing.T) {
	store := newMockStore()
	finalization := &FakeFinalizationPort{}
	agent := &MockEinoAgent{}

	supervisor := NewExecutionSupervisor(store, finalization, agent)

	// 创建 Run 但不 Claim（保持 queued 状态）
	run := core.Run{
		ID:        "run-1",
		AgentType: "chat",
		UserID:    1,
		Input: core.InputRef{
			Kind: "chat_message",
			Ref:  "hello",
		},
		State:    core.RunStateQueued,
		StateVersion: 1,
	}
	if err := store.CreateQueued(context.Background(), run); err != nil {
		t.Fatalf("CreateQueued failed: %v", err)
	}

	// 执行应该失败（状态不是 running）
	err := supervisor.Execute(context.Background(), "run-1", "worker-1")
	if err == nil {
		t.Error("expected error for non-running state")
	}
}
