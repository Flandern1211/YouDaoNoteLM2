package adapter

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/schema"

	"YoudaoNoteLm/internal/agentcontext"
)

// LegacyTurnAdapter 将当前 chatAgentService 的请求映射为 agentcontext.TurnSession。
// 用于 W4 过渡期，将现有 conversationID + content 映射为强类型 TurnSession。
// 不伪造 Run ID、Authority 或持久化 Handle。
type LegacyTurnAdapter struct {
	registry *agentcontext.Registry
}

// NewLegacyTurnAdapter 创建 Legacy Turn Adapter。
func NewLegacyTurnAdapter(registry *agentcontext.Registry) *LegacyTurnAdapter {
	return &LegacyTurnAdapter{registry: registry}
}

// PrepareChatSession 为 Chat Agent 构造 TurnSession。
// conversationID 和 content 来自当前请求；RunID 使用 "legacy-" 前缀标识。
func (a *LegacyTurnAdapter) PrepareChatSession(
	conversationID uint,
	userID uint,
	content string,
) (*agentcontext.TurnSession, error) {
	profile, ok := a.registry.ResolveProfile(agentcontext.ChatV1)
	if !ok {
		return nil, fmt.Errorf("未找到 chat.v1 Profile")
	}

	return &agentcontext.TurnSession{
		Handle: agentcontext.AcceptedTurnHandle{
			RunID:          fmt.Sprintf("legacy-%d", conversationID),
			StepID:         "step-0",
			AgentID:        agentcontext.AgentIDChat,
			UserID:         userID,
			ConversationID: conversationID,
			Input:          agentcontext.UserMessageInput{Content: content},
			ContextMode: agentcontext.ContextModeSnapshot{
				Mode:            "legacy",
				WritebackOwner:  "legacy",
				ContractVersion: "v1",
			},
		},
		Profile: profile.ToSnapshot(),
	}, nil
}

// PrepareSearchSession 为 Search Agent 构造 TurnSession。
func (a *LegacyTurnAdapter) PrepareSearchSession(
	userID uint,
	query string,
) (*agentcontext.TurnSession, error) {
	profile, ok := a.registry.ResolveProfile(agentcontext.SearchV1)
	if !ok {
		return nil, fmt.Errorf("未找到 search.v1 Profile")
	}

	return &agentcontext.TurnSession{
		Handle: agentcontext.AcceptedTurnHandle{
			RunID:   fmt.Sprintf("legacy-search-%d", userID),
			StepID:  "step-0",
			AgentID: agentcontext.AgentIDSearch,
			UserID:  userID,
			Input:   agentcontext.SearchTaskInput{Task: agentcontext.SearchTask{Query: query}},
			ContextMode: agentcontext.ContextModeSnapshot{
				Mode:            "legacy",
				WritebackOwner:  "legacy",
				ContractVersion: "v1",
			},
		},
		Profile: profile.ToSnapshot(),
	}, nil
}

// LegacyHistoryAdapter 将现有 ContextBuilder.BuildMessages 的结果适配为 HistoryProvider。
// 用于 Shadow 模式：Legacy 继续由 ContextBuilder 生成真实输入，
// Shadow 使用 LegacyHistoryProvider 通过 ContextCompiler 生成新 MessagePlan。
type LegacyHistoryAdapter struct {
	// BuildMessages 是现有 ContextBuilder.BuildMessages 的引用
	BuildMessages func(ctx context.Context, conversationID uint, content string) ([]*schema.Message, error)
}

// BuildLegacyMessages 调用现有 ContextBuilder 构建 Legacy 消息。
func (a *LegacyHistoryAdapter) BuildLegacyMessages(
	ctx context.Context,
	conversationID uint,
	content string,
) ([]*schema.Message, error) {
	if a.BuildMessages == nil {
		return nil, fmt.Errorf("BuildMessages 未配置")
	}
	return a.BuildMessages(ctx, conversationID, content)
}
