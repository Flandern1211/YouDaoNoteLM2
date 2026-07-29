package supervisor

import (
	"fmt"

	"YoudaoNoteLm/internal/agentharness/core"
)

// FakeFinalizationPort 用于测试的假 FinalizationPort 实现。
type FakeFinalizationPort struct {
	// Calls 记录调用次数。
	Calls int
	// LastRequest 记录最后一次请求。
	LastRequest *core.FinalizationRequest
	// Error 返回的错误（可选）。
	Error error
}

// Finalize 实现 FinalizationPort 接口。
func (f *FakeFinalizationPort) Finalize(req core.FinalizationRequest) (core.FinalizeResult, error) {
	f.Calls++
	f.LastRequest = &req

	if f.Error != nil {
		return core.FinalizeResult{}, f.Error
	}

	// 根据 Outcome 确定终态
	var newState core.RunState
	switch req.Outcome.Status {
	case core.OutcomeStatusSuccess:
		newState = core.RunStateSucceeded
	case core.OutcomeStatusFailed:
		newState = core.RunStateFailed
	case core.OutcomeStatusCancelled:
		newState = core.RunStateCancelled
	default:
		return core.FinalizeResult{}, fmt.Errorf("未知的 Outcome 状态: %s", req.Outcome.Status)
	}

	return core.FinalizeResult{
		NewState:    newState,
		NewVersion:  req.FinalizingStateVersion + 1,
		NewRevision: req.Revision + 1,
	}, nil
}
