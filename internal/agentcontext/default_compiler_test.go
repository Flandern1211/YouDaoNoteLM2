package agentcontext

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultCompiler_AddsManifestHMAC(t *testing.T) {
	turn := &PreparedTurn{
		Session: &TurnSession{
			Handle: AcceptedTurnHandle{RunID: "run-1", StepID: "step-0"},
		},
		Profile: ContextProfileSnapshot{
			Key:     ChatV1,
			AgentID: AgentIDChat,
			Budget:  DefaultBudgetConfig(),
		},
		Instruction: "system",
		MessagePlan: MessagePlan{
			CurrentInput: UserMessageInput{Content: "hello"},
		},
		BaseManifest: ContextManifest{
			ProfileID:      ChatV1.Name,
			ProfileVersion: ChatV1.Version,
			PromptVersion:  "v1",
			Model:          "openai/test",
			TurnStatus:     "prepared",
		},
	}
	compiler := NewDefaultCompiler(PrepareTurnConfig{}, NewContextFingerprinter("test-salt"), nil)

	compiled, err := compiler.CompileModelInput(context.Background(), CompileRequest{
		Turn:     turn,
		Messages: []*schema.Message{schema.UserMessage("legacy")},
	})

	require.NoError(t, err)
	assert.NotEmpty(t, compiled.Record.ContextHMAC)
	assert.Equal(t, compiled.Record.ContextHMAC, compiled.Record.Manifest.ContextHMAC)
	assert.NotContains(t, compiled.Record.ContextHMAC, "hello")
}

func TestDefaultCompiler_PreservesRuntimeToolExchangeAndActualInstruction(t *testing.T) {
	turn := &PreparedTurn{
		Session: &TurnSession{
			Handle: AcceptedTurnHandle{RunID: "run-1", StepID: "step-0"},
		},
		Profile: ContextProfileSnapshot{
			Key:     ChatV1,
			AgentID: AgentIDChat,
			Budget:  DefaultBudgetConfig(),
		},
		Instruction: "stale instruction",
		MessagePlan: MessagePlan{
			CurrentInput: UserMessageInput{Content: "hello"},
		},
		BaseManifest: ContextManifest{Model: "openai/test"},
	}
	compiler := NewDefaultCompiler(PrepareTurnConfig{}, nil, nil)
	toolCall := schema.AssistantMessage("", []schema.ToolCall{{
		ID: "call-1",
		Function: schema.FunctionCall{
			Name:      "search",
			Arguments: `{"q":"hello"}`,
		},
	}})
	toolResult := schema.ToolMessage("result", "call-1")

	compiled, err := compiler.CompileModelInput(context.Background(), CompileRequest{
		Turn: turn,
		Messages: []*schema.Message{
			schema.SystemMessage("actual instruction with sources"),
			schema.UserMessage("hello"),
			toolCall,
			toolResult,
		},
	})

	require.NoError(t, err)
	require.Len(t, compiled.Messages, 4)
	assert.Equal(t, "actual instruction with sources", compiled.Messages[0].Content)
	assert.Equal(t, schema.User, compiled.Messages[1].Role)
	assert.Equal(t, toolCall, compiled.Messages[2])
	assert.Equal(t, toolResult, compiled.Messages[3])
	assert.Equal(t, "stale instruction", turn.Instruction, "编译不得修改请求级 PreparedTurn")
}
