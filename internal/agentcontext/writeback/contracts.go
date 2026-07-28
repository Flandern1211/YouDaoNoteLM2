// Package writeback 定义 Writer 端口和写回协调契约。
// Writer 接口由 WritebackCoordinator 使用方拥有；
// 具体持久化适配通过 internal/app 注入。
package writeback

import (
	"context"

	"YoudaoNoteLm/internal/agentcontext"
)

// ============ Writer 端口 ============

// AssistantMessageWriter 提交助手消息。
// 在同一事务中记录后续 Writeback intents 和 Outbox/Journal。
type AssistantMessageWriter interface {
	CommitAssistant(ctx context.Context, req AssistantWriteRequest) (CommittedMessage, error)
}

// SummaryWriter 评估并更新会话摘要。
// 使用 CAS 推进 ThroughMessageID 或 Sequence。
type SummaryWriter interface {
	EvaluateAndUpdate(ctx context.Context, req SummaryWriteRequest) (SummaryWriteResult, error)
}

// MemoryWriter 评估并存储用户记忆。
// 只从调用方原始输入和本轮真实输出提取。
type MemoryWriter interface {
	EvaluateAndStore(ctx context.Context, req MemoryWriteRequest) (MemoryWriteResult, error)
}

// ManifestWriter 存储无正文上下文清单。
// 成功、失败和取消的 Turn 都允许记录。
type ManifestWriter interface {
	StoreManifest(ctx context.Context, req ManifestWriteRequest) error
}

// StepResultWriter 提交步骤结果（Search Profile）。
// 不创建主会话 Assistant Message。
type StepResultWriter interface {
	CommitStepResult(ctx context.Context, req StepResultWriteRequest) (CommittedStepResult, error)
}

// ============ 写回请求 ============

// AssistantWriteRequest 助手消息写入请求
type AssistantWriteRequest struct {
	FinalizeKey agentcontext.FinalizeKey
	Ticket      FinalizationTicket
	Authority   agentcontext.ActiveExecutionAuthority

	RunID          string
	ConversationID uint
	UserID         uint
	UserContent    string
	Content        string
	References     []byte // JSON 编码的引用
	IdempotencyKey string
	ProfileID      string
	Mode           string
}

// CommittedMessage 已提交的消息
type CommittedMessage struct {
	MessageID uint
	Sequence  uint64
	Hash      string
}

// SummaryWriteRequest 摘要写入请求
type SummaryWriteRequest struct {
	FinalizeKey agentcontext.FinalizeKey
	Ticket      FinalizationTicket

	ConversationID   uint
	CurrentSummary   string
	NewContent       string
	ThroughMessageID uint
	ExpectedVersion  uint64
	IdempotencyKey   string
}

// SummaryWriteResult 摘要写入结果
type SummaryWriteResult struct {
	Status        WritebackStatus
	NewVersion    uint64
	SkippedReason string
}

// MemoryWriteRequest 记忆写入请求
type MemoryWriteRequest struct {
	FinalizeKey agentcontext.FinalizeKey
	Ticket      FinalizationTicket

	UserID         uint
	SourceContent  string // 调用方原始输入
	OutputContent  string // 本轮真实助手输出
	ProfileID      string
	IdempotencyKey string
}

// MemoryWriteResult 记忆写入结果
type MemoryWriteResult struct {
	Status        WritebackStatus
	FactCount     int
	SkippedReason string
}

// ManifestWriteRequest 清单写入请求
type ManifestWriteRequest struct {
	FinalizeKey agentcontext.FinalizeKey
	Ticket      FinalizationTicket

	Manifest       agentcontext.ContextManifest
	ModelCallID    string
	TurnStatus     string
	IdempotencyKey string
}

// StepResultWriteRequest 步骤结果写入请求
type StepResultWriteRequest struct {
	FinalizeKey agentcontext.FinalizeKey
	Ticket      FinalizationTicket
	Authority   agentcontext.ActiveExecutionAuthority

	RunID          string
	StepID         string
	UserID         uint
	Result         interface{}
	IdempotencyKey string
	ProfileID      string
}

// CommittedStepResult 已提交的步骤结果
type CommittedStepResult struct {
	StepResultID string
}

// ============ 写回状态 ============

// WritebackStatus 写回状态
type WritebackStatus string

const (
	WritebackStatusSuccess WritebackStatus = "success"
	WritebackStatusSkipped WritebackStatus = "skipped"
	WritebackStatusFailed  WritebackStatus = "failed"
	WritebackStatusPending WritebackStatus = "pending"
)

// WritebackOperation 写回操作类型
type WritebackOperation string

const (
	WritebackOperationAssistant  WritebackOperation = "assistant"
	WritebackOperationStepResult WritebackOperation = "step_result"
	WritebackOperationSummary    WritebackOperation = "summary"
	WritebackOperationMemory     WritebackOperation = "memory"
	WritebackOperationManifest   WritebackOperation = "manifest"
)

// ============ Finalization Ticket ============

// FinalizationTicket 终态化票据
type FinalizationTicket struct {
	ID          string
	Key         agentcontext.FinalizeKey
	IntentKinds []WritebackOperation
}

// FinalizationAuthority 终态化授权
type FinalizationAuthority struct {
	TicketID      string
	LeaseToken    string
	TicketVersion uint64
}
