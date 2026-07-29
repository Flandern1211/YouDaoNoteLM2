package supervisor

import (
	"context"
)

// MockEinoAgent 用于测试的假 EinoAgent 实现。
type MockEinoAgent struct {
	// Events 返回的事件（可选）。
	Events []AgentEvent
	// Error 返回的错误（可选）。
	Error error
	// Calls 记录调用次数。
	Calls int
}

// Execute 实现 EinoAgent 接口。
func (m *MockEinoAgent) Execute(ctx context.Context, input AgentInput) (<-chan AgentEvent, error) {
	m.Calls++

	if m.Error != nil {
		return nil, m.Error
	}

	eventCh := make(chan AgentEvent, len(m.Events))
	for _, event := range m.Events {
		eventCh <- event
	}
	close(eventCh)

	return eventCh, nil
}
