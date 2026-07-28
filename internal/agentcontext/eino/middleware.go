package eino

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"YoudaoNoteLm/internal/agentcontext"
	"YoudaoNoteLm/pkg/logger"

	"go.uber.org/zap"
)

// ContextMode 上下文模式
type ContextMode string

const (
	// ContextModeLegacy 使用 Legacy ContextBuilder 生成模型输入
	ContextModeLegacy ContextMode = "legacy"
	// ContextModeShadow Legacy 生成真实输入，Shadow 并行编译并比较
	ContextModeShadow ContextMode = "shadow"
	// ContextModeEnabled 完全启用新编译器（W6+ 才可用）
	ContextModeEnabled ContextMode = "enabled"
)

// TurnPreparedTurn turn 级 PreparedTurn 存储
type TurnPreparedTurn struct {
	Turn *agentcontext.PreparedTurn
}

// ContextMiddlewareConfig ContextMiddleware 配置
type ContextMiddlewareConfig struct {
	// Compiler 上下文编译器
	Compiler agentcontext.ContextCompiler
	// Mode 运行模式（legacy 或 shadow）
	Mode ContextMode
	// ShadowSink Shadow 比较记录的接收端（可选）
	ShadowSink func(ShadowCompareRecord)
}

// ContextMiddleware 实现 ChatModelAgentMiddleware，在每次模型调用前执行上下文编译。
// 固定注册顺序：context compiler rewrite → metrics/tracing → 其他业务 handler。
type ContextMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	compiler   agentcontext.ContextCompiler
	mode       ContextMode
	shadowSink func(ShadowCompareRecord)
}

// NewContextMiddleware 创建 ContextMiddleware。
// enabled 模式在 W4 必须返回 nil，不能半启用。
func NewContextMiddleware(cfg ContextMiddlewareConfig) *ContextMiddleware {
	// W4 不支持 enabled 模式，必须明确拒绝
	if cfg.Mode == ContextModeEnabled {
		logger.Error("[ContextMiddleware] enabled 模式在 W4 不可用，请使用 legacy 或 shadow")
		return nil
	}

	return &ContextMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		compiler:                     cfg.Compiler,
		mode:                         cfg.Mode,
		shadowSink:                   cfg.ShadowSink,
	}
}

// BeforeModelRewriteState 在每次模型调用前执行上下文编译。
// 根据 mode 决定 Legacy/Shadow 行为：
//   - Legacy: 不修改 state（现有 ContextBuilder 已在 Agent 外部构建消息）
//   - Shadow: 使用 ContextCompiler 编译新消息，但只比较不替换
func (m *ContextMiddleware) BeforeModelRewriteState(
	ctx context.Context,
	state *adk.ChatModelAgentState,
	mc *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
	// 从 context 获取 PreparedTurn（由 LegacyTurnAdapter 或测试注入）
	turn := getPreparedTurnFromContext(ctx)
	if turn == nil {
		// 没有 PreparedTurn，跳过编译（Legacy 模式或未配置）
		return ctx, state, nil
	}

	switch m.mode {
	case ContextModeShadow:
		return m.handleShadow(ctx, state, turn)
	default:
		// Legacy 模式：不修改 state
		return ctx, state, nil
	}
}

// handleShadow 执行 Shadow 编译并比较结果。
func (m *ContextMiddleware) handleShadow(
	ctx context.Context,
	state *adk.ChatModelAgentState,
	turn *agentcontext.PreparedTurn,
) (context.Context, *adk.ChatModelAgentState, error) {
	// Shadow 编译：使用 ContextCompiler 生成新的编译结果
	compiled, err := m.compiler.CompileModelInput(ctx, agentcontext.CompileRequest{
		Turn:              turn,
		Messages:          state.Messages,
		ToolInfos:         state.ToolInfos,
		DeferredToolInfos: state.DeferredToolInfos,
	})
	if err != nil {
		// Shadow 错误只记录，不改变用户请求结果
		logger.Warn("[ContextMiddleware] Shadow 编译失败",
			zap.Error(err),
		)
		if m.shadowSink != nil {
			m.shadowSink(ShadowCompareRecord{
				TurnID: turn.Session.Handle.RunID,
				Error:  err.Error(),
			})
		}
		return ctx, state, nil
	}

	// 比较 Legacy 和 Shadow 结果
	record := compareLegacyShadow(state.Messages, compiled.Messages, turn, compiled.Record)
	if m.shadowSink != nil {
		m.shadowSink(record)
	}

	// Shadow 不修改 state，模型仍消费 Legacy 输入
	return ctx, state, nil
}

// ShadowCompareRecord Shadow 比较记录
type ShadowCompareRecord struct {
	TurnID           string
	LegacyMsgCount   int
	ShadowMsgCount   int
	LegacyEstTokens  int
	ShadowEstTokens  int
	RoleOrderMatch   bool
	TruncateDecision string
	Error            string
}

// compareLegacyShadow 比较 Legacy 和 Shadow 的消息列表。
func compareLegacyShadow(
	legacy []*schema.Message,
	shadow []*schema.Message,
	turn *agentcontext.PreparedTurn,
	record agentcontext.CompileRecord,
) ShadowCompareRecord {
	result := ShadowCompareRecord{
		TurnID:         turn.Session.Handle.RunID,
		LegacyMsgCount: len(legacy),
		ShadowMsgCount: len(shadow),
	}

	// 比较角色顺序
	result.RoleOrderMatch = compareRoleOrder(legacy, shadow)

	// 估算 token 数
	result.LegacyEstTokens = estimateMessagesTokens(legacy)
	result.ShadowEstTokens = record.Manifest.EstimatedTokens

	// 截断决策
	if record.Manifest.TurnStatus == "governed" {
		result.TruncateDecision = "governed"
	} else {
		result.TruncateDecision = "none"
	}

	return result
}

// compareRoleOrder 比较两组消息的角色顺序是否一致。
func compareRoleOrder(a, b []*schema.Message) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Role != b[i].Role {
			return false
		}
	}
	return true
}

// estimateMessagesTokens 保守估算消息列表的 token 数。
func estimateMessagesTokens(messages []*schema.Message) int {
	total := 0
	for _, msg := range messages {
		total += 4 // 固定开销
		total += len([]byte(msg.Content)) / 3
		for _, tc := range msg.ToolCalls {
			total += 10
			total += len([]byte(tc.Function.Name)) / 3
			total += len([]byte(tc.Function.Arguments)) / 3
		}
		if msg.ToolCallID != "" {
			total += 10
		}
	}
	return total
}

// PrepareAndInject 执行 PrepareTurn 并将 PreparedTurn 注入 context。
// 由 LegacyTurnAdapter 在 Agent invocation 开始时调用。
func PrepareAndInject(
	ctx context.Context,
	compiler agentcontext.ContextCompiler,
	session *agentcontext.TurnSession,
	req agentcontext.PrepareTurnRequest,
) (context.Context, *agentcontext.PreparedTurn, error) {
	turn, err := compiler.PrepareTurn(ctx, session, req)
	if err != nil {
		return ctx, nil, fmt.Errorf("PrepareTurn 失败: %w", err)
	}
	return context.WithValue(ctx, contextCompilerKey{}, turn), turn, nil
}

// getPreparedTurnFromContext 从 context 中获取 PreparedTurn。
func getPreparedTurnFromContext(ctx context.Context) *agentcontext.PreparedTurn {
	turn, _ := ctx.Value(contextCompilerKey{}).(*agentcontext.PreparedTurn)
	return turn
}
