package eino

import (
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"YoudaoNoteLm/internal/agentcontext"
)

func TestAgentInputBuilder_BuildMessages_WithHistory(t *testing.T) {
	builder := NewAgentInputBuilder()

	turn := &agentcontext.PreparedTurn{
		MessagePlan: agentcontext.MessagePlan{
			History: []*schema.Message{
				schema.UserMessage("hello"),
				schema.AssistantMessage("hi there", nil),
			},
			CurrentInput: agentcontext.UserMessageInput{Content: "how are you?"},
		},
	}

	messages, err := builder.BuildMessages(turn)
	require.NoError(t, err)
	assert.Len(t, messages, 3) // 2 history + 1 final user
	assert.Equal(t, schema.User, messages[0].Role)
	assert.Equal(t, "hello", messages[0].Content)
	assert.Equal(t, schema.Assistant, messages[1].Role)
	assert.Equal(t, "hi there", messages[1].Content)
	assert.Equal(t, schema.User, messages[2].Role)
	assert.Equal(t, "how are you?", messages[2].Content)
}

func TestAgentInputBuilder_BuildMessages_WithSummary(t *testing.T) {
	builder := NewAgentInputBuilder()

	turn := &agentcontext.PreparedTurn{
		MessagePlan: agentcontext.MessagePlan{
			Summary: &agentcontext.ContextItem{
				ID:      "summary",
				Content: "previous conversation about Go",
			},
			CurrentInput: agentcontext.UserMessageInput{Content: "continue"},
		},
	}

	messages, err := builder.BuildMessages(turn)
	require.NoError(t, err)
	assert.Len(t, messages, 1)
	assert.Contains(t, messages[0].Content, "<conversation_summary>")
	assert.Contains(t, messages[0].Content, "previous conversation about Go")
	assert.Contains(t, messages[0].Content, "continue")
}

func TestAgentInputBuilder_BuildMessages_WithMemories(t *testing.T) {
	builder := NewAgentInputBuilder()

	turn := &agentcontext.PreparedTurn{
		MessagePlan: agentcontext.MessagePlan{
			Memories: []agentcontext.ContextItem{
				{ID: "mem1", Content: "user prefers Chinese"},
				{ID: "mem2", Content: "user is a developer"},
			},
			CurrentInput: agentcontext.UserMessageInput{Content: "hello"},
		},
	}

	messages, err := builder.BuildMessages(turn)
	require.NoError(t, err)
	assert.Len(t, messages, 1)
	assert.Contains(t, messages[0].Content, "<user_memories>")
	assert.Contains(t, messages[0].Content, "user prefers Chinese")
	assert.Contains(t, messages[0].Content, "user is a developer")
}

func TestAgentInputBuilder_BuildMessages_SearchTask(t *testing.T) {
	builder := NewAgentInputBuilder()

	turn := &agentcontext.PreparedTurn{
		MessagePlan: agentcontext.MessagePlan{
			CurrentInput: agentcontext.SearchTaskInput{
				Task: agentcontext.SearchTask{Query: "Go context management"},
			},
		},
	}

	messages, err := builder.BuildMessages(turn)
	require.NoError(t, err)
	assert.Len(t, messages, 1)
	assert.Contains(t, messages[0].Content, "<search_task>")
	assert.Contains(t, messages[0].Content, "Go context management")
}

func TestAgentInputBuilder_BuildMessages_NilTurn(t *testing.T) {
	builder := NewAgentInputBuilder()
	_, err := builder.BuildMessages(nil)
	require.Error(t, err)
}

func TestAgentInputBuilder_BuildMessages_EmptyPlan(t *testing.T) {
	builder := NewAgentInputBuilder()

	turn := &agentcontext.PreparedTurn{
		MessagePlan: agentcontext.MessagePlan{},
	}

	messages, err := builder.BuildMessages(turn)
	require.NoError(t, err)
	assert.Len(t, messages, 0) // 没有内容，不生成消息
}
