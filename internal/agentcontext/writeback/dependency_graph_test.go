package writeback

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"YoudaoNoteLm/internal/agentcontext"
)

func TestWritebackGraph_ConversationTurn_Success(t *testing.T) {
	graph := NewWritebackGraph(
		agentcontext.WritebackPolicyConversationTurn,
		agentcontext.TurnStatusSuccess,
	)

	plan, err := graph.Plan()
	require.NoError(t, err)

	// 成功路径：Assistant 在第一阶段，(Summary, Memory, Manifest) 在第二阶段
	require.Len(t, plan.Stages, 2)
	assert.Contains(t, plan.Stages[0], WritebackOperationAssistant)
	assert.Contains(t, plan.Stages[1], WritebackOperationSummary)
	assert.Contains(t, plan.Stages[1], WritebackOperationMemory)
	assert.Contains(t, plan.Stages[1], WritebackOperationManifest)
}

func TestWritebackGraph_ConversationTurn_Failed(t *testing.T) {
	graph := NewWritebackGraph(
		agentcontext.WritebackPolicyConversationTurn,
		agentcontext.TurnStatusFailed,
	)

	plan, err := graph.Plan()
	require.NoError(t, err)

	// 失败路径：只有 Manifest
	require.Len(t, plan.Stages, 1)
	assert.Contains(t, plan.Stages[0], WritebackOperationManifest)
}

func TestWritebackGraph_StepResult_Success(t *testing.T) {
	graph := NewWritebackGraph(
		agentcontext.WritebackPolicyStepResult,
		agentcontext.TurnStatusSuccess,
	)

	plan, err := graph.Plan()
	require.NoError(t, err)

	// Search 成功路径：StepResult 在第一阶段，Manifest 在第二阶段
	require.Len(t, plan.Stages, 2)
	assert.Contains(t, plan.Stages[0], WritebackOperationStepResult)
	assert.Contains(t, plan.Stages[1], WritebackOperationManifest)
}

func TestWritebackGraph_StepResult_Cancelled(t *testing.T) {
	graph := NewWritebackGraph(
		agentcontext.WritebackPolicyStepResult,
		agentcontext.TurnStatusCancelled,
	)

	plan, err := graph.Plan()
	require.NoError(t, err)

	// 取消路径：只有 Manifest
	require.Len(t, plan.Stages, 1)
	assert.Contains(t, plan.Stages[0], WritebackOperationManifest)
}

func TestWritebackGraph_EmptyPolicy(t *testing.T) {
	graph := NewWritebackGraph("", agentcontext.TurnStatusSuccess)

	plan, err := graph.Plan()
	require.NoError(t, err)
	assert.Empty(t, plan.Stages)
}

func TestWritebackGraph_GetRequiredOperations(t *testing.T) {
	graph := NewWritebackGraph(
		agentcontext.WritebackPolicyConversationTurn,
		agentcontext.TurnStatusSuccess,
	)

	required := graph.GetRequiredOperations()
	assert.Contains(t, required, WritebackOperationAssistant)
	assert.Contains(t, required, WritebackOperationManifest)
	assert.NotContains(t, required, WritebackOperationSummary)
	assert.NotContains(t, required, WritebackOperationMemory)
}

func TestWritebackGraph_CanSkip(t *testing.T) {
	graph := NewWritebackGraph(
		agentcontext.WritebackPolicyConversationTurn,
		agentcontext.TurnStatusSuccess,
	)

	assert.False(t, graph.CanSkip(WritebackOperationAssistant))
	assert.True(t, graph.CanSkip(WritebackOperationSummary))
	assert.True(t, graph.CanSkip(WritebackOperationMemory))
	assert.False(t, graph.CanSkip(WritebackOperationManifest))
	assert.True(t, graph.CanSkip("unknown"))
}

func TestWritebackGraph_GetMaxRetries(t *testing.T) {
	graph := NewWritebackGraph(
		agentcontext.WritebackPolicyConversationTurn,
		agentcontext.TurnStatusSuccess,
	)

	assert.Equal(t, 2, graph.GetMaxRetries(WritebackOperationAssistant))
	assert.Equal(t, 1, graph.GetMaxRetries(WritebackOperationSummary))
	assert.Equal(t, 0, graph.GetMaxRetries("unknown"))
}

func TestWritebackGraph_TopologicalSort_NoCycle(t *testing.T) {
	graph := NewWritebackGraph(
		agentcontext.WritebackPolicyConversationTurn,
		agentcontext.TurnStatusSuccess,
	)

	_, err := graph.Plan()
	assert.NoError(t, err) // 无循环依赖
}
