package agentcontext

import (
	"context"
	"time"

	"github.com/cloudwego/eino/schema"
)

// DefaultCompiler 将 PrepareTurn 和 CompileModelInput 组合成可注入实现。
type DefaultCompiler struct {
	prepareConfig PrepareTurnConfig
	fingerprinter *ContextFingerprinter
	metrics       ContextMetrics
}

// NewDefaultCompiler 创建默认上下文编译器。
func NewDefaultCompiler(
	prepareConfig PrepareTurnConfig,
	fingerprinter *ContextFingerprinter,
	metrics ContextMetrics,
) *DefaultCompiler {
	if metrics == nil {
		metrics = &NoopMetrics{}
	}
	return &DefaultCompiler{
		prepareConfig: prepareConfig,
		fingerprinter: fingerprinter,
		metrics:       metrics,
	}
}

// PrepareTurn 加载并固化本轮上下文候选。
func (c *DefaultCompiler) PrepareTurn(
	ctx context.Context,
	session *TurnSession,
	req PrepareTurnRequest,
) (*PreparedTurn, error) {
	startedAt := time.Now()
	turn, err := PrepareTurn(ctx, session, req, c.prepareConfig)
	if session != nil {
		c.metrics.RecordPrepareDuration(session.Handle.AgentID, time.Since(startedAt))
	}
	return turn, err
}

// CompileModelInput 编译一次模型调用输入并生成无正文指纹。
func (c *DefaultCompiler) CompileModelInput(
	ctx context.Context,
	req CompileRequest,
) (*CompiledContext, error) {
	startedAt := time.Now()
	turn := turnWithRuntimeState(req.Turn, req.Messages)
	compiled, err := CompileModelInput(ctx, turn, req.Messages, append(req.ToolInfos, req.DeferredToolInfos...))
	if req.Turn != nil {
		c.metrics.RecordCompileDuration(req.Turn.Profile.AgentID, time.Since(startedAt))
	}
	if err != nil {
		return nil, err
	}

	if c.fingerprinter != nil {
		fingerprint := c.fingerprinter.GenerateCompiledFingerprint(
			compiled.Record.Manifest,
			compiled.Messages,
			append(req.ToolInfos, req.DeferredToolInfos...),
		)
		compiled.Record.ContextHMAC = fingerprint.Fingerprint
		compiled.Record.Manifest.ContextHMAC = fingerprint.Fingerprint
	}
	return compiled, nil
}

func turnWithRuntimeState(turn *PreparedTurn, messages []*schema.Message) *PreparedTurn {
	if turn == nil {
		return nil
	}
	cloned := *turn
	cloned.MessagePlan = turn.MessagePlan

	if len(messages) > 0 && messages[0] != nil && messages[0].Role == schema.System {
		cloned.Instruction = messages[0].Content
	}

	currentContent := turnInputContent(turn.MessagePlan.CurrentInput)
	currentIndex := -1
	for i, message := range messages {
		if message != nil && message.Role == schema.User && message.Content == currentContent {
			currentIndex = i
		}
	}
	if currentIndex >= 0 && currentIndex+1 < len(messages) {
		cloned.MessagePlan.RuntimeMessages = append(
			[]*schema.Message(nil),
			messages[currentIndex+1:]...,
		)
	}
	return &cloned
}

func turnInputContent(input TurnInput) string {
	switch value := input.(type) {
	case UserMessageInput:
		return value.Content
	case SearchTaskInput:
		return "<search_task>\nquery: " + value.Task.Query + "\n</search_task>"
	default:
		return ""
	}
}
