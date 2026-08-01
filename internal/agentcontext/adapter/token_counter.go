package adapter

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/schema"

	"YoudaoNoteLm/internal/agentcontext"
)

// ConservativeTokenCounter UTF-8 保守估算计数器。
// 按中文、ASCII、emoji、工具 JSON 的上界规则计数。
// 首期用于所有模型，直到本地 tokenizer 通过契约测试。
type ConservativeTokenCounter struct{}

// NewConservativeTokenCounter 创建保守估算计数器
func NewConservativeTokenCounter() *ConservativeTokenCounter {
	return &ConservativeTokenCounter{}
}

// CountTokens 使用 UTF-8 保守估算计算 token 数。
// 返回值标记为 conservative_estimate 模式。
func (c *ConservativeTokenCounter) CountTokens(_ context.Context, req agentcontext.TokenCountRequest) (agentcontext.TokenCount, error) {
	total := 0

	// 计算消息 token
	for _, msg := range req.Messages {
		total += estimateMessageTokens(msg)
	}

	// 计算工具定义 token
	for _, tool := range req.ToolInfos {
		total += estimateToolInfoTokens(tool)
	}

	// 模型调用固定包装开销
	total += 10

	return agentcontext.TokenCount{
		Count: total,
		Mode:  agentcontext.TokenizerStrategyConservativeUTF8,
	}, nil
}

// estimateMessageTokens 估算单条消息的 token 数
func estimateMessageTokens(msg *schema.Message) int {
	if msg == nil {
		return 0
	}

	// 固定开销：角色、消息结构
	tokens := 4

	// Content
	tokens += estimateTextTokens(msg.Content)

	// Tool calls
	for _, tc := range msg.ToolCalls {
		tokens += 10 // tool call 结构开销
		tokens += estimateTextTokens(tc.Function.Name)
		tokens += estimateTextTokens(tc.Function.Arguments)
	}

	// Tool call ID
	if msg.ToolCallID != "" {
		tokens += 10
	}

	return tokens
}

// estimateToolInfoTokens 估算工具定义的 token 数
func estimateToolInfoTokens(tool *schema.ToolInfo) int {
	if tool == nil {
		return 0
	}

	tokens := 10 // 结构开销
	tokens += estimateTextTokens(tool.Name)
	tokens += estimateTextTokens(tool.Desc)

	// 参数 schema
	if tool.ParamsOneOf != nil {
		// 简单估算：schema 的 JSON 表示长度
		tokens += 50 // 保守估算参数 schema 开销
	}

	return tokens
}

// estimateTextTokens 估算文本的 token 数（UTF-8 保守估算）
func estimateTextTokens(text string) int {
	if text == "" {
		return 0
	}

	// UTF-8 字节数 / 3 的上界
	// 中文约 3 字节/字符，约 1.5-2 token/字符
	// 英文约 1 字节/字符，约 0.75 token/字符
	// 使用保守上界：字节数 / 3
	bytes := len([]byte(text))
	tokens := (bytes + 2) / 3 // 向上取整
	if tokens < 1 {
		tokens = 1
	}
	return tokens
}

// AnthropicClient Anthropic API 客户端接口（预留）
type AnthropicClient interface {
	CountTokens(ctx context.Context, model string, messages []AnthropicMessage) (int, error)
}

// AnthropicMessage Anthropic 消息格式（预留）
type AnthropicMessage struct {
	Role    string
	Content string
}

// AnthropicTokenCounter Anthropic 官方计数器适配器（预留）。
// 首期使用 ConservativeTokenCounter，待 Anthropic SDK 集成后切换。
type AnthropicTokenCounter struct {
	client AnthropicClient
	model  string
}

// NewAnthropicTokenCounter 创建 Anthropic 计数器（预留）
func NewAnthropicTokenCounter(client AnthropicClient, model string) *AnthropicTokenCounter {
	return &AnthropicTokenCounter{
		client: client,
		model:  model,
	}
}

// CountTokens 调用 Anthropic 官方 API 计算 token 数。
// 返回值标记为 exact_provider 模式。
func (c *AnthropicTokenCounter) CountTokens(ctx context.Context, req agentcontext.TokenCountRequest) (agentcontext.TokenCount, error) {
	if c.client == nil {
		return agentcontext.TokenCount{}, fmt.Errorf("Anthropic client 未配置")
	}

	// 转换消息格式
	var messages []AnthropicMessage
	for _, msg := range req.Messages {
		messages = append(messages, AnthropicMessage{
			Role:    string(msg.Role),
			Content: msg.Content,
		})
	}

	count, err := c.client.CountTokens(ctx, c.model, messages)
	if err != nil {
		return agentcontext.TokenCount{}, fmt.Errorf("Anthropic 计数失败: %w", err)
	}

	return agentcontext.TokenCount{
		Count: count,
		Mode:  agentcontext.TokenizerStrategyExactProvider,
	}, nil
}
