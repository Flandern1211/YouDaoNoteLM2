package eino

import (
	"fmt"

	"github.com/cloudwego/eino/schema"

	"YoudaoNoteLm/internal/agentcontext"
)

// AgentInputBuilder 从 PreparedTurn 构建 Eino Agent 的初始消息。
// 负责将 MessagePlan 转换为 Eino schema.Message 列表。
// 不处理 Token 预算治理（由 BudgetCompiler 在 Middleware 中完成）。
type AgentInputBuilder struct{}

// NewAgentInputBuilder 创建 AgentInputBuilder。
func NewAgentInputBuilder() *AgentInputBuilder {
	return &AgentInputBuilder{}
}

// BuildMessages 从 PreparedTurn 构建初始消息列表。
// 消息布局（对应设计文档第 10 节）：
//
//	System: Agent Instruction
//	History: 原生 User/Assistant/Tool 消息
//	Final User: <conversation_summary> + <user_memories> + <current_request>
//
// 注意：GenModelInput 会在头部注入唯一系统 Instruction，
// 所以此处不重复添加系统消息。只构建历史、摘要、记忆和当前输入。
func (b *AgentInputBuilder) BuildMessages(turn *agentcontext.PreparedTurn) ([]*schema.Message, error) {
	if turn == nil {
		return nil, fmt.Errorf("turn 不能为空")
	}

	var messages []*schema.Message

	// 1. 历史消息（已在 PrepareTurn 中通过 HistoryMessageToSchema 转换）
	messages = append(messages, turn.MessagePlan.History...)

	// 2. 构建最终用户消息（包含摘要、记忆和当前输入）
	finalMsg := b.buildFinalUserMessage(turn)
	if finalMsg != nil {
		messages = append(messages, finalMsg)
	}

	return messages, nil
}

// buildFinalUserMessage 构建包含摘要、记忆和当前输入的最终用户消息。
func (b *AgentInputBuilder) buildFinalUserMessage(turn *agentcontext.PreparedTurn) *schema.Message {
	var content string

	// 摘要
	if turn.MessagePlan.Summary != nil && turn.MessagePlan.Summary.Content != "" {
		content += fmt.Sprintf("<conversation_summary>%s</conversation_summary>\n", turn.MessagePlan.Summary.Content)
	}

	// 记忆
	if len(turn.MessagePlan.Memories) > 0 {
		content += "<user_memories>\n"
		for _, mem := range turn.MessagePlan.Memories {
			content += fmt.Sprintf("<memory id=\"%s\">%s</memory>\n", mem.ID, mem.Content)
		}
		content += "</user_memories>\n"
	}

	// 当前输入
	if turn.MessagePlan.CurrentInput != nil {
		switch input := turn.MessagePlan.CurrentInput.(type) {
		case agentcontext.UserMessageInput:
			content += input.Content
		case agentcontext.SearchTaskInput:
			content += fmt.Sprintf("<search_task>\nquery: %s\n</search_task>", input.Task.Query)
		}
	}

	if content == "" {
		return nil
	}

	return &schema.Message{
		Role:    schema.User,
		Content: content,
	}
}
