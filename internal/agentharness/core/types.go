// Package core 定义 Agent Harness 的领域契约、纯状态机、端口与错误。
// 该包仅依赖 Go 标准库，不依赖 Eino、GORM、Redis、NATS 或 agentcontext。
package core

// RunID 标识一次用户意图的顶层持久化生命周期。
type RunID string

// AttemptID 标识某个 Worker 在一次执行权下的执行或恢复尝试。
type AttemptID string

// StepID 标识 Search Agent、重要工具或有外部副作用的边界。
type StepID string

// StateVersion 用于 CAS 操作的状态版本号。
type StateVersion uint64

// FencingToken 执行权代数，用于拒绝旧执行者的迟到写入。
type FencingToken uint64

// Revision 终态/写回 revision。
type Revision uint64

// ExecutionAuthority 执行权，只在 Worker claim 后有效。
type ExecutionAuthority struct {
	AttemptID       AttemptID
	FencingToken    FencingToken
	RunStateVersion StateVersion
}

// VersionSnapshot 在 Admission 时冻结的版本快照。
// 不包含 Prompt 正文、API key、用户消息、网页正文或可恢复的模型凭据。
type VersionSnapshot struct {
	AgentDefinitionVersion string
	PromptVersion          string
	ToolSchemaVersion      string
	ModelConfigHash        string
	ContextProfileVersion  string
	EinoVersion            string
}

// ErrorClass 统一错误分类。
type ErrorClass string

const (
	// ErrorClassPermanent 项目内部不可恢复错误。
	ErrorClassPermanent ErrorClass = "permanent"
	// ErrorClassTransient 瞬时错误，可重试。
	ErrorClassTransient ErrorClass = "transient"
	// ErrorClassRateLimited 限流错误。
	ErrorClassRateLimited ErrorClass = "rate_limited"
	// ErrorClassTimeout 超时错误。
	ErrorClassTimeout ErrorClass = "timeout"
	// ErrorClassCancelled 已取消。
	ErrorClassCancelled ErrorClass = "cancelled"
	// ErrorClassResourceExhausted 资源耗尽。
	ErrorClassResourceExhausted ErrorClass = "resource_exhausted"
	// ErrorClassInvalidInput 无效输入。
	ErrorClassInvalidInput ErrorClass = "invalid_input"
	// ErrorClassPermission 权限错误。
	ErrorClassPermission ErrorClass = "permission"
	// ErrorClassDependencyPermanent 已确认不会因重试改变结果的外部依赖错误。
	ErrorClassDependencyPermanent ErrorClass = "dependency_permanent"
	// ErrorClassWorkerLost Worker 丢失。
	ErrorClassWorkerLost ErrorClass = "worker_lost"
	// ErrorClassCheckpointIncompatible Checkpoint 不兼容。
	ErrorClassCheckpointIncompatible ErrorClass = "checkpoint_incompatible"
	// ErrorClassSideEffectUnknown 副作用未知。
	ErrorClassSideEffectUnknown ErrorClass = "side_effect_unknown"
)

// KnownErrorClasses 所有已知的错误分类。
var KnownErrorClasses = []ErrorClass{
	ErrorClassPermanent,
	ErrorClassTransient,
	ErrorClassRateLimited,
	ErrorClassTimeout,
	ErrorClassCancelled,
	ErrorClassResourceExhausted,
	ErrorClassInvalidInput,
	ErrorClassPermission,
	ErrorClassDependencyPermanent,
	ErrorClassWorkerLost,
	ErrorClassCheckpointIncompatible,
	ErrorClassSideEffectUnknown,
}

// IsKnownErrorClass 检查是否为已知的错误分类。
func IsKnownErrorClass(ec ErrorClass) bool {
	for _, known := range KnownErrorClasses {
		if ec == known {
			return true
		}
	}
	return false
}

// InputRef 引用入口输入且不复制正文。
type InputRef struct {
	Kind string // chat_message | search_task | external_reference
	Ref  string // 领域记录的稳定 ID
	Hash string // SHA-256，校验引用内容是否被意外替换
}

// RunState 运行状态。
type RunState string

const (
	RunStateQueued          RunState = "queued"
	RunStateRunning         RunState = "running"
	RunStateFinalizing      RunState = "finalizing"
	RunStateRetryWait       RunState = "retry_wait"
	RunStatePauseRequested  RunState = "pause_requested"
	RunStatePausing         RunState = "pausing"
	RunStatePaused          RunState = "paused"
	RunStateCancelRequested RunState = "cancel_requested"
	RunStateSucceeded       RunState = "succeeded"
	RunStateFailed          RunState = "failed"
	RunStateCancelled       RunState = "cancelled"
	RunStateSuspended       RunState = "suspended"
)

// DesiredState 用户意图的目标状态。
type DesiredState string

const (
	DesiredStateRunning   DesiredState = "running"
	DesiredStatePaused    DesiredState = "paused"
	DesiredStateCancelled DesiredState = "cancelled"
)

// Run 一次用户意图的顶层持久化生命周期。
type Run struct {
	ID              RunID
	ParentRunID     *RunID
	AgentType       string
	UserID          uint
	NotebookID      *uint
	ConversationID  *uint
	Input           InputRef
	VersionSnapshot VersionSnapshot
	State           RunState
	DesiredState    DesiredState
	StateVersion    StateVersion
	Revision        Revision
	Authority       *ExecutionAuthority
}

// Attempt 某个 Worker 在一次执行权下的执行或恢复尝试。
type Attempt struct {
	ID                  AttemptID
	RunID               RunID
	AttemptNumber       uint
	WorkerID            string
	FencingToken        FencingToken
	ResumeCheckpointRef *string
	TraceID             *string
	State               AttemptState
	ErrorClass          *ErrorClass
	ErrorCode           *string
	StartedAt           int64
	HeartbeatAt         *int64
	FinishedAt          *int64
}

// AttemptState 尝试状态。
type AttemptState string

const (
	AttemptStateRunning   AttemptState = "running"
	AttemptStateCompleted AttemptState = "completed"
	AttemptStateFailed    AttemptState = "failed"
	AttemptStateCancelled AttemptState = "cancelled"
)

// Step Search Agent、重要工具或有外部副作用的边界。
type Step struct {
	ID                StepID
	RunID             RunID
	AttemptID         AttemptID
	ParentStepID      *StepID
	Kind              StepKind
	AgentName         string
	State             StepState
	InputHash         *string
	ToolCallID        *string
	ResultArtifactRef *string
	FencingToken      FencingToken
	ErrorClass        *ErrorClass
	ErrorCode         *string
	StartedAt         int64
	FinishedAt        *int64
}

// StepKind 步骤类型。
type StepKind string

const (
	StepKindSearch      StepKind = "search"
	StepKindTool        StepKind = "tool"
	StepKindSubAgent    StepKind = "sub_agent"
	StepKindExternalJob StepKind = "external_job"
)

// StepState 步骤状态。
type StepState string

const (
	StepStateRunning   StepState = "running"
	StepStateCompleted StepState = "completed"
	StepStateFailed    StepState = "failed"
	StepStateCancelled StepState = "cancelled"
)

// IsTerminalRunState 检查是否为终态。
func IsTerminalRunState(state RunState) bool {
	return state == RunStateSucceeded ||
		state == RunStateFailed ||
		state == RunStateCancelled ||
		state == RunStateSuspended
}

// IsTerminalAttemptState 检查是否为终态。
func IsTerminalAttemptState(state AttemptState) bool {
	return state == AttemptStateCompleted ||
		state == AttemptStateFailed ||
		state == AttemptStateCancelled
}

// IsTerminalStepState 检查是否为终态。
func IsTerminalStepState(state StepState) bool {
	return state == StepStateCompleted ||
		state == StepStateFailed ||
		state == StepStateCancelled
}

// --- Admission 类型 ---

// EventType 语义事件类型。
type EventType string

const (
	// EventRunAccepted Run 被接纳。
	EventRunAccepted EventType = "run.accepted"
)

// RunEvent 语义事件信封。
type RunEvent struct {
	RunID          RunID
	Sequence       uint64
	EventID        string
	AttemptID      *AttemptID
	StepID         *StepID
	EventType      EventType
	PayloadVersion uint
	PayloadJSON    string
}

// AcceptRequest Admission 请求。
type AcceptRequest struct {
	UserID          uint
	ConversationID  *uint
	NotebookID      *uint
	AgentType       string
	Input           InputRef
	SourceIDs       []uint
	IdempotencyKey  string
	VersionSnapshot VersionSnapshot
}

// AcceptedRun Admission 结果。
type AcceptedRun struct {
	RunID          RunID
	MessageID      string
	State          RunState
	Sequence       uint64
	IsIdempotentReplay bool
}
