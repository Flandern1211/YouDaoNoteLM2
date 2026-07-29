// Package events 实现事件日志和 SSE 重放。
package events

// EventType 语义事件类型。
type EventType string

const (
	// Run 生命周期事件
	EventRunAccepted    EventType = "run.accepted"
	EventRunQueued      EventType = "run.queued"
	EventRunClaimed     EventType = "run.claimed"
	EventRunStateChanged EventType = "run.state_changed"
	EventRunFinalizing  EventType = "run.finalizing"
	EventRunCompleted   EventType = "run.completed"
	EventRunFailed      EventType = "run.failed"
	EventRunCancelled   EventType = "run.cancelled"
	EventRunError       EventType = "run.error"
	EventRunSuspended   EventType = "run.suspended"

	// Attempt 生命周期事件
	EventAttemptStarted  EventType = "attempt.started"
	EventAttemptFinished EventType = "attempt.finished"

	// Step 生命周期事件
	EventStepStarted  EventType = "step.started"
	EventStepFinished EventType = "step.finished"
)

// RunEvent 语义事件信封（与 H2 的 agent_run_events 表对应）。
type RunEvent struct {
	ID              string     `json:"id"`
	RunID           string     `json:"run_id"`
	Sequence        uint64     `json:"sequence"`
	EventID         string     `json:"event_id"`
	AttemptID       *string    `json:"attempt_id,omitempty"`
	StepID          *string    `json:"step_id,omitempty"`
	EventType       EventType  `json:"event_type"`
	PayloadVersion  uint       `json:"payload_version"`
	Payload         EventPayload `json:"payload"`
	CreatedAt       int64      `json:"created_at"`
}

// EventPayload 事件负载（脱敏）。
type EventPayload struct {
	// 通用字段
	State     string `json:"state,omitempty"`
	ErrorClass string `json:"error_class,omitempty"`
	ErrorCode  string `json:"error_code,omitempty"`
	Timestamp  int64  `json:"timestamp,omitempty"`

	// Run 相关
	AgentType   string `json:"agent_type,omitempty"`
	UserID      uint   `json:"user_id,omitempty"`
	RunID       string `json:"run_id,omitempty"`

	// Attempt 相关
	AttemptID     string `json:"attempt_id,omitempty"`
	AttemptNumber uint   `json:"attempt_number,omitempty"`
	WorkerID      string `json:"worker_id,omitempty"`

	// Step 相关
	StepID      string `json:"step_id,omitempty"`
	StepKind    string `json:"step_kind,omitempty"`
	AgentName   string `json:"agent_name,omitempty"`

	// 结果相关
	MessageID       string `json:"message_id,omitempty"`
	ArtifactRef     string `json:"artifact_ref,omitempty"`
	Summary         string `json:"summary,omitempty"`
}

// RunStateInfo Run 状态信息（用于 GetRun 响应）。
type RunStateInfo struct {
	RunID          string  `json:"run_id"`
	State          string  `json:"state"`
	AttemptID      *string `json:"attempt_id,omitempty"`
	WorkerID       *string `json:"worker_id,omitempty"`
	StartedAt      *int64  `json:"started_at,omitempty"`
	FinishedAt     *int64  `json:"finished_at,omitempty"`
	ErrorClass     *string `json:"error_class,omitempty"`
	ErrorCode      *string `json:"error_code,omitempty"`
	MessageID      *string `json:"message_id,omitempty"`
	Sequence       uint64  `json:"sequence"`
}
