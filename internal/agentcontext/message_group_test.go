package agentcontext

import (
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildMessageGroups_Empty(t *testing.T) {
	groups := BuildMessageGroups(nil)
	assert.Nil(t, groups)
}

func TestBuildMessageGroups_StandaloneMessages(t *testing.T) {
	messages := []*schema.Message{
		{Role: schema.User, Content: "hello"},
		{Role: schema.Assistant, Content: "hi"},
		{Role: schema.User, Content: "how are you"},
	}

	groups := BuildMessageGroups(messages)

	require.Len(t, groups, 3)
	for _, g := range groups {
		assert.Equal(t, MessageGroupTypeStandalone, g.Type)
		assert.Len(t, g.Messages, 1)
	}
}

func TestBuildMessageGroups_ToolExchangeClosed(t *testing.T) {
	messages := []*schema.Message{
		{Role: schema.User, Content: "search for X"},
		{
			Role:    schema.Assistant,
			Content: "I'll search",
			ToolCalls: []schema.ToolCall{
				{ID: "call-1", Function: schema.FunctionCall{Name: "search"}},
			},
		},
		{
			Role:       schema.Tool,
			ToolCallID: "call-1",
			Content:    "search results",
		},
		{Role: schema.Assistant, Content: "Here are the results"},
	}

	groups := BuildMessageGroups(messages)

	require.Len(t, groups, 3)
	assert.Equal(t, MessageGroupTypeStandalone, groups[0].Type) // user
	assert.Equal(t, MessageGroupTypeToolExchange, groups[1].Type)
	assert.True(t, groups[1].IsClosed)
	assert.Len(t, groups[1].Messages, 2)                        // call + result
	assert.Equal(t, MessageGroupTypeStandalone, groups[2].Type) // assistant
}

func TestBuildMessageGroups_ToolExchangeUnclosed(t *testing.T) {
	messages := []*schema.Message{
		{
			Role:    schema.Assistant,
			Content: "calling tool",
			ToolCalls: []schema.ToolCall{
				{ID: "call-1", Function: schema.FunctionCall{Name: "search"}},
			},
		},
	}

	groups := BuildMessageGroups(messages)

	require.Len(t, groups, 1)
	assert.Equal(t, MessageGroupTypeToolExchange, groups[0].Type)
	assert.False(t, groups[0].IsClosed)
	assert.Len(t, groups[0].Messages, 1)
}

func TestBuildMessageGroups_MultipleToolCalls(t *testing.T) {
	messages := []*schema.Message{
		{
			Role:    schema.Assistant,
			Content: "calling tools",
			ToolCalls: []schema.ToolCall{
				{ID: "call-1", Function: schema.FunctionCall{Name: "search"}},
				{ID: "call-2", Function: schema.FunctionCall{Name: "lookup"}},
			},
		},
		{Role: schema.Tool, ToolCallID: "call-1", Content: "result 1"},
		{Role: schema.Tool, ToolCallID: "call-2", Content: "result 2"},
	}

	groups := BuildMessageGroups(messages)

	require.Len(t, groups, 1)
	assert.Equal(t, MessageGroupTypeToolExchange, groups[0].Type)
	assert.True(t, groups[0].IsClosed)
	assert.Len(t, groups[0].Messages, 3) // call + 2 results
}

func TestBuildMessageGroups_MultipleToolCallsPartialResult(t *testing.T) {
	messages := []*schema.Message{
		{
			Role:    schema.Assistant,
			Content: "calling tools",
			ToolCalls: []schema.ToolCall{
				{ID: "call-1", Function: schema.FunctionCall{Name: "search"}},
				{ID: "call-2", Function: schema.FunctionCall{Name: "lookup"}},
			},
		},
		{Role: schema.Tool, ToolCallID: "call-1", Content: "result 1"},
	}

	groups := BuildMessageGroups(messages)

	require.Len(t, groups, 1)
	assert.Equal(t, MessageGroupTypeToolExchange, groups[0].Type)
	assert.False(t, groups[0].IsClosed) // call-2 没有 result
}

func TestFlattenGroups(t *testing.T) {
	messages := []*schema.Message{
		{Role: schema.User, Content: "hello"},
		{Role: schema.Assistant, Content: "hi"},
	}

	groups := BuildMessageGroups(messages)
	flattened := FlattenGroups(groups)

	assert.Equal(t, messages, flattened)
}

func TestFlattenGroups_ToolExchange(t *testing.T) {
	messages := []*schema.Message{
		{Role: schema.User, Content: "search"},
		{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{
				{ID: "call-1", Function: schema.FunctionCall{Name: "search"}},
			},
		},
		{Role: schema.Tool, ToolCallID: "call-1", Content: "results"},
	}

	groups := BuildMessageGroups(messages)
	flattened := FlattenGroups(groups)

	assert.Len(t, flattened, 3)
}

func TestBuildMessageGroups_NoResultLoss(t *testing.T) {
	// 验证不会保留 result 而丢弃 call
	messages := []*schema.Message{
		{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{
				{ID: "call-1", Function: schema.FunctionCall{Name: "search"}},
			},
		},
		{Role: schema.Tool, ToolCallID: "call-1", Content: "results"},
	}

	groups := BuildMessageGroups(messages)

	require.Len(t, groups, 1)
	assert.Len(t, groups[0].Messages, 2) // call + result 都在
}

func TestBuildMessageGroups_NoCallLoss(t *testing.T) {
	// 验证不会保留 call 而丢弃已存在的 result
	messages := []*schema.Message{
		{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{
				{ID: "call-1", Function: schema.FunctionCall{Name: "search"}},
			},
		},
		{Role: schema.Tool, ToolCallID: "call-1", Content: "results"},
	}

	groups := BuildMessageGroups(messages)

	require.Len(t, groups, 1)
	require.Len(t, groups[0].Messages, 2)
	assert.Equal(t, schema.Tool, groups[0].Messages[1].Role) // result 保留
}

func TestIsToolCallMessage(t *testing.T) {
	assert.True(t, IsToolCallMessage(&schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{
			{ID: "call-1"},
		},
	}))
	assert.False(t, IsToolCallMessage(&schema.Message{
		Role:    schema.Assistant,
		Content: "no tools",
	}))
	assert.False(t, IsToolCallMessage(&schema.Message{
		Role:    schema.User,
		Content: "user msg",
	}))
}

func TestIsToolResultMessage(t *testing.T) {
	assert.True(t, IsToolResultMessage(&schema.Message{
		Role:       schema.Tool,
		ToolCallID: "call-1",
	}))
	assert.False(t, IsToolResultMessage(&schema.Message{
		Role:    schema.User,
		Content: "user msg",
	}))
}
