// Package supervisor 实现 ExecutionSupervisor，管理 Run 的执行生命周期。
package supervisor

import (
	"context"
	"fmt"
	"time"

	"YoudaoNoteLm/internal/agentharness/core"
)

// EinoAgent Eino Agent 接口，由具体的 Chat/Main/Search Agent 实现。
type EinoAgent interface {
	// Execute 执行 Agent，返回结果事件流。
	Execute(ctx context.Context, input AgentInput) (<-chan AgentEvent, error)
}

// AgentInput Agent 输入。
type AgentInput struct {
	RunID     core.RunID
	AttemptID core.AttemptID
	Content   string
	Context   map[string]interface{}
}

// AgentEvent Agent 事件。
type AgentEvent struct {
	Type    string
	Content string
	Error   error
}

// StepGateway Step 网关，管理 Step 的创建和完成。
type StepGateway struct {
	store core.RunStore
}

// NewStepGateway 创建 StepGateway。
func NewStepGateway(store core.RunStore) *StepGateway {
	return &StepGateway{store: store}
}

// CreateStep 创建一个 running Step。
func (g *StepGateway) CreateStep(ctx context.Context, step core.Step) error {
	return g.store.CreateStep(ctx, step)
}

// FinishStep 完成一个 Step。
func (g *StepGateway) FinishStep(ctx context.Context, req core.FinishStepRequest) (core.Step, error) {
	return g.store.FinishStep(ctx, req)
}

// ExecutionSupervisor 执行 Supervisor，管理 Run 的执行生命周期。
type ExecutionSupervisor struct {
	store           core.RunStore
	finalizationPort core.FinalizationPort
	stepGateway     *StepGateway
	agent           EinoAgent
}

// NewExecutionSupervisor 创建 ExecutionSupervisor。
func NewExecutionSupervisor(
	store core.RunStore,
	finalizationPort core.FinalizationPort,
	agent EinoAgent,
) *ExecutionSupervisor {
	return &ExecutionSupervisor{
		store:           store,
		finalizationPort: finalizationPort,
		stepGateway:     NewStepGateway(store),
		agent:           agent,
	}
}

// Execute 执行一个 Run。
// 前置条件：Run 已被 Claim，处于 running 状态。
func (s *ExecutionSupervisor) Execute(ctx context.Context, runID core.RunID, workerID string) error {
	// 1. 获取 Run
	run, err := s.store.Get(ctx, runID)
	if err != nil {
		return fmt.Errorf("获取 Run 失败: %w", err)
	}

	// 验证 Run 状态
	if run.State != core.RunStateRunning {
		return fmt.Errorf("Run 状态不是 running: %s", run.State)
	}

	if run.Authority == nil {
		return fmt.Errorf("Run 没有 Authority")
	}

	// 2. 创建独立的 runCtx（不依赖 HTTP context）
	runCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// 3. 执行 Agent
	outcome := s.executeAgent(runCtx, run)

	// 4. 进入 Finalization
	return s.finalize(ctx, run, outcome)
}

// executeAgent 执行 Agent 并收集结果。
func (s *ExecutionSupervisor) executeAgent(ctx context.Context, run core.Run) core.Outcome {
	// 准备 Agent 输入
	input := AgentInput{
		RunID:   run.ID,
		Content: run.Input.Ref,
		Context: make(map[string]interface{}),
	}

	if run.Authority != nil {
		input.AttemptID = run.Authority.AttemptID
	}

	// 执行 Agent
	eventCh, err := s.agent.Execute(ctx, input)
	if err != nil {
		return core.Outcome{
			Status:     core.OutcomeStatusFailed,
			ErrorClass: errorClassPtr(core.ErrorClassPermanent),
			ErrorCode:  stringPtr("agent_execute_failed"),
		}
	}

	// 消费事件流
	for event := range eventCh {
		if event.Error != nil {
			return mapError(event.Error)
		}
		// 处理成功事件（实际实现在后续迭代中完善）
	}

	return core.Outcome{
		Status: core.OutcomeStatusSuccess,
	}
}

// finalize 进入终态化。
func (s *ExecutionSupervisor) finalize(ctx context.Context, run core.Run, outcome core.Outcome) error {
	if run.Authority == nil {
		return fmt.Errorf("Run 没有 Authority")
	}

	// CAS: running -> finalizing
	transitionReq := core.TransitionRequest{
		RunID:        run.ID,
		CurrentState: core.RunStateRunning,
		TargetState:  core.RunStateFinalizing,
		StateVersion: run.StateVersion,
		FencingToken: run.Authority.FencingToken,
	}

	run, err := s.store.Transition(ctx, transitionReq)
	if err != nil {
		return fmt.Errorf("转换到 finalizing 失败: %w", err)
	}

	// 调用 FinalizationPort
	finalizeReq := core.FinalizationRequest{
		RunID:                  run.ID,
		Authority:              *run.Authority,
		FinalizingStateVersion: run.StateVersion,
		Revision:               run.Revision,
		Outcome:                outcome,
	}

	_, err = s.finalizationPort.Finalize(finalizeReq)
	if err != nil {
		return fmt.Errorf("终态化失败: %w", err)
	}

	return nil
}

// mapError 将错误映射到 ErrorClass。
func mapError(err error) core.Outcome {
	// 默认为 permanent 错误
	return core.Outcome{
		Status:      core.OutcomeStatusFailed,
		ErrorClass:  errorClassPtr(core.ErrorClassPermanent),
		ErrorCode:   stringPtr("unknown_error"),
		ErrorDetail: stringPtr(err.Error()),
	}
}

func errorClassPtr(ec core.ErrorClass) *core.ErrorClass {
	return &ec
}

func stringPtr(s string) *string {
	return &s
}
