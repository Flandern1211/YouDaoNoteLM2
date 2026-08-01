package agentcontext

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/schema"
)

// CompileModelInput 编译模型输入。
// 由 Eino Middleware 在每次模型调用前执行。
// 对相同输入产生确定结果。
func CompileModelInput(
	ctx context.Context,
	turn *PreparedTurn,
	messages []*schema.Message,
	toolInfos []*schema.ToolInfo,
) (*CompiledContext, error) {
	if turn == nil {
		return nil, NewError(ErrCodeInvalidInput, "turn 不能为空")
	}

	// 创建预算编译器
	budgetCompiler := NewBudgetCompiler(nil, ModelRef{
		Provider: string(turn.Profile.AgentID),
		ModelID:  turn.BaseManifest.Model,
	})

	// 执行预算编译
	result, compiledMessages, err := budgetCompiler.Compile(
		turn.Profile,
		turn.Instruction,
		turn.MessagePlan,
		toolInfos,
	)
	if err != nil {
		return nil, fmt.Errorf("预算编译失败: %w", err)
	}

	// 构建编译记录
	record := CompileRecord{
		ModelCallID: generateModelCallID(turn),
		Manifest: ContextManifest{
			ProfileID:       turn.Profile.Key.Name,
			ProfileVersion:  turn.Profile.Key.Version,
			PromptVersion:   turn.BaseManifest.PromptVersion,
			ToolsetVersion:  calculateToolsetVersion(toolInfos),
			Model:           turn.BaseManifest.Model,
			InputBudget:     calculateInputBudget(turn.Profile.Budget, ModelRef{}),
			EstimatedTokens: result.AfterTokens,
			CounterMode:     string(result.CounterMode),
			TurnStatus:      turn.BaseManifest.TurnStatus,
			Degraded:        turn.BaseManifest.Degraded,
			Sources:         buildSourceManifests(turn, result),
		},
	}

	return &CompiledContext{
		Messages: compiledMessages,
		Record:   record,
	}, nil
}

// generateModelCallID 生成模型调用 ID
func generateModelCallID(turn *PreparedTurn) string {
	if turn.Session != nil {
		return fmt.Sprintf("%s-%s", turn.Session.Handle.RunID, turn.Session.Handle.StepID)
	}
	return "unknown"
}

// calculateToolsetVersion 计算工具集版本
func calculateToolsetVersion(toolInfos []*schema.ToolInfo) string {
	if len(toolInfos) == 0 {
		return "empty"
	}
	return fmt.Sprintf("tools-%d", len(toolInfos))
}

// buildSourceManifests 构建来源清单
func buildSourceManifests(turn *PreparedTurn, result *CompileGovernanceResult) []SourceManifest {
	var sources []SourceManifest

	// Prompt 来源
	sources = append(sources, SourceManifest{
		Provider:       "prompt",
		CandidateCount: 1,
		SelectedCount:  1,
		Status:         "selected",
	})

	// Summary 来源
	if turn.MessagePlan.Summary != nil {
		sources = append(sources, SourceManifest{
			Provider:       "summary",
			CandidateCount: 1,
			SelectedCount:  1,
			Status:         "selected",
		})
	}

	// Memory 来源
	if len(turn.MessagePlan.Memories) > 0 {
		sources = append(sources, SourceManifest{
			Provider:       "memory",
			CandidateCount: len(turn.MessagePlan.Memories),
			SelectedCount:  len(turn.MessagePlan.Memories),
			Status:         "selected",
		})
	}

	// History 来源
	if len(turn.MessagePlan.History) > 0 {
		dropped := len(result.DroppedItems)
		selected := len(turn.MessagePlan.History) - dropped
		if selected < 0 {
			selected = 0
		}
		sources = append(sources, SourceManifest{
			Provider:       "history",
			CandidateCount: len(turn.MessagePlan.History),
			SelectedCount:  selected,
			Status:         getStatus(dropped > 0),
		})
	}

	return sources
}

// getStatus 根据是否有淘汰返回状态
func getStatus(hasDropped bool) string {
	if hasDropped {
		return "partial"
	}
	return "selected"
}
