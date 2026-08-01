package agentcontext

import (
	"github.com/cloudwego/eino/schema"
)

// MessageGroupType 消息组类型
type MessageGroupType string

const (
	// MessageGroupTypeStandalone 独立消息（普通用户/助手消息）
	MessageGroupTypeStandalone MessageGroupType = "standalone"
	// MessageGroupTypeToolExchange 工具交换组（tool call + tool results）
	MessageGroupTypeToolExchange MessageGroupType = "tool_exchange"
)

// MessageGroup 消息原子组。
// tool call 与对应 tool result 建模为不可拆分组。
type MessageGroup struct {
	Type     MessageGroupType
	Messages []*schema.Message
	// IsClosed 表示工具交换是否已闭合（所有 tool call 都有对应 result）
	IsClosed bool
	// IsHardReserved 硬保留项不可被淘汰
	IsHardReserved bool
}

// BuildMessageGroups 将消息列表分组为原子组。
// 普通消息各自独立成组；tool call 与其对应的 tool result 组成不可拆分组。
func BuildMessageGroups(messages []*schema.Message) []MessageGroup {
	if len(messages) == 0 {
		return nil
	}

	var groups []MessageGroup
	i := 0

	for i < len(messages) {
		msg := messages[i]

		// 检查是否是 tool call 消息
		if msg.Role == schema.Assistant && len(msg.ToolCalls) > 0 {
			group := buildToolExchangeGroup(messages, i)
			groups = append(groups, group)
			i += len(group.Messages)
			continue
		}

		// 普通消息独立成组
		groups = append(groups, MessageGroup{
			Type:     MessageGroupTypeStandalone,
			Messages: []*schema.Message{msg},
		})
		i++
	}

	return groups
}

// buildToolExchangeGroup 从 tool call 消息开始构建工具交换组。
// 包含 tool call 消息及其后所有对应的 tool result 消息。
func buildToolExchangeGroup(messages []*schema.Message, startIdx int) MessageGroup {
	group := MessageGroup{
		Type:     MessageGroupTypeToolExchange,
		IsClosed: false,
	}

	// 添加 tool call 消息
	callMsg := messages[startIdx]
	group.Messages = append(group.Messages, callMsg)

	// 收集所有 tool call ID
	callIDs := make(map[string]bool)
	for _, tc := range callMsg.ToolCalls {
		callIDs[tc.ID] = true
	}

	// 收集对应的 tool result 消息
	resultCount := 0
	for j := startIdx + 1; j < len(messages); j++ {
		resultMsg := messages[j]

		// tool result 消息
		if resultMsg.Role == schema.Tool {
			group.Messages = append(group.Messages, resultMsg)
			if callIDs[resultMsg.ToolCallID] {
				resultCount++
			}
			continue
		}

		// 遇到非 tool 消息，停止收集
		break
	}

	// 判断是否闭合：所有 tool call 都有对应 result
	group.IsClosed = resultCount == len(callIDs)

	return group
}

// FlattenGroups 将消息组列表展平为消息列表，保持原始顺序。
func FlattenGroups(groups []MessageGroup) []*schema.Message {
	var messages []*schema.Message
	for _, g := range groups {
		messages = append(messages, g.Messages...)
	}
	return messages
}

// CountGroupTokens 估算消息组的 token 数。
// 使用保守估算：每条消息的 content 长度 + 固定开销。
func CountGroupTokens(group MessageGroup, counter TokenCounter, model ModelRef) (int, error) {
	total := 0
	for _, msg := range group.Messages {
		count, err := counter.CountTokens(nil, TokenCountRequest{
			Model:    model,
			Messages: []*schema.Message{msg},
		})
		if err != nil {
			return 0, err
		}
		total += count.Count
	}
	return total, nil
}

// IsToolCallMessage 检查消息是否包含 tool calls
func IsToolCallMessage(msg *schema.Message) bool {
	return msg.Role == schema.Assistant && len(msg.ToolCalls) > 0
}

// IsToolResultMessage 检查消息是否是 tool result
func IsToolResultMessage(msg *schema.Message) bool {
	return msg.Role == schema.Tool
}
