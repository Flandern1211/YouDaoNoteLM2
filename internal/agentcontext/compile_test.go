package agentcontext

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompileModelInput_NilTurn(t *testing.T) {
	_, err := CompileModelInput(context.Background(), nil, nil, nil)
	require.Error(t, err)
	assert.True(t, IsErrorCode(err, ErrCodeInvalidInput))
}

func TestCompileModelInput_BasicCompile(t *testing.T) {
	profile := NewChatV1Profile()
	turn := &PreparedTurn{
		Session: &TurnSession{
			Handle: AcceptedTurnHandle{
				RunID:  "run-1",
				StepID: "step-1",
			},
		},
		Profile:     profile.ToSnapshot(),
		Instruction: "system prompt",
		MessagePlan: MessagePlan{
			Summary: &ContextItem{
				ID:      "summary",
				Content: "previous conversation summary",
			},
			History: []*schema.Message{
				schema.UserMessage("hello"),
				schema.AssistantMessage("hi", nil),
			},
			CurrentInput: UserMessageInput{Content: "how are you"},
		},
		BaseManifest: ContextManifest{
			ProfileID:      "chat",
			ProfileVersion: "v1",
			PromptVersion:  "v1",
			Model:          "openai/gpt-4o",
		},
	}

	compiled, err := CompileModelInput(context.Background(), turn, nil, nil)

	require.NoError(t, err)
	assert.NotNil(t, compiled)
	assert.NotEmpty(t, compiled.Messages)
	assert.NotEmpty(t, compiled.Record.Manifest.ProfileID)
	assert.Equal(t, "chat", compiled.Record.Manifest.ProfileID)
}

func TestCompileModelInput_WithToolInfos(t *testing.T) {
	profile := NewChatV1Profile()
	turn := &PreparedTurn{
		Session: &TurnSession{
			Handle: AcceptedTurnHandle{
				RunID:  "run-1",
				StepID: "step-1",
			},
		},
		Profile:     profile.ToSnapshot(),
		Instruction: "system prompt",
		MessagePlan: MessagePlan{
			CurrentInput: UserMessageInput{Content: "search for X"},
		},
		BaseManifest: ContextManifest{
			Model: "openai/gpt-4o",
		},
	}

	toolInfos := []*schema.ToolInfo{
		{Name: "search_knowledge", Desc: "search knowledge base"},
	}

	compiled, err := CompileModelInput(context.Background(), turn, nil, toolInfos)

	require.NoError(t, err)
	assert.NotNil(t, compiled)
	assert.Equal(t, "tools-1", compiled.Record.Manifest.ToolsetVersion)
}

func TestCompileModelInput_SearchProfile(t *testing.T) {
	profile := NewSearchV1Profile()
	turn := &PreparedTurn{
		Session: &TurnSession{
			Handle: AcceptedTurnHandle{
				RunID:  "run-1",
				StepID: "step-1",
			},
		},
		Profile:     profile.ToSnapshot(),
		Instruction: "search prompt",
		MessagePlan: MessagePlan{
			CurrentInput: SearchTaskInput{Task: SearchTask{Query: "find documents"}},
		},
		BaseManifest: ContextManifest{
			Model: "openai/gpt-4o",
		},
	}

	compiled, err := CompileModelInput(context.Background(), turn, nil, nil)

	require.NoError(t, err)
	assert.NotNil(t, compiled)
	assert.NotEmpty(t, compiled.Messages)

	// 验证搜索任务被正确格式化
	found := false
	for _, msg := range compiled.Messages {
		if msg.Role == schema.User {
			assert.Contains(t, msg.Content, "search_task")
			assert.Contains(t, msg.Content, "find documents")
			found = true
		}
	}
	assert.True(t, found, "应该包含搜索任务消息")
}

func TestCompileModelInput_DeterministicOutput(t *testing.T) {
	// 验证相同输入产生确定输出
	profile := NewChatV1Profile()
	turn := &PreparedTurn{
		Session: &TurnSession{
			Handle: AcceptedTurnHandle{
				RunID:  "run-1",
				StepID: "step-1",
			},
		},
		Profile:     profile.ToSnapshot(),
		Instruction: "system prompt",
		MessagePlan: MessagePlan{
			History: []*schema.Message{
				schema.UserMessage("hello"),
				schema.AssistantMessage("hi", nil),
			},
			CurrentInput: UserMessageInput{Content: "how are you"},
		},
		BaseManifest: ContextManifest{
			Model: "openai/gpt-4o",
		},
	}

	// 编译两次
	compiled1, err := CompileModelInput(context.Background(), turn, nil, nil)
	require.NoError(t, err)

	compiled2, err := CompileModelInput(context.Background(), turn, nil, nil)
	require.NoError(t, err)

	// 验证结果一致
	assert.Equal(t, len(compiled1.Messages), len(compiled2.Messages))
	assert.Equal(t, compiled1.Record.Manifest.InputBudget, compiled2.Record.Manifest.InputBudget)
	assert.Equal(t, compiled1.Record.Manifest.EstimatedTokens, compiled2.Record.Manifest.EstimatedTokens)
}

func TestGenerateModelCallID(t *testing.T) {
	turn := &PreparedTurn{
		Session: &TurnSession{
			Handle: AcceptedTurnHandle{
				RunID:  "run-123",
				StepID: "step-456",
			},
		},
	}

	id := generateModelCallID(turn)
	assert.Equal(t, "run-123-step-456", id)
}

func TestGenerateModelCallID_NilSession(t *testing.T) {
	turn := &PreparedTurn{}
	id := generateModelCallID(turn)
	assert.Equal(t, "unknown", id)
}

func TestCalculateToolsetVersion_Empty(t *testing.T) {
	version := calculateToolsetVersion(nil)
	assert.Equal(t, "empty", version)
}

func TestCalculateToolsetVersion_WithTools(t *testing.T) {
	toolInfos := []*schema.ToolInfo{
		{Name: "tool1"},
		{Name: "tool2"},
	}

	version := calculateToolsetVersion(toolInfos)
	assert.Equal(t, "tools-2", version)
}

func TestGetStatus(t *testing.T) {
	assert.Equal(t, "partial", getStatus(true))
	assert.Equal(t, "selected", getStatus(false))
}
