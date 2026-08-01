package agentcontext

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateInputBudget_Default(t *testing.T) {
	budget := DefaultBudgetConfig()
	budget.ContextWindow = 128000
	budget.MaxOutputTokens = 4096

	inputBudget := calculateInputBudget(budget, ModelRef{Provider: "openai", ModelID: "gpt-4o"})

	// 128000 - 4096 - max(512, 128000*0.05) = 128000 - 4096 - 6400 = 117504
	expected := 128000 - 4096 - 6400
	assert.Equal(t, expected, inputBudget)
}

func TestCalculateInputBudget_SmallContext(t *testing.T) {
	budget := DefaultBudgetConfig()
	budget.ContextWindow = 8000
	budget.MaxOutputTokens = 1024

	inputBudget := calculateInputBudget(budget, ModelRef{})

	// 8000 - 1024 - max(512, 8000*0.05) = 8000 - 1024 - 512 = 6464
	expected := 8000 - 1024 - 512
	assert.Equal(t, expected, inputBudget)
}

func TestEstimateTextTokens_Empty(t *testing.T) {
	assert.Equal(t, 0, estimateTextTokens(""))
}

func TestEstimateTextTokens_ASCII(t *testing.T) {
	tokens := estimateTextTokens("hello world")
	// 11 bytes / 3 = 4 (rounded up)
	assert.Greater(t, tokens, 0)
	assert.LessOrEqual(t, tokens, 10)
}

func TestEstimateTextTokens_Chinese(t *testing.T) {
	tokens := estimateTextTokens("你好世界")
	// 12 bytes / 3 = 4
	assert.Greater(t, tokens, 0)
	assert.LessOrEqual(t, tokens, 10)
}

func TestEstimateTextTokens_Emoji(t *testing.T) {
	tokens := estimateTextTokens("👍🎉")
	// 8 bytes / 3 = 3
	assert.Greater(t, tokens, 0)
	assert.LessOrEqual(t, tokens, 10)
}

func TestEstimateMessageTokens_UserMessage(t *testing.T) {
	msg := &schema.Message{
		Role:    schema.User,
		Content: "hello",
	}

	tokens := estimateMessageTokens(msg)
	assert.Greater(t, tokens, 0)
}

func TestEstimateMessageTokens_ToolCall(t *testing.T) {
	msg := &schema.Message{
		Role:    schema.Assistant,
		Content: "calling tool",
		ToolCalls: []schema.ToolCall{
			{ID: "call-1", Function: schema.FunctionCall{Name: "search", Arguments: `{"query":"test"}`}},
		},
	}

	tokens := estimateMessageTokens(msg)
	assert.Greater(t, tokens, 10) // 包含 tool call 开销
}

func TestEstimateMessageTokens_ToolResult(t *testing.T) {
	msg := &schema.Message{
		Role:       schema.Tool,
		ToolCallID: "call-1",
		Content:    "search results",
	}

	tokens := estimateMessageTokens(msg)
	assert.Greater(t, tokens, 0)
}

func TestBudgetCompiler_FastPath(t *testing.T) {
	// 估算值低于 70%：快速路径
	counter := &mockTokenCounter{count: 100}
	bc := NewBudgetCompiler(counter, ModelRef{Provider: "openai", ModelID: "gpt-4o"})

	profile := ContextProfileSnapshot{
		Budget: BudgetConfig{
			ContextWindow:           128000,
			MaxOutputTokens:         4096,
			SafetyMarginRatio:       0.05,
			SafetyMarginMin:         512,
			FastPathThreshold:       0.70,
			FullGovernanceThreshold: 0.80,
			GovernanceTarget:        0.60,
		},
	}

	plan := MessagePlan{
		CurrentInput: UserMessageInput{Content: "short question"},
	}

	result, messages, err := bc.Compile(profile, "system prompt", plan, nil)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, messages)
	assert.Equal(t, GovernanceActionNone, result.Action)
}

func TestBudgetCompiler_GovernanceNeeded(t *testing.T) {
	// 创建大量消息触发治理
	counter := &mockTokenCounter{count: 50000}
	bc := NewBudgetCompiler(counter, ModelRef{Provider: "openai", ModelID: "gpt-4o"})

	profile := ContextProfileSnapshot{
		Budget: BudgetConfig{
			ContextWindow:           128000,
			MaxOutputTokens:         4096,
			SafetyMarginRatio:       0.05,
			SafetyMarginMin:         512,
			FastPathThreshold:       0.70,
			FullGovernanceThreshold: 0.80,
			GovernanceTarget:        0.60,
		},
	}

	// 构建大量历史消息
	var history []*schema.Message
	for i := 0; i < 100; i++ {
		history = append(history, schema.UserMessage("this is a long message with many tokens to trigger governance"))
		history = append(history, schema.AssistantMessage("this is a long response with many tokens to trigger governance", nil))
	}

	plan := MessagePlan{
		Summary:      &ContextItem{Content: "a very long summary " + repeatString("x", 5000)},
		History:      history,
		CurrentInput: UserMessageInput{Content: "question"},
	}

	result, messages, err := bc.Compile(profile, "system prompt", plan, nil)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, messages)
	// 治理后应该有消息被淘汰
	assert.LessOrEqual(t, result.AfterTokens, result.BeforeTokens)
}

func TestBuildMessagesFromPlan_WithSummary(t *testing.T) {
	plan := MessagePlan{
		Summary:      &ContextItem{Content: "previous summary"},
		CurrentInput: UserMessageInput{Content: "hello"},
	}

	messages := buildMessagesFromPlan("system prompt", plan, nil)

	require.Len(t, messages, 3) // system + summary + user
	assert.Equal(t, schema.System, messages[0].Role)
	assert.Equal(t, schema.System, messages[1].Role)
	assert.Contains(t, messages[1].Content, "conversation_summary")
	assert.Equal(t, schema.User, messages[2].Role)
}

func TestBuildMessagesFromPlan_WithMemories(t *testing.T) {
	plan := MessagePlan{
		Memories: []ContextItem{
			{ID: "mem-1", Content: "memory 1"},
			{ID: "mem-2", Content: "memory 2"},
		},
		CurrentInput: UserMessageInput{Content: "hello"},
	}

	messages := buildMessagesFromPlan("system prompt", plan, nil)

	require.Len(t, messages, 3) // system + memories + user
	assert.Contains(t, messages[1].Content, "user_memories")
	assert.Contains(t, messages[1].Content, "memory 1")
	assert.Contains(t, messages[1].Content, "memory 2")
}

func TestBuildMessagesFromPlan_SearchTask(t *testing.T) {
	plan := MessagePlan{
		CurrentInput: SearchTaskInput{Task: SearchTask{Query: "search query"}},
	}

	messages := buildMessagesFromPlan("system prompt", plan, nil)

	require.Len(t, messages, 2) // system + search task
	assert.Contains(t, messages[1].Content, "search_task")
	assert.Contains(t, messages[1].Content, "search query")
}

// mockTokenCounter 用于测试的 TokenCounter mock
type mockTokenCounter struct {
	count int
	err   error
}

func (m *mockTokenCounter) CountTokens(_ context.Context, _ TokenCountRequest) (TokenCount, error) {
	return TokenCount{
		Count: m.count,
		Mode:  TokenizerStrategyConservativeUTF8,
	}, m.err
}

// repeatString 重复字符串
func repeatString(s string, n int) string {
	result := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		result = append(result, s...)
	}
	return string(result)
}
