// Package agentcontext 定义上下文管理模块的核心类型和接口。
// 该包不依赖 Repository、Redis、具体模型 SDK 或 Eino Agent Builder。
package agentcontext

import (
	"github.com/cloudwego/eino/schema"
)

// RecentRoundsLimit 缓存中保留的最近对话轮数（与 Legacy ContextBuilder 一致）
const RecentRoundsLimit = 10

// AgentID 标识 Agent 类型
type AgentID string

const (
	AgentIDChat   AgentID = "chat"
	AgentIDMain   AgentID = "main"
	AgentIDSearch AgentID = "search"
)

// TurnInput 封闭的强类型输入表示
type TurnInput interface {
	isTurnInput()
}

// UserMessageInput 用户消息输入
type UserMessageInput struct {
	Content string
}

func (UserMessageInput) isTurnInput() {}

// SearchTaskInput 搜索任务输入
type SearchTaskInput struct {
	Task SearchTask
}

func (SearchTaskInput) isTurnInput() {}

// SearchTask 搜索任务
type SearchTask struct {
	Query string
}

// MessageRef 消息引用
type MessageRef struct {
	MessageID uint
	Sequence  uint64
	Hash      string
}

// ContextModeSnapshot 上下文模式快照
type ContextModeSnapshot struct {
	Mode            string
	WritebackOwner  string
	ContractVersion string
}

// ProfileKey Profile 唯一标识
type ProfileKey struct {
	Name    string
	Version string
}

// ContextProfileSnapshot Profile 快照
type ContextProfileSnapshot struct {
	Key             ProfileKey
	AgentID         AgentID
	AllowedSources  []ContextKind
	WritebackPolicy WritebackPolicy
	// 预算配置
	Budget BudgetConfig
}

// WritebackPolicy 写回策略
type WritebackPolicy string

const (
	WritebackPolicyConversationTurn WritebackPolicy = "conversation_turn"
	WritebackPolicyStepResult       WritebackPolicy = "step_result"
)

// ContextKind 上下文内容类别
type ContextKind string

const (
	ContextKindConversationSummary ContextKind = "conversation_summary"
	ContextKindUserMemory          ContextKind = "user_memory"
	ContextKindDelegatedPreference ContextKind = "delegated_preference"
)

// TrustLevel 信任级别
type TrustLevel string

const (
	TrustLevelSystemTrusted     TrustLevel = "system_trusted"
	TrustLevelUserProvided      TrustLevel = "user_provided"
	TrustLevelExternalUntrusted TrustLevel = "external_untrusted"
)

// Provenance 来源信息
type Provenance struct {
	Provider string
	Stage    string
}

// ContextItem 可独立选择或淘汰的数据块
type ContextItem struct {
	ID         string
	Kind       ContextKind
	Content    string
	Priority   int
	Trust      TrustLevel
	TokenCount int
	Pinned     bool
	Provenance Provenance
}

// ModelRef 模型引用
type ModelRef struct {
	Provider string
	ModelID  string
}

// ModelCapabilities 模型能力
type ModelCapabilities struct {
	ContextWindow     int
	MaxOutputTokens   int
	TokenizerStrategy TokenizerStrategy
	SupportsToolCalls bool
}

// TokenizerStrategy 分词器策略
type TokenizerStrategy string

const (
	TokenizerStrategyExactProvider    TokenizerStrategy = "exact_provider"
	TokenizerStrategyCompatibleLocal  TokenizerStrategy = "compatible_local"
	TokenizerStrategyConservativeUTF8 TokenizerStrategy = "conservative_utf8_bytes"
)

// TokenCountRequest Token 计数请求
type TokenCountRequest struct {
	Model     ModelRef
	Messages  []*schema.Message
	ToolInfos []*schema.ToolInfo
}

// TokenCount Token 计数结果
type TokenCount struct {
	Count int
	Mode  TokenizerStrategy
}

// BudgetConfig 预算配置
type BudgetConfig struct {
	ContextWindow           int
	MaxOutputTokens         int
	SafetyMarginRatio       float64
	SafetyMarginMin         int
	MemoryMaxRatio          float64
	SummaryMaxRatio         float64
	FastPathThreshold       float64
	FullGovernanceThreshold float64
	GovernanceTarget        float64
}

// DefaultBudgetConfig 返回默认预算配置
func DefaultBudgetConfig() BudgetConfig {
	return BudgetConfig{
		SafetyMarginRatio:       0.05,
		SafetyMarginMin:         512,
		MemoryMaxRatio:          0.05,
		SummaryMaxRatio:         0.10,
		FastPathThreshold:       0.70,
		FullGovernanceThreshold: 0.80,
		GovernanceTarget:        0.60,
	}
}
