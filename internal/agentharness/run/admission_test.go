package run

import (
	"context"
	"testing"

	"YoudaoNoteLm/internal/agentharness/core"
)

// mockAdmissionStore 实现 core.AdmissionStore 接口用于测试。
type mockAdmissionStore struct {
	*mockStore
	accepted []core.AcceptRequest
}

func newMockAdmissionStore() *mockAdmissionStore {
	return &mockAdmissionStore{
		mockStore: newMockStore(),
		accepted:  make([]core.AcceptRequest, 0),
	}
}

func (s *mockAdmissionStore) Accept(ctx context.Context, req core.AcceptRequest) (core.AcceptedRun, error) {
	s.accepted = append(s.accepted, req)

	// 模拟幂等检查
	for _, existing := range s.accepted[:len(s.accepted)-1] {
		if existing.IdempotencyKey == req.IdempotencyKey && existing.UserID == req.UserID {
			// 幂等命中
			return core.AcceptedRun{
				RunID:              "existing-run",
				State:              core.RunStateQueued,
				Sequence:           1,
				IsIdempotentReplay: true,
			}, nil
		}
	}

	// 创建新 Run
	runID := "run-" + req.IdempotencyKey
	run := core.Run{
		ID:        core.RunID(runID),
		AgentType: req.AgentType,
		UserID:    req.UserID,
		Input:     req.Input,
		State:     core.RunStateQueued,
	}
	if err := s.mockStore.CreateQueued(ctx, run); err != nil {
		return core.AcceptedRun{}, err
	}

	return core.AcceptedRun{
		RunID:              core.RunID(runID),
		MessageID:          "msg-1",
		State:              core.RunStateQueued,
		Sequence:           1,
		IsIdempotentReplay: false,
	}, nil
}

func TestAdmissionService_Accept(t *testing.T) {
	store := newMockAdmissionStore()
	svc := NewAdmissionService(store)

	req := core.AcceptRequest{
		UserID:      1,
		AgentType:   "chat",
		IdempotencyKey: "key-1",
		Input: core.InputRef{
			Kind: "chat_message",
			Ref:  "hello",
			Hash: "hash-1",
		},
	}

	result, err := svc.Accept(context.Background(), req)
	if err != nil {
		t.Fatalf("Accept failed: %v", err)
	}

	if result.IsIdempotentReplay {
		t.Error("expected IsIdempotentReplay=false for first request")
	}
	if result.State != core.RunStateQueued {
		t.Errorf("expected state 'queued', got '%s'", result.State)
	}
}

func TestAdmissionService_Accept_IdempotentReplay(t *testing.T) {
	store := newMockAdmissionStore()
	svc := NewAdmissionService(store)

	req := core.AcceptRequest{
		UserID:      1,
		AgentType:   "chat",
		IdempotencyKey: "key-1",
		Input: core.InputRef{
			Kind: "chat_message",
			Ref:  "hello",
			Hash: "hash-1",
		},
	}

	// 第一次请求
	_, err := svc.Accept(context.Background(), req)
	if err != nil {
		t.Fatalf("first Accept failed: %v", err)
	}

	// 第二次请求（相同幂等键）
	result, err := svc.Accept(context.Background(), req)
	if err != nil {
		t.Fatalf("second Accept failed: %v", err)
	}

	if !result.IsIdempotentReplay {
		t.Error("expected IsIdempotentReplay=true for idempotent replay")
	}
}

func TestAdmissionService_Accept_AutoGenerateIdempotencyKey(t *testing.T) {
	store := newMockAdmissionStore()
	svc := NewAdmissionService(store)

	req := core.AcceptRequest{
		UserID:    1,
		AgentType: "chat",
		Input: core.InputRef{
			Kind: "chat_message",
			Ref:  "hello",
		},
		// 不提供 IdempotencyKey
	}

	result, err := svc.Accept(context.Background(), req)
	if err != nil {
		t.Fatalf("Accept failed: %v", err)
	}

	if result.IsIdempotentReplay {
		t.Error("expected IsIdempotentReplay=false for first request")
	}
}

func TestAdmissionService_Accept_Validation(t *testing.T) {
	store := newMockAdmissionStore()
	svc := NewAdmissionService(store)

	// 测试缺少 UserID
	_, err := svc.Accept(context.Background(), core.AcceptRequest{
		AgentType: "chat",
		Input:     core.InputRef{Kind: "chat_message", Ref: "hello"},
	})
	if err == nil {
		t.Error("expected error for missing UserID")
	}

	// 测试缺少 AgentType
	_, err = svc.Accept(context.Background(), core.AcceptRequest{
		UserID: 1,
		Input:  core.InputRef{Kind: "chat_message", Ref: "hello"},
	})
	if err == nil {
		t.Error("expected error for missing AgentType")
	}

	// 测试缺少 Input.Kind
	_, err = svc.Accept(context.Background(), core.AcceptRequest{
		UserID:    1,
		AgentType: "chat",
		Input:     core.InputRef{Ref: "hello"},
	})
	if err == nil {
		t.Error("expected error for missing Input.Kind")
	}
}
