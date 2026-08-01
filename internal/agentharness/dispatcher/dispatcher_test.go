package dispatcher

import (
	"context"
	"testing"
	"time"
)

// mockDispatcherStore 实现 DispatcherStore 接口。
type mockDispatcherStore struct {
	queuedRuns []QueuedRun
	claimed    map[string]bool
	cancelled  map[string]bool
}

func newMockStore() *mockDispatcherStore {
	return &mockDispatcherStore{
		claimed:   make(map[string]bool),
		cancelled: make(map[string]bool),
	}
}

func (s *mockDispatcherStore) GetQueuedRuns(ctx context.Context, limit int) ([]QueuedRun, error) {
	var result []QueuedRun
	for _, run := range s.queuedRuns {
		if !s.claimed[run.RunID] {
			result = append(result, run)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (s *mockDispatcherStore) ClaimRun(ctx context.Context, runID string, workerID string) error {
	s.claimed[runID] = true
	return nil
}

func (s *mockDispatcherStore) CheckCancel(ctx context.Context, runID string) (bool, error) {
	return s.cancelled[runID], nil
}

// mockExecutor 实现 Executor 接口。
type mockExecutor struct {
	executed []string
}

func (e *mockExecutor) Execute(ctx context.Context, runID string, workerID string) error {
	e.executed = append(e.executed, runID)
	return nil
}

func TestDispatcher_ProcessRun(t *testing.T) {
	store := newMockStore()
	executor := &mockExecutor{}
	config := DefaultConfig()
	config.WorkerID = "test-worker"

	store.queuedRuns = []QueuedRun{
		{ID: "qr-1", RunID: "run-1", State: RunStateQueued},
	}

	d := NewDispatcher(config, store, executor)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	d.poll(ctx)

	if len(executor.executed) != 1 {
		t.Fatalf("expected 1 execution, got %d", len(executor.executed))
	}
	if executor.executed[0] != "run-1" {
		t.Errorf("expected run-1, got %s", executor.executed[0])
	}
}

func TestDispatcher_SkipCancelled(t *testing.T) {
	store := newMockStore()
	executor := &mockExecutor{}
	config := DefaultConfig()

	store.queuedRuns = []QueuedRun{
		{ID: "qr-1", RunID: "run-1", State: RunStateQueued},
	}
	store.cancelled["run-1"] = true

	d := NewDispatcher(config, store, executor)
	ctx := context.Background()

	d.poll(ctx)

	if len(executor.executed) != 0 {
		t.Errorf("expected 0 executions, got %d", len(executor.executed))
	}
}

func TestDispatcher_APIRoleNoPoll(t *testing.T) {
	store := newMockStore()
	executor := &mockExecutor{}
	config := DefaultConfig()
	config.Role = RoleAPI

	store.queuedRuns = []QueuedRun{
		{ID: "qr-1", RunID: "run-1", State: RunStateQueued},
	}

	d := NewDispatcher(config, store, executor)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	d.Start(ctx)
	<-ctx.Done()

	if len(executor.executed) != 0 {
		t.Errorf("API role should not execute, got %d", len(executor.executed))
	}
}
