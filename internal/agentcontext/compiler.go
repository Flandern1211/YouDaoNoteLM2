package agentcontext

import (
	"context"

	"github.com/cloudwego/eino/schema"
)

// ContextCompiler 上下文编译器接口（W0-W4 可独立交付边界）
type ContextCompiler interface {
	// PrepareTurn 每个 Agent invocation 执行一次，加载稳定的 Provider 候选
	PrepareTurn(
		ctx context.Context,
		turn *TurnSession,
		req PrepareTurnRequest,
	) (*PreparedTurn, error)

	// CompileModelInput 由 Eino Middleware 在每次模型调用前执行
	CompileModelInput(
		ctx context.Context,
		req CompileRequest,
	) (*CompiledContext, error)
}

// TurnLifecycleCoordinator 生命周期协调器接口（W5-W7 依赖 Harness）
type TurnLifecycleCoordinator interface {
	// BeginTurn 验证持久化 Handle 和执行权
	BeginTurn(
		ctx context.Context,
		req BeginTurnRequest,
	) (*TurnSession, error)

	// FinalizeTurn 覆盖成功、失败、取消等终态
	FinalizeTurn(
		ctx context.Context,
		req FinalizeRequest,
	) (*FinalizeResult, error)
}

// Manager 聚合门面（目标态）
type Manager interface {
	ContextCompiler
	TurnLifecycleCoordinator
}

// TurnSession Turn 会话
type TurnSession struct {
	Handle  AcceptedTurnHandle
	Profile ContextProfileSnapshot
}

// AcceptedTurnHandle 已接受的 Turn 句柄
type AcceptedTurnHandle struct {
	RunID           string
	StepID          string
	AgentID         AgentID
	UserID          uint
	ConversationID  uint
	Input           TurnInput
	CurrentInputRef *MessageRef
	ContextMode     ContextModeSnapshot
}

// ActiveExecutionAuthority 活跃执行权
type ActiveExecutionAuthority struct {
	AttemptID       string
	FencingToken    uint64
	RunStateVersion uint64
}

// BeginTurnRequest 开始 Turn 请求
type BeginTurnRequest struct {
	Handle    AcceptedTurnHandle
	Authority ActiveExecutionAuthority
}

// PrepareTurnRequest 准备 Turn 请求
type PrepareTurnRequest struct {
	Model ModelRef
}

// PreparedTurn 已准备的 Turn
type PreparedTurn struct {
	Session           *TurnSession
	Profile           ContextProfileSnapshot
	Instruction       string
	MessagePlan       MessagePlan
	ModelCapabilities ModelCapabilities
	BaseManifest      ContextManifest
}

// MessagePlan 消息计划
type MessagePlan struct {
	Summary      *ContextItem
	Memories     []ContextItem
	History      []*schema.Message
	CurrentInput TurnInput
	// RuntimeMessages 保存当前输入之后由 Eino 产生的显式 ToolCall/ToolResult。
	RuntimeMessages []*schema.Message
}

// CompileRequest 编译请求
type CompileRequest struct {
	Turn              *PreparedTurn
	Messages          []*schema.Message
	ToolInfos         []*schema.ToolInfo
	DeferredToolInfos []*schema.ToolInfo
}

// CompiledContext 已编译的上下文
type CompiledContext struct {
	Messages []*schema.Message
	Record   CompileRecord
}

// CompileRecord 编译记录
type CompileRecord struct {
	ModelCallID             string
	Manifest                ContextManifest
	ContextHMAC             string
	RuntimeSummaryCandidate *SummaryCandidate
}

// SummaryCandidate 摘要候选
type SummaryCandidate struct {
	Content          string
	ThroughMessageID uint
}

// ContextManifest 上下文清单
type ContextManifest struct {
	ProfileID       string
	ProfileVersion  string
	PromptVersion   string
	ToolsetVersion  string
	Model           string
	InputBudget     int
	EstimatedTokens int
	ExactTokens     *int
	CounterMode     string
	Sources         []SourceManifest
	TurnStatus      string
	Degraded        bool
	ContextHMAC     string
}

// SourceManifest 来源清单
type SourceManifest struct {
	Provider       string
	CandidateCount int
	SelectedCount  int
	TokenCount     int
	Status         string
	RetryCount     int
	FallbackStage  int
}

// FinalizeRequest 终态请求
type FinalizeRequest struct {
	Turn           *PreparedTurn
	Outcome        TurnOutcome
	CompileRecords []CompileRecord
	FinalizeKey    FinalizeKey
	Authority      ActiveExecutionAuthority
}

// FinalizeKey 终态键
type FinalizeKey struct {
	RunID    string
	Revision uint64
}

// TurnOutcome Turn 结果
type TurnOutcome struct {
	Status        TurnStatus
	PrimaryOutput PrimaryOutput
	Messages      []*schema.Message
}

// TurnStatus Turn 状态
type TurnStatus string

const (
	TurnStatusSuccess   TurnStatus = "success"
	TurnStatusFailed    TurnStatus = "failed"
	TurnStatusCancelled TurnStatus = "cancelled"
	TurnStatusSuspended TurnStatus = "suspended"
)

// PrimaryOutput 主输出
type PrimaryOutput interface {
	isPrimaryOutput()
}

// ConversationOutput 会话输出
type ConversationOutput struct {
	FinalMessage *schema.Message
	// Metadata 保存可持久化的结构化元数据（例如引用 JSON），不包含隐藏推理。
	Metadata []byte
}

func (ConversationOutput) isPrimaryOutput() {}

// StepOutput 步骤输出（Search）
type StepOutput struct {
	Result interface{}
}

func (StepOutput) isPrimaryOutput() {}

// FinalizeResult 终态结果
type FinalizeResult struct {
	Primary  interface{}
	Summary  WritebackStatus
	Memory   WritebackStatus
	Manifest WritebackStatus
}

// WritebackStatus 写回状态
type WritebackStatus string

const (
	WritebackStatusSuccess WritebackStatus = "success"
	WritebackStatusSkipped WritebackStatus = "skipped"
	WritebackStatusFailed  WritebackStatus = "failed"
)
