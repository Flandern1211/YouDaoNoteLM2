package finalization

import (
	"testing"

	"YoudaoNoteLm/internal/agentharness/core"
)

// mockFinalizationStore 实现 FinalizationStore 接口。
type mockFinalizationStore struct {
	journalEntries map[string]WritebackJournal
	finalizeCalls  int
}

func newMockFinalizationStore() *mockFinalizationStore {
	return &mockFinalizationStore{
		journalEntries: make(map[string]WritebackJournal),
	}
}

func (s *mockFinalizationStore) CreateJournalEntry(entry WritebackJournal) error {
	s.journalEntries[entry.ID] = entry
	return nil
}

func (s *mockFinalizationStore) GetJournalEntry(id string) (WritebackJournal, error) {
	entry, exists := s.journalEntries[id]
	if !exists {
		return WritebackJournal{}, nil
	}
	return entry, nil
}

func (s *mockFinalizationStore) UpdateJournalEntry(entry WritebackJournal) error {
	s.journalEntries[entry.ID] = entry
	return nil
}

func (s *mockFinalizationStore) FinalizeRun(runID string, authority core.ExecutionAuthority, expectedVersion core.StateVersion, newState core.RunState) (core.StateVersion, error) {
	s.finalizeCalls++
	return expectedVersion + 1, nil
}

// mockMessageWriter 实现 MessageWriter 接口。
type mockMessageWriter struct {
	calls int
}

func (w *mockMessageWriter) WriteAssistantMessage(runID string, content string, idempotencyKey string) (string, error) {
	w.calls++
	return "msg-1", nil
}

func TestFinalizationPort_Finalize_Success(t *testing.T) {
	store := newMockFinalizationStore()
	writer := &mockMessageWriter{}
	finalizer := NewFinalizationPort(store, writer)

	req := core.FinalizationRequest{
		RunID: "run-1",
		Authority: core.ExecutionAuthority{
			AttemptID:       "attempt-1",
			FencingToken:    1,
			RunStateVersion: 2,
		},
		FinalizingStateVersion: 2,
		Revision:               1,
		Outcome: core.Outcome{
			Status: core.OutcomeStatusSuccess,
		},
	}

	result, err := finalizer.Finalize(req)
	if err != nil {
		t.Fatalf("Finalize failed: %v", err)
	}

	if result.NewState != core.RunStateSucceeded {
		t.Errorf("expected state 'succeeded', got '%s'", result.NewState)
	}
	if result.NewVersion != 3 {
		t.Errorf("expected version 3, got %d", result.NewVersion)
	}
	if store.finalizeCalls != 1 {
		t.Errorf("expected 1 finalize call, got %d", store.finalizeCalls)
	}
	if writer.calls != 1 {
		t.Errorf("expected 1 write call, got %d", writer.calls)
	}
}

func TestFinalizationPort_Finalize_Failed(t *testing.T) {
	store := newMockFinalizationStore()
	writer := &mockMessageWriter{}
	finalizer := NewFinalizationPort(store, writer)

	ec := core.ErrorClassPermanent
	req := core.FinalizationRequest{
		RunID: "run-1",
		Authority: core.ExecutionAuthority{
			AttemptID:       "attempt-1",
			FencingToken:    1,
			RunStateVersion: 2,
		},
		FinalizingStateVersion: 2,
		Revision:               1,
		Outcome: core.Outcome{
			Status:     core.OutcomeStatusFailed,
			ErrorClass: &ec,
		},
	}

	result, err := finalizer.Finalize(req)
	if err != nil {
		t.Fatalf("Finalize failed: %v", err)
	}

	if result.NewState != core.RunStateFailed {
		t.Errorf("expected state 'failed', got '%s'", result.NewState)
	}
	// Failed 不写入 assistant message
	if writer.calls != 0 {
		t.Errorf("expected 0 write calls, got %d", writer.calls)
	}
}

func TestFinalizationPort_Finalize_Cancelled(t *testing.T) {
	store := newMockFinalizationStore()
	writer := &mockMessageWriter{}
	finalizer := NewFinalizationPort(store, writer)

	req := core.FinalizationRequest{
		RunID: "run-1",
		Authority: core.ExecutionAuthority{
			AttemptID:       "attempt-1",
			FencingToken:    1,
			RunStateVersion: 2,
		},
		FinalizingStateVersion: 2,
		Revision:               1,
		Outcome: core.Outcome{
			Status: core.OutcomeStatusCancelled,
		},
	}

	result, err := finalizer.Finalize(req)
	if err != nil {
		t.Fatalf("Finalize failed: %v", err)
	}

	if result.NewState != core.RunStateCancelled {
		t.Errorf("expected state 'cancelled', got '%s'", result.NewState)
	}
}
