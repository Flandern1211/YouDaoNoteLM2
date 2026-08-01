package core

import "fmt"

// TransitionRequest 状态转换请求。
type TransitionRequest struct {
	RunID        RunID
	CurrentState RunState
	TargetState  RunState
	StateVersion StateVersion
	FencingToken FencingToken
	ErrorClass   *ErrorClass
	ErrorCode    *string
}

// TransitionResult 状态转换结果。
type TransitionResult struct {
	Success      bool
	NewState     RunState
	NewVersion   StateVersion
	ErrorMessage string
}

// validTransitions 定义合法的状态转换。
// 首期 RunStore 必须验证以下转换；尚未实现对应命令的转换只能由测试覆盖，不能从 HTTP 暴露。
var validTransitions = map[RunState]map[RunState]bool{
	RunStateQueued: {
		RunStateRunning:         true, // Worker claim
		RunStateCancelRequested: true, // Cancel command
	},
	RunStateRunning: {
		RunStateFinalizing:      true, // Supervisor
		RunStateRetryWait:       true, // Retry policy
		RunStatePauseRequested:  true, // Pause command
		RunStateCancelRequested: true, // Cancel command
		RunStateSuspended:       true, // Supervisor (unsafe recovery)
	},
	RunStateFinalizing: {
		RunStateSucceeded: true, // Finalizer
		RunStateFailed:    true, // Finalizer
		RunStateCancelled: true, // Finalizer
	},
	RunStateRetryWait: {
		RunStateQueued: true, // Dispatcher (next_retry_at expired)
	},
	RunStatePauseRequested: {
		RunStatePausing: true, // Worker observed command
	},
	RunStatePausing: {
		RunStatePaused:    true, // Checkpoint adapter verified
		RunStateSuspended: true, // Checkpoint unavailable
	},
	RunStatePaused: {
		RunStateQueued:         true, // Resume command
		RunStateCancelRequested: true, // Cancel command
	},
	RunStateCancelRequested: {
		RunStateCancelled: true, // Worker/Finalizer cleanup
	},
}

// ValidateTransition 验证状态转换是否合法。
func ValidateTransition(from, to RunState) error {
	targets, ok := validTransitions[from]
	if !ok {
		return fmt.Errorf("invalid source state: %s", from)
	}
	if !targets[to] {
		return fmt.Errorf("invalid transition: %s -> %s", from, to)
	}
	return nil
}

// ValidateErrorClassForRetry 验证错误分类是否允许重试。
// 只有 transient、rate_limited 和 timeout 错误可以重试。
func ValidateErrorClassForRetry(ec ErrorClass) bool {
	return ec == ErrorClassTransient ||
		ec == ErrorClassRateLimited ||
		ec == ErrorClassTimeout
}

// GetValidTransitions 返回当前状态的所有合法目标状态。
func GetValidTransitions(from RunState) []RunState {
	targets, ok := validTransitions[from]
	if !ok {
		return nil
	}
	result := make([]RunState, 0, len(targets))
	for to := range targets {
		result = append(result, to)
	}
	return result
}
