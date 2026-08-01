package adapter

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"YoudaoNoteLm/internal/agentcontext"
)

func TestConservativeTokenCounter_EmptyRequest(t *testing.T) {
	counter := NewConservativeTokenCounter()

	count, err := counter.CountTokens(context.Background(), agentcontext.TokenCountRequest{
		Model: agentcontext.ModelRef{Provider: "openai", ModelID: "gpt-4o"},
	})

	require.NoError(t, err)
	assert.Equal(t, 10, count.Count) // 只有固定包装开销
	assert.Equal(t, agentcontext.TokenizerStrategyConservativeUTF8, count.Mode)
}

func TestConservativeTokenCounter_WithMessages(t *testing.T) {
	counter := NewConservativeTokenCounter()

	messages := []*schema.Message{
		{Role: schema.System, Content: "You are a helpful assistant."},
		{Role: schema.User, Content: "Hello, how are you?"},
		{Role: schema.Assistant, Content: "I'm doing well, thank you!"},
	}

	count, err := counter.CountTokens(context.Background(), agentcontext.TokenCountRequest{
		Model:    agentcontext.ModelRef{Provider: "openai", ModelID: "gpt-4o"},
		Messages: messages,
	})

	require.NoError(t, err)
	assert.Greater(t, count.Count, 10)
	assert.Equal(t, agentcontext.TokenizerStrategyConservativeUTF8, count.Mode)
}

func TestConservativeTokenCounter_ChineseText(t *testing.T) {
	counter := NewConservativeTokenCounter()

	messages := []*schema.Message{
		{Role: schema.User, Content: "你好，世界！"},
	}

	count, err := counter.CountTokens(context.Background(), agentcontext.TokenCountRequest{
		Model:    agentcontext.ModelRef{Provider: "openai", ModelID: "gpt-4o"},
		Messages: messages,
	})

	require.NoError(t, err)
	assert.Greater(t, count.Count, 0)
}

func TestConservativeTokenCounter_EmojiText(t *testing.T) {
	counter := NewConservativeTokenCounter()

	messages := []*schema.Message{
		{Role: schema.User, Content: "👍🎉🚀"},
	}

	count, err := counter.CountTokens(context.Background(), agentcontext.TokenCountRequest{
		Model:    agentcontext.ModelRef{Provider: "openai", ModelID: "gpt-4o"},
		Messages: messages,
	})

	require.NoError(t, err)
	assert.Greater(t, count.Count, 0)
}

func TestConservativeTokenCounter_WithToolCalls(t *testing.T) {
	counter := NewConservativeTokenCounter()

	messages := []*schema.Message{
		{
			Role:    schema.Assistant,
			Content: "I'll search for that.",
			ToolCalls: []schema.ToolCall{
				{
					ID: "call-1",
					Function: schema.FunctionCall{
						Name:      "search_knowledge",
						Arguments: `{"query": "test"}`,
					},
				},
			},
		},
		{
			Role:       schema.Tool,
			ToolCallID: "call-1",
			Content:    "search results here",
		},
	}

	count, err := counter.CountTokens(context.Background(), agentcontext.TokenCountRequest{
		Model:    agentcontext.ModelRef{Provider: "openai", ModelID: "gpt-4o"},
		Messages: messages,
	})

	require.NoError(t, err)
	assert.Greater(t, count.Count, 20) // 包含 tool call 开销
}

func TestConservativeTokenCounter_WithToolInfos(t *testing.T) {
	counter := NewConservativeTokenCounter()

	toolInfos := []*schema.ToolInfo{
		{
			Name: "search_knowledge",
			Desc: "Search the knowledge base for relevant information",
		},
		{
			Name: "get_sources_summary",
			Desc: "Get a summary of the selected sources",
		},
	}

	count, err := counter.CountTokens(context.Background(), agentcontext.TokenCountRequest{
		Model:     agentcontext.ModelRef{Provider: "openai", ModelID: "gpt-4o"},
		ToolInfos: toolInfos,
	})

	require.NoError(t, err)
	assert.Greater(t, count.Count, 20) // 包含工具定义开销
}

func TestConservativeTokenCounter_MixedContent(t *testing.T) {
	counter := NewConservativeTokenCounter()

	messages := []*schema.Message{
		{Role: schema.System, Content: "You are a helpful assistant."},
		{Role: schema.User, Content: "请帮我搜索一下关于机器学习的资料"},
		{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{
				{
					ID: "call-1",
					Function: schema.FunctionCall{
						Name:      "search_knowledge",
						Arguments: `{"query": "机器学习"}`,
					},
				},
			},
		},
		{
			Role:       schema.Tool,
			ToolCallID: "call-1",
			Content:    "机器学习是人工智能的一个分支...",
		},
		{Role: schema.Assistant, Content: "根据资料，机器学习是人工智能的一个重要分支。"},
	}

	toolInfos := []*schema.ToolInfo{
		{Name: "search_knowledge", Desc: "Search knowledge base"},
	}

	count, err := counter.CountTokens(context.Background(), agentcontext.TokenCountRequest{
		Model:     agentcontext.ModelRef{Provider: "openai", ModelID: "gpt-4o"},
		Messages:  messages,
		ToolInfos: toolInfos,
	})

	require.NoError(t, err)
	assert.Greater(t, count.Count, 50)
}

func TestEstimateTextTokens(t *testing.T) {
	tests := []struct {
		name string
		text string
		min  int
	}{
		{"empty", "", 0},
		{"ascii short", "hello", 1},
		{"ascii long", "hello world this is a test", 5},
		{"chinese", "你好世界", 1},
		{"chinese long", "这是一段很长的中文文本用于测试", 5},
		{"emoji", "👍🎉", 1},
		{"mixed", "hello 你好 👍", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := estimateTextTokens(tt.text)
			assert.GreaterOrEqual(t, tokens, tt.min)
		})
	}
}

func TestEstimateToolInfoTokens(t *testing.T) {
	tool := &schema.ToolInfo{
		Name: "search_knowledge",
		Desc: "Search the knowledge base for relevant information",
	}

	tokens := estimateToolInfoTokens(tool)
	assert.Greater(t, tokens, 10)
}

func TestEstimateToolInfoTokens_Nil(t *testing.T) {
	tokens := estimateToolInfoTokens(nil)
	assert.Equal(t, 0, tokens)
}

func TestEstimateMessageTokens_Nil(t *testing.T) {
	tokens := estimateMessageTokens(nil)
	assert.Equal(t, 0, tokens)
}
