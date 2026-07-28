package writeback

import (
	"context"
	"fmt"

	"YoudaoNoteLm/internal/agentcontext"
	"YoudaoNoteLm/pkg/logger"

	"go.uber.org/zap"
)

// TurnVerifier Turn 验证器端口。
// 由未来 RunService/Harness 适配器实现。
// 核心逻辑只消费验证结果，不了解查询实现。
type TurnVerifier interface {
	// VerifyAccepted 验证 AcceptedTurnHandle 对应持久化且未终止的 Run。
	VerifyAccepted(ctx context.Context, handle agentcontext.AcceptedTurnHandle) (VerifiedTurn, error)

	// VerifyAuthority 验证当前 Attempt 的执行权。
	VerifyAuthority(ctx context.Context, attemptID string, authority agentcontext.ActiveExecutionAuthority) error
}

// VerifiedTurn 已验证的 Turn
type VerifiedTurn struct {
	Handle  agentcontext.AcceptedTurnHandle
	Profile agentcontext.ContextProfileSnapshot
}

// WriterRegistry Writer 注册表。
// 由应用层在启动时注入。
type WriterRegistry struct {
	Assistant  AssistantMessageWriter
	Summary    SummaryWriter
	Memory     MemoryWriter
	Manifest   ManifestWriter
	StepResult StepResultWriter
}

// CoordinatorConfig Coordinator 配置
type CoordinatorConfig struct {
	Verifier TurnVerifier
	Writers  WriterRegistry
}

// TurnLifecycleCoordinator Turn 生命周期协调器核心实现。
// 负责 BeginTurn 验证和 FinalizeTurn 写回调度。
// 不直接访问数据库或外部服务。
type TurnLifecycleCoordinator struct {
	verifier TurnVerifier
	writers  WriterRegistry
}

// NewTurnLifecycleCoordinator 创建 TurnLifecycleCoordinator。
func NewTurnLifecycleCoordinator(cfg CoordinatorConfig) *TurnLifecycleCoordinator {
	return &TurnLifecycleCoordinator{
		verifier: cfg.Verifier,
		writers:  cfg.Writers,
	}
}

// BeginTurn 验证持久化 Handle 和执行权，返回 TurnSession。
// 验证失败不得调用 Provider、模型或 Writer。
func (c *TurnLifecycleCoordinator) BeginTurn(
	ctx context.Context,
	req agentcontext.BeginTurnRequest,
) (*agentcontext.TurnSession, error) {
	// 1. 验证 Handle
	verified, err := c.verifier.VerifyAccepted(ctx, req.Handle)
	if err != nil {
		return nil, fmt.Errorf("Handle 验证失败: %w", err)
	}

	// 2. 验证 Authority
	if err := c.verifier.VerifyAuthority(ctx, req.Handle.RunID, req.Authority); err != nil {
		return nil, fmt.Errorf("Authority 验证失败: %w", err)
	}

	return &agentcontext.TurnSession{
		Handle:  verified.Handle,
		Profile: verified.Profile,
	}, nil
}

// FinalizeTurn 覆盖成功、失败、取消等终态。
// 验证 Authority，调度写回依赖图。
func (c *TurnLifecycleCoordinator) FinalizeTurn(
	ctx context.Context,
	req agentcontext.FinalizeRequest,
) (*agentcontext.FinalizeResult, error) {
	// 1. 验证 Finalize 的 Authority
	if err := c.verifier.VerifyAuthority(ctx, req.Turn.Session.Handle.RunID, req.Authority); err != nil {
		return nil, fmt.Errorf("Finalize Authority 验证失败: %w", err)
	}

	// 2. 构建写回依赖图
	policy := req.Turn.Profile.WritebackPolicy
	graph := NewWritebackGraph(policy, req.Outcome.Status)

	// 3. 生成执行计划
	plan, err := graph.Plan()
	if err != nil {
		return nil, fmt.Errorf("生成执行计划失败: %w", err)
	}

	// 4. 执行写回
	result := &agentcontext.FinalizeResult{}
	ticket := FinalizationTicket{
		ID:  fmt.Sprintf("ticket-%s-%d", req.FinalizeKey.RunID, req.FinalizeKey.Revision),
		Key: req.FinalizeKey,
	}

	for _, stage := range plan.Stages {
		for _, op := range stage {
			c.executeWriteback(ctx, op, req, ticket, result, graph)
		}
	}

	return result, nil
}

// executeWriteback 执行单个写回操作
func (c *TurnLifecycleCoordinator) executeWriteback(
	ctx context.Context,
	op WritebackOperation,
	req agentcontext.FinalizeRequest,
	ticket FinalizationTicket,
	result *agentcontext.FinalizeResult,
	graph *WritebackGraph,
) {
	idempotencyKey := fmt.Sprintf("%s-%d-%s", req.FinalizeKey.RunID, req.FinalizeKey.Revision, op)

	switch op {
	case WritebackOperationAssistant:
		c.executeAssistant(ctx, req, ticket, idempotencyKey, result)
	case WritebackOperationStepResult:
		c.executeStepResult(ctx, req, ticket, idempotencyKey, result)
	case WritebackOperationSummary:
		c.executeSummary(ctx, req, ticket, idempotencyKey, result)
	case WritebackOperationMemory:
		c.executeMemory(ctx, req, ticket, idempotencyKey, result)
	case WritebackOperationManifest:
		c.executeManifest(ctx, req, ticket, idempotencyKey, result)
	}
}

// executeAssistant 执行助手消息写入
func (c *TurnLifecycleCoordinator) executeAssistant(
	ctx context.Context,
	req agentcontext.FinalizeRequest,
	ticket FinalizationTicket,
	idempotencyKey string,
	result *agentcontext.FinalizeResult,
) {
	if c.writers.Assistant == nil {
		result.Primary = nil
		return
	}

	output, ok := req.Outcome.PrimaryOutput.(agentcontext.ConversationOutput)
	if !ok {
		logger.Warn("[Coordinator] 成功 Turn 缺少 ConversationOutput")
		return
	}

	_, err := c.writers.Assistant.CommitAssistant(ctx, AssistantWriteRequest{
		FinalizeKey:    req.FinalizeKey,
		Ticket:         ticket,
		Authority:      req.Authority,
		Content:        output.FinalMessage.Content,
		IdempotencyKey: idempotencyKey,
		ProfileID:      req.Turn.Profile.Key.Name,
	})
	if err != nil {
		logger.Error("[Coordinator] Assistant 写入失败", zap.Error(err))
		// 主结果失败，后续派生操作跳过
	}
}

// executeStepResult 执行步骤结果写入
func (c *TurnLifecycleCoordinator) executeStepResult(
	ctx context.Context,
	req agentcontext.FinalizeRequest,
	ticket FinalizationTicket,
	idempotencyKey string,
	result *agentcontext.FinalizeResult,
) {
	if c.writers.StepResult == nil {
		return
	}

	_, err := c.writers.StepResult.CommitStepResult(ctx, StepResultWriteRequest{
		FinalizeKey:    req.FinalizeKey,
		Ticket:         ticket,
		Authority:      req.Authority,
		IdempotencyKey: idempotencyKey,
		ProfileID:      req.Turn.Profile.Key.Name,
	})
	if err != nil {
		logger.Error("[Coordinator] StepResult 写入失败", zap.Error(err))
	}
}

// executeSummary 执行摘要写入
func (c *TurnLifecycleCoordinator) executeSummary(
	ctx context.Context,
	req agentcontext.FinalizeRequest,
	ticket FinalizationTicket,
	idempotencyKey string,
	result *agentcontext.FinalizeResult,
) {
	if c.writers.Summary == nil {
		result.Summary = agentcontext.WritebackStatusSkipped
		return
	}

	r, err := c.writers.Summary.EvaluateAndUpdate(ctx, SummaryWriteRequest{
		FinalizeKey:    req.FinalizeKey,
		Ticket:         ticket,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		logger.Warn("[Coordinator] Summary 写入失败", zap.Error(err))
		result.Summary = agentcontext.WritebackStatusFailed
		return
	}
	result.Summary = agentcontext.WritebackStatus(r.Status)
}

// executeMemory 执行记忆写入
func (c *TurnLifecycleCoordinator) executeMemory(
	ctx context.Context,
	req agentcontext.FinalizeRequest,
	ticket FinalizationTicket,
	idempotencyKey string,
	result *agentcontext.FinalizeResult,
) {
	if c.writers.Memory == nil {
		result.Memory = agentcontext.WritebackStatusSkipped
		return
	}

	r, err := c.writers.Memory.EvaluateAndStore(ctx, MemoryWriteRequest{
		FinalizeKey:    req.FinalizeKey,
		Ticket:         ticket,
		IdempotencyKey: idempotencyKey,
		ProfileID:      req.Turn.Profile.Key.Name,
	})
	if err != nil {
		logger.Warn("[Coordinator] Memory 写入失败", zap.Error(err))
		result.Memory = agentcontext.WritebackStatusFailed
		return
	}
	result.Memory = agentcontext.WritebackStatus(r.Status)
}

// executeManifest 执行清单写入
func (c *TurnLifecycleCoordinator) executeManifest(
	ctx context.Context,
	req agentcontext.FinalizeRequest,
	ticket FinalizationTicket,
	idempotencyKey string,
	result *agentcontext.FinalizeResult,
) {
	if c.writers.Manifest == nil {
		result.Manifest = agentcontext.WritebackStatusSkipped
		return
	}

	err := c.writers.Manifest.StoreManifest(ctx, ManifestWriteRequest{
		FinalizeKey:    req.FinalizeKey,
		Ticket:         ticket,
		Manifest:       req.Turn.BaseManifest,
		TurnStatus:     string(req.Outcome.Status),
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		logger.Warn("[Coordinator] Manifest 写入失败", zap.Error(err))
		result.Manifest = agentcontext.WritebackStatusFailed
		return
	}
	result.Manifest = agentcontext.WritebackStatusSuccess
}
