package run

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

	// 创建 Attempt
	attempt := core.Attempt{
		ID:            "attempt-1",
		RunID:         id,
		AttemptNumber: 1,
		WorkerID:      workerID,
		FencingToken:  1,
		State:         core.AttemptStateRunning,
	}
	s.attempts[id] = append(s.attempts[id], attempt)

	// 更新 Run
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
	if run.Authority != nil {
		run.Authority.RunStateVersion = run.StateVersion
	}
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
	if req.ErrorClass != nil {
		step.ErrorClass = req.ErrorClass
	}
	if req.ErrorCode != nil {
		step.ErrorCode = req.ErrorCode
	}
	if req.ResultArtifactRef != nil {
		step.ResultArtifactRef = req.ResultArtifactRef
	}
	s.steps[req.StepID] = step
	return step, nil
}

func TestService_CreateQueued(t *testing.T) {
	store := newMockStore()
	service := NewService(store)

	run := core.Run{
		ID:        "run-1",
		AgentType: "chat",
		UserID:    1,
		Input: core.InputRef{
			Kind: "chat_message",
			Ref:  "msg-1",
			Hash: "hash-1",
		},
		State:    core.RunStateQueued,
		StateVersion: 1,
	}

	err := service.CreateQueued(context.Background(), run)
	if err != nil {
		t.Fatalf("CreateQueued failed: %v", err)
	}

	// 验证 Run 已创建
	saved, err := service.Get(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if saved.ID != "run-1" {
		t.Errorf("expected run ID 'run-1', got '%s'", saved.ID)
	}
	if saved.State != core.RunStateQueued {
		t.Errorf("expected state 'queued', got '%s'", saved.State)
	}
}

func TestService_CreateQueued_InvalidState(t *testing.T) {
	store := newMockStore()
	service := NewService(store)

	run := core.Run{
		ID:        "run-1",
		AgentType: "chat",
		UserID:    1,
		Input: core.InputRef{
			Kind: "chat_message",
			Ref:  "msg-1",
			Hash: "hash-1",
		},
		State:    core.RunStateRunning, // 非法状态
		StateVersion: 1,
	}

	err := service.CreateQueued(context.Background(), run)
	if err != core.ErrInvalidTransition {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestService_Claim(t *testing.T) {
	store := newMockStore()
	service := NewService(store)

	// 先创建 Run
	run := core.Run{
		ID:        "run-1",
		AgentType: "chat",
		UserID:    1,
		Input: core.InputRef{
			Kind: "chat_message",
			Ref:  "msg-1",
			Hash: "hash-1",
		},
		State:    core.RunStateQueued,
		StateVersion: 1,
	}
	if err := service.CreateQueued(context.Background(), run); err != nil {
		t.Fatalf("CreateQueued failed: %v", err)
	}

	// Claim
	claimedRun, attempt, err := service.Claim(context.Background(), "run-1", "worker-1")
	if err != nil {
		t.Fatalf("Claim failed: %v", err)
	}

	// 验证 Run 状态
	if claimedRun.State != core.RunStateRunning {
		t.Errorf("expected state 'running', got '%s'", claimedRun.State)
	}
	if claimedRun.Authority == nil {
		t.Fatal("expected authority to be set")
	}
	if claimedRun.Authority.FencingToken != 1 {
		t.Errorf("expected fencing token 1, got %d", claimedRun.Authority.FencingToken)
	}
	if claimedRun.Authority.AttemptID != "attempt-1" {
		t.Errorf("expected attempt ID 'attempt-1', got '%s'", claimedRun.Authority.AttemptID)
	}

	// 验证 Attempt
	if attempt.RunID != "run-1" {
		t.Errorf("expected attempt run ID 'run-1', got '%s'", attempt.RunID)
	}
	if attempt.WorkerID != "worker-1" {
		t.Errorf("expected worker ID 'worker-1', got '%s'", attempt.WorkerID)
	}
}

func TestService_Transition(t *testing.T) {
	store := newMockStore()
	service := NewService(store)

	// 创建并 Claim Run
	run := core.Run{
		ID:        "run-1",
		AgentType: "chat",
		UserID:    1,
		Input: core.InputRef{
			Kind: "chat_message",
			Ref:  "msg-1",
			Hash: "hash-1",
		},
		State:    core.RunStateQueued,
		StateVersion: 1,
	}
	if err := service.CreateQueued(context.Background(), run); err != nil {
		t.Fatalf("CreateQueued failed: %v", err)
	}
	if _, _, err := service.Claim(context.Background(), "run-1", "worker-1"); err != nil {
		t.Fatalf("Claim failed: %v", err)
	}

	// 转换到 finalizing
	transitionReq := core.TransitionRequest{
		RunID:        "run-1",
		CurrentState: core.RunStateRunning,
		TargetState:  core.RunStateFinalizing,
		StateVersion: 2,
		FencingToken: 1,
	}
	transitioned, err := service.Transition(context.Background(), transitionReq)
	if err != nil {
		t.Fatalf("Transition failed: %v", err)
	}
	if transitioned.State != core.RunStateFinalizing {
		t.Errorf("expected state 'finalizing', got '%s'", transitioned.State)
	}
}

func TestService_Transition_InvalidTransition(t *testing.T) {
	store := newMockStore()
	service := NewService(store)

	// 创建并 Claim Run
	run := core.Run{
		ID:        "run-1",
		AgentType: "chat",
		UserID:    1,
		Input: core.InputRef{
			Kind: "chat_message",
			Ref:  "msg-1",
			Hash: "hash-1",
		},
		State:    core.RunStateQueued,
		StateVersion: 1,
	}
	if err := service.CreateQueued(context.Background(), run); err != nil {
		t.Fatalf("CreateQueued failed: %v", err)
	}
	if _, _, err := service.Claim(context.Background(), "run-1", "worker-1"); err != nil {
		t.Fatalf("Claim failed: %v", err)
	}

	// 尝试非法转换 running -> succeeded
	transitionReq := core.TransitionRequest{
		RunID:        "run-1",
		CurrentState: core.RunStateRunning,
		TargetState:  core.RunStateSucceeded,
		StateVersion: 2,
		FencingToken: 1,
	}
	_, err := service.Transition(context.Background(), transitionReq)
	if err == nil {
		t.Error("expected error for invalid transition, got nil")
	}
}

func TestService_CreateStep(t *testing.T) {
	store := newMockStore()
	service := NewService(store)

	step := core.Step{
		ID:        "step-1",
		RunID:     "run-1",
		AttemptID: "attempt-1",
		Kind:      core.StepKindSearch,
		AgentName: "search-agent",
		State:     core.StepStateRunning,
		StartedAt: 1000,
	}

	err := service.CreateStep(context.Background(), step)
	if err != nil {
		t.Fatalf("CreateStep failed: %v", err)
	}

	// 验证 Step 已创建
	saved := store.steps["step-1"]
	if saved.ID != "step-1" {
		t.Errorf("expected step ID 'step-1', got '%s'", saved.ID)
	}
	if saved.Kind != core.StepKindSearch {
		t.Errorf("expected kind 'search', got '%s'", saved.Kind)
	}
}

func TestService_FinishStep(t *testing.T) {
	store := newMockStore()
	service := NewService(store)

	// 先创建 Step
	step := core.Step{
		ID:        "step-1",
		RunID:     "run-1",
		AttemptID: "attempt-1",
		Kind:      core.StepKindSearch,
		AgentName: "search-agent",
		State:     core.StepStateRunning,
		StartedAt: 1000,
	}
	if err := service.CreateStep(context.Background(), step); err != nil {
		t.Fatalf("CreateStep failed: %v", err)
	}

	// 完成 Step
	finishReq := core.FinishStepRequest{
		StepID:       "step-1",
		RunID:        "run-1",
		AttemptID:    "attempt-1",
		FencingToken: 0,
		State:        core.StepStateCompleted,
	}
	finished, err := service.FinishStep(context.Background(), finishReq)
	if err != nil {
		t.Fatalf("FinishStep failed: %v", err)
	}
	if finished.State != core.StepStateCompleted {
		t.Errorf("expected state 'completed', got '%s'", finished.State)
	}
}
