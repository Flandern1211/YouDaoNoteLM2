package agentcontext

import (
	"context"
)

// PromptProvider Prompt 提供者接口
type PromptProvider interface {
	LoadPrompt(ctx context.Context, query PromptQuery) (Prompt, error)
}

// PromptQuery Prompt 查询
type PromptQuery struct {
	AgentID    AgentID
	ProfileKey ProfileKey
}

// Prompt 版本化 Prompt
type Prompt struct {
	ID      string
	Version string
	Content string
}

// MemoryProvider 记忆提供者接口
type MemoryProvider interface {
	SearchMemory(ctx context.Context, query MemoryQuery) ([]MemoryCandidate, error)
}

// MemoryQuery 记忆查询
type MemoryQuery struct {
	UserID         uint
	Query          string
	Namespaces     []MemoryNamespace
	CandidateLimit int
}

// MemoryNamespace 记忆命名空间
type MemoryNamespace string

const (
	MemoryNamespaceGeneral    MemoryNamespace = "general"
	MemoryNamespacePreference MemoryNamespace = "preference"
)

// MemoryCandidate 记忆候选
type MemoryCandidate struct {
	ID          string
	Content     string
	Score       float64
	Importance  float64
	Pinned      bool
	Sensitivity Sensitivity
	Provenance  Provenance
}

// Sensitivity 敏感度
type Sensitivity string

const (
	SensitivityLow    Sensitivity = "low"
	SensitivityMedium Sensitivity = "medium"
	SensitivityHigh   Sensitivity = "high"
)

// HistoryProvider 历史提供者接口
type HistoryProvider interface {
	LoadHistory(ctx context.Context, query HistoryQuery) (HistorySnapshot, error)
}

// HistoryQuery 历史查询
type HistoryQuery struct {
	ConversationID  uint
	CurrentInputRef *MessageRef
	Limit           int
}

// HistorySnapshot 历史快照
type HistorySnapshot struct {
	Summary  *ConversationSummary
	Messages []HistoryMessage
	Cutoff   MessageRef
}

// ConversationSummary 会话摘要
type ConversationSummary struct {
	Content          string
	ThroughMessageID uint
	ThroughSequence  uint64
	Version          uint64
}

// HistoryMessage 历史消息
type HistoryMessage struct {
	Role    string
	Content string
}

// ModelCapabilitiesResolver 模型能力解析器接口
type ModelCapabilitiesResolver interface {
	ResolveModel(ctx context.Context, ref ModelRef) (ModelCapabilities, error)
}

// TokenCounter Token 计数器接口
type TokenCounter interface {
	CountTokens(ctx context.Context, req TokenCountRequest) (TokenCount, error)
}

// TurnVerifier Turn 验证器接口（W5 使用）
type TurnVerifier interface {
	VerifyAccepted(ctx context.Context, handle AcceptedTurnHandle) (VerifiedTurn, error)
	VerifyAuthority(ctx context.Context, attemptID string, authority ActiveExecutionAuthority) error
}

// VerifiedTurn 已验证的 Turn
type VerifiedTurn struct {
	Handle  AcceptedTurnHandle
	Profile ContextProfileSnapshot
}
