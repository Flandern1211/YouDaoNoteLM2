package events

import (
	"context"
	"testing"
)

// mockEventStore 实现 EventStore 接口。
type mockEventStore struct {
	events map[string][]RunEvent
	runs   map[string]RunStateInfo
}

func newMockEventStore() *mockEventStore {
	return &mockEventStore{
		events: make(map[string][]RunEvent),
		runs:   make(map[string]RunStateInfo),
	}
}

func (s *mockEventStore) AppendEvent(ctx context.Context, event RunEvent) error {
	s.events[event.RunID] = append(s.events[event.RunID], event)
	return nil
}

func (s *mockEventStore) GetRun(ctx context.Context, runID string) (RunStateInfo, error) {
	run, exists := s.runs[runID]
	if !exists {
		return RunStateInfo{}, nil
	}
	return run, nil
}

func (s *mockEventStore) ListEvents(ctx context.Context, runID string, afterSeq uint64, limit int) ([]RunEvent, error) {
	events := s.events[runID]
	var result []RunEvent
	for _, event := range events {
		if event.Sequence > afterSeq {
			result = append(result, event)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (s *mockEventStore) GetLastSequence(ctx context.Context, runID string) (uint64, error) {
	events := s.events[runID]
	if len(events) == 0 {
		return 0, nil
	}
	return events[len(events)-1].Sequence, nil
}

func TestEventStore_AppendAndList(t *testing.T) {
	store := newMockEventStore()
	ctx := context.Background()

	// 追加事件
	event1 := RunEvent{
		ID:        "event-1",
		RunID:     "run-1",
		Sequence:  1,
		EventID:   "evt-1",
		EventType: EventRunAccepted,
	}
	event2 := RunEvent{
		ID:        "event-2",
		RunID:     "run-1",
		Sequence:  2,
		EventID:   "evt-2",
		EventType: EventRunQueued,
	}

	if err := store.AppendEvent(ctx, event1); err != nil {
		t.Fatalf("AppendEvent failed: %v", err)
	}
	if err := store.AppendEvent(ctx, event2); err != nil {
		t.Fatalf("AppendEvent failed: %v", err)
	}

	// 列出事件
	events, err := store.ListEvents(ctx, "run-1", 0, 10)
	if err != nil {
		t.Fatalf("ListEvents failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].EventType != EventRunAccepted {
		t.Errorf("expected event type 'run.accepted', got '%s'", events[0].EventType)
	}
	if events[1].EventType != EventRunQueued {
		t.Errorf("expected event type 'run.queued', got '%s'", events[1].EventType)
	}

	// 使用 after_seq
	events, err = store.ListEvents(ctx, "run-1", 1, 10)
	if err != nil {
		t.Fatalf("ListEvents failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Sequence != 2 {
		t.Errorf("expected sequence 2, got %d", events[0].Sequence)
	}
}

func TestEventNotifier_SubscribeAndNotify(t *testing.T) {
	notifier := NewMemoryNotifier()

	// 订阅
	ch, err := notifier.Subscribe("run-1")
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// 通知
	event := RunEvent{
		RunID:     "run-1",
		Sequence:  1,
		EventType: EventRunAccepted,
	}
	notifier.Notify(event)

	// 接收
	received := <-ch
	if received.EventType != EventRunAccepted {
		t.Errorf("expected event type 'run.accepted', got '%s'", received.EventType)
	}

	// 取消订阅
	notifier.Unsubscribe("run-1", ch)
}

func TestEventNotifier_MultipleSubscribers(t *testing.T) {
	notifier := NewMemoryNotifier()

	// 多个订阅者
	ch1, _ := notifier.Subscribe("run-1")
	ch2, _ := notifier.Subscribe("run-1")

	// 通知
	event := RunEvent{
		RunID:     "run-1",
		Sequence:  1,
		EventType: EventRunAccepted,
	}
	notifier.Notify(event)

	// 两个订阅者都应该收到
	e1 := <-ch1
	e2 := <-ch2
	if e1.EventType != EventRunAccepted {
		t.Errorf("subscriber 1: expected 'run.accepted', got '%s'", e1.EventType)
	}
	if e2.EventType != EventRunAccepted {
		t.Errorf("subscriber 2: expected 'run.accepted', got '%s'", e2.EventType)
	}
}
