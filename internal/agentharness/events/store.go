package events

import (
	"context"
	"time"
)

// EventStore 事件存储接口。
type EventStore interface {
	// AppendEvent 追加事件（在同一事务中与状态转换一起写入）。
	AppendEvent(ctx context.Context, event RunEvent) error

	// GetRun 获取 Run 当前状态。
	GetRun(ctx context.Context, runID string) (RunStateInfo, error)

	// ListEvents 列出事件（支持 after_seq 游标分页）。
	ListEvents(ctx context.Context, runID string, afterSeq uint64, limit int) ([]RunEvent, error)

	// GetLastSequence 获取 Run 的最后一个事件序号。
	GetLastSequence(ctx context.Context, runID string) (uint64, error)
}

// EventStoreGorm GORM 实现的 EventStore。
type EventStoreGorm struct {
	// db 数据库连接（实际实现中注入 gorm.DB）
}

// NewEventStoreGorm 创建 EventStoreGorm。
func NewEventStoreGorm() *EventStoreGorm {
	return &EventStoreGorm{}
}

// AppendEvent 追加事件。
func (s *EventStoreGorm) AppendEvent(ctx context.Context, event RunEvent) error {
	// 实际实现中写入 agent_run_events 表
	// 这里提供接口骨架
	return nil
}

// GetRun 获取 Run 当前状态。
func (s *EventStoreGorm) GetRun(ctx context.Context, runID string) (RunStateInfo, error) {
	// 实际实现中查询 agent_runs 表
	return RunStateInfo{}, nil
}

// ListEvents 列出事件。
func (s *EventStoreGorm) ListEvents(ctx context.Context, runID string, afterSeq uint64, limit int) ([]RunEvent, error) {
	// 实际实现中查询 agent_run_events 表
	return nil, nil
}

// GetLastSequence 获取最后序号。
func (s *EventStoreGorm) GetLastSequence(ctx context.Context, runID string) (uint64, error) {
	// 实际实现中查询 MAX(sequence)
	return 0, nil
}

// EventNotifier 事件通知器（用于 SSE）。
type EventNotifier interface {
	// Subscribe 订阅 Run 的事件。
	Subscribe(runID string) (<-chan RunEvent, error)

	// Unsubscribe 取消订阅。
	Unsubscribe(runID string, ch <-chan RunEvent)

	// Notify 通知事件。
	Notify(event RunEvent)
}

// MemoryNotifier 内存实现的事件通知器。
type MemoryNotifier struct {
	subscribers map[string][]chan RunEvent
}

// NewMemoryNotifier 创建 MemoryNotifier。
func NewMemoryNotifier() *MemoryNotifier {
	return &MemoryNotifier{
		subscribers: make(map[string][]chan RunEvent),
	}
}

// Subscribe 订阅事件。
func (n *MemoryNotifier) Subscribe(runID string) (<-chan RunEvent, error) {
	ch := make(chan RunEvent, 100)
	n.subscribers[runID] = append(n.subscribers[runID], ch)
	return ch, nil
}

// Unsubscribe 取消订阅。
func (n *MemoryNotifier) Unsubscribe(runID string, ch <-chan RunEvent) {
	subs := n.subscribers[runID]
	for i, sub := range subs {
		if sub == ch {
			n.subscribers[runID] = append(subs[:i], subs[i+1:]...)
			close(sub)
			break
		}
	}
}

// Notify 通知事件。
func (n *MemoryNotifier) Notify(event RunEvent) {
	subs := n.subscribers[event.RunID]
	for _, ch := range subs {
		select {
		case ch <- event:
		default:
			// 队列满，丢弃
		}
	}
}

// RetentionConfig 事件保留配置。
type RetentionConfig struct {
	// RetentionDays 保留天数。
	RetentionDays int
	// CleanupInterval 清理间隔。
	CleanupInterval time.Duration
}

// DefaultRetentionConfig 默认保留配置。
func DefaultRetentionConfig() RetentionConfig {
	return RetentionConfig{
		RetentionDays:   30,
		CleanupInterval: 24 * time.Hour,
	}
}
