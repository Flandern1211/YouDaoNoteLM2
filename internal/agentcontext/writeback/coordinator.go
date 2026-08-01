package writeback

import (
	"context"
	"fmt"

	"YoudaoNoteLm/internal/agentcontext"
	"YoudaoNoteLm/pkg/logger"

	"go.uber.org/zap"
)

// TurnVerifier Turn 验证器端口。
// 由最小 Harness 或未来完整 RunService 适配器实现。
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
	if c == nil || c.verifier == nil {
		return nil, fmt.Errorf("TurnVerifier 未配置")
	}
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
	if c == nil || c.verifier == nil {
		return nil, fmt.Errorf("TurnVerifier 未配置")
	}
	if req.Turn == nil || req.Turn.Session == nil {
		return nil, fmt.Errorf("PreparedTurn 或 TurnSession 不能为空")
	}

	// 1. 验证 Finalize 的 Authority
	if err := c.verifier.VerifyAuthority(ctx, req.Turn.Session.Handle.RunID, req.Authority); err != nil {
		return nil, fmt.Errorf("Finalize Authority 验证失败: %w", err)
	}

	// 2. 构建写回依赖图
	policy := req.Turn.Profile.WritebackPolicy
	graph := NewWritebackGraph(policy, req.Outcome.Status)
	writebackOwner := req.Turn.Session.Handle.ContextMode.WritebackOwner
	if writebackOwner != "" && writebackOwner != "context" {
		graph = NewManifestOnlyGraph()
	}

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

	var requiredErr error
	failed := make(map[WritebackOperation]bool)
	for _, stage := range plan.Stages {
		for _, op := range stage {
			node := plan.Nodes[op]
			if hasFailedDependency(node, failed) {
				failed[op] = true
				continue
			}
			if err := c.executeWriteback(ctx, op, req, ticket, result); err != nil {
				failed[op] = true
				if node.Required && requiredErr == nil {
					requiredErr = err
				}
			}
		}
	}

	return result, requiredErr
}

// executeWriteback 执行单个写回操作
func (c *TurnLifecycleCoordinator) executeWriteback(
	ctx context.Context,
	op WritebackOperation,
	req agentcontext.FinalizeRequest,
	ticket FinalizationTicket,
	result *agentcontext.FinalizeResult,
) error {
	idempotencyKey := fmt.Sprintf("%s-%d-%s", req.FinalizeKey.RunID, req.FinalizeKey.Revision, op)

	switch op {
	case WritebackOperationAssistant:
		return c.executeAssistant(ctx, req, ticket, idempotencyKey, result)
	case WritebackOperationStepResult:
		return c.executeStepResult(ctx, req, ticket, idempotencyKey, result)
	case WritebackOperationSummary:
		return c.executeSummary(ctx, req, ticket, idempotencyKey, result)
	case WritebackOperationMemory:
		return c.executeMemory(ctx, req, ticket, idempotencyKey, result)
	case WritebackOperationManifest:
		return c.executeManifest(ctx, req, ticket, idempotencyKey, result)
	}
	return nil
}

// executeAssistant 执行助手消息写入
func (c *TurnLifecycleCoordinator) executeAssistant(
	ctx context.Context,
	req agentcontext.FinalizeRequest,
	ticket FinalizationTicket,
	idempotencyKey string,
	result *agentcontext.FinalizeResult,
) error {
	if c.writers.Assistant == nil {
		return fmt.Errorf("AssistantMessageWriter 未配置")
	}

	output, ok := req.Outcome.PrimaryOutput.(agentcontext.ConversationOutput)
	if !ok || output.FinalMessage == nil {
		return fmt.Errorf("成功 Turn 缺少 ConversationOutput")
	}

	committed, err := c.writers.Assistant.CommitAssistant(ctx, AssistantWriteRequest{
		FinalizeKey:    req.FinalizeKey,
		Ticket:         ticket,
		Authority:      req.Authority,
		RunID:          req.Turn.Session.Handle.RunID,
		ConversationID: req.Turn.Session.Handle.ConversationID,
		UserID:         req.Turn.Session.Handle.UserID,
		UserContent:    userInputContent(req.Turn.Session.Handle.Input),
		Content:        output.FinalMessage.Content,
		References:     output.Metadata,
		IdempotencyKey: idempotencyKey,
		ProfileID:      req.Turn.Profile.Key.Name,
		Mode:           req.Turn.Session.Handle.ContextMode.Mode,
	})
	if err != nil {
		logger.Error("[Coordinator] Assistant 写入失败", zap.Error(err))
		return fmt.Errorf("Assistant 写入失败: %w", err)
	}
	result.Primary = committed
	return nil
}

// executeStepResult 执行步骤结果写入
func (c *TurnLifecycleCoordinator) executeStepResult(
	ctx context.Context,
	req agentcontext.FinalizeRequest,
	ticket FinalizationTicket,
	idempotencyKey string,
	result *agentcontext.FinalizeResult,
) error {
	if c.writers.StepResult == nil {
		return fmt.Errorf("StepResultWriter 未配置")
	}

	output, ok := req.Outcome.PrimaryOutput.(agentcontext.StepOutput)
	if !ok {
		return fmt.Errorf("成功 Search Turn 缺少 StepOutput")
	}

	committed, err := c.writers.StepResult.CommitStepResult(ctx, StepResultWriteRequest{
		FinalizeKey:    req.FinalizeKey,
		Ticket:         ticket,
		Authority:      req.Authority,
		RunID:          req.Turn.Session.Handle.RunID,
		StepID:         req.Turn.Session.Handle.StepID,
		UserID:         req.Turn.Session.Handle.UserID,
		Result:         output.Result,
		IdempotencyKey: idempotencyKey,
		ProfileID:      req.Turn.Profile.Key.Name,
	})
	if err != nil {
		logger.Error("[Coordinator] StepResult 写入失败", zap.Error(err))
		return fmt.Errorf("StepResult 写入失败: %w", err)
	}
	result.Primary = committed
	return nil
}

// executeSummary 执行摘要写入
func (c *TurnLifecycleCoordinator) executeSummary(
	ctx context.Context,
	req agentcontext.FinalizeRequest,
	ticket FinalizationTicket,
	idempotencyKey string,
	result *agentcontext.FinalizeResult,
) error {
	if c.writers.Summary == nil {
		result.Summary = agentcontext.WritebackStatusSkipped
		return nil
	}

	r, err := c.writers.Summary.EvaluateAndUpdate(ctx, SummaryWriteRequest{
		FinalizeKey:    req.FinalizeKey,
		Ticket:         ticket,
		ConversationID: req.Turn.Session.Handle.ConversationID,
		NewContent:     conversationOutputContent(req.Outcome.PrimaryOutput),
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		logger.Warn("[Coordinator] Summary 写入失败", zap.Error(err))
		result.Summary = agentcontext.WritebackStatusFailed
		return err
	}
	result.Summary = agentcontext.WritebackStatus(r.Status)
	return nil
}

// executeMemory 执行记忆写入
func (c *TurnLifecycleCoordinator) executeMemory(
	ctx context.Context,
	req agentcontext.FinalizeRequest,
	ticket FinalizationTicket,
	idempotencyKey string,
	result *agentcontext.FinalizeResult,
) error {
	if c.writers.Memory == nil {
		result.Memory = agentcontext.WritebackStatusSkipped
		return nil
	}

	r, err := c.writers.Memory.EvaluateAndStore(ctx, MemoryWriteRequest{
		FinalizeKey:    req.FinalizeKey,
		Ticket:         ticket,
		UserID:         req.Turn.Session.Handle.UserID,
		SourceContent:  userInputContent(req.Turn.Session.Handle.Input),
		OutputContent:  conversationOutputContent(req.Outcome.PrimaryOutput),
		IdempotencyKey: idempotencyKey,
		ProfileID:      req.Turn.Profile.Key.Name,
	})
	if err != nil {
		logger.Warn("[Coordinator] Memory 写入失败", zap.Error(err))
		result.Memory = agentcontext.WritebackStatusFailed
		return err
	}
	result.Memory = agentcontext.WritebackStatus(r.Status)
	return nil
}

// executeManifest 执行清单写入
func (c *TurnLifecycleCoordinator) executeManifest(
	ctx context.Context,
	req agentcontext.FinalizeRequest,
	ticket FinalizationTicket,
	idempotencyKey string,
	result *agentcontext.FinalizeResult,
) error {
	if c.writers.Manifest == nil {
		result.Manifest = agentcontext.WritebackStatusSkipped
		return nil
	}

	records := req.CompileRecords
	if len(records) == 0 {
		records = []agentcontext.CompileRecord{{
			ModelCallID: "base",
			Manifest:    req.Turn.BaseManifest,
		}}
	}
	for index, record := range records {
		manifest := record.Manifest
		manifest.TurnStatus = string(req.Outcome.Status)
		recordKey := fmt.Sprintf("%s-%d", idempotencyKey, index)
		err := c.writers.Manifest.StoreManifest(ctx, ManifestWriteRequest{
			FinalizeKey:    req.FinalizeKey,
			Ticket:         ticket,
			Manifest:       manifest,
			ModelCallID:    record.ModelCallID,
			TurnStatus:     string(req.Outcome.Status),
			IdempotencyKey: recordKey,
		})
		if err != nil {
			logger.Warn("[Coordinator] Manifest 写入失败", zap.Error(err))
			result.Manifest = agentcontext.WritebackStatusFailed
			return err
		}
	}
	result.Manifest = agentcontext.WritebackStatusSuccess
	return nil
}

func hasFailedDependency(node WritebackNode, failed map[WritebackOperation]bool) bool {
	for _, dependency := range node.DependsOn {
		if failed[dependency] {
			return true
		}
	}
	return false
}

func userInputContent(input agentcontext.TurnInput) string {
	switch value := input.(type) {
	case agentcontext.UserMessageInput:
		return value.Content
	case agentcontext.SearchTaskInput:
		return value.Task.Query
	default:
		return ""
	}
}

func conversationOutputContent(output agentcontext.PrimaryOutput) string {
	conversation, ok := output.(agentcontext.ConversationOutput)
	if !ok || conversation.FinalMessage == nil {
		return ""
	}
	return conversation.FinalMessage.Content
}
