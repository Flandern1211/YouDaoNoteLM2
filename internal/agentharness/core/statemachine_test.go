package core

import (
	"testing"
)

func TestValidateTransition(t *testing.T) {
	tests := []struct {
		name    string
		from    RunState
		to      RunState
		wantErr bool
	}{
		// 合法转换
		{"queued -> running", RunStateQueued, RunStateRunning, false},
		{"queued -> cancel_requested", RunStateQueued, RunStateCancelRequested, false},
		{"running -> finalizing", RunStateRunning, RunStateFinalizing, false},
		{"running -> retry_wait", RunStateRunning, RunStateRetryWait, false},
		{"running -> pause_requested", RunStateRunning, RunStatePauseRequested, false},
		{"running -> cancel_requested", RunStateRunning, RunStateCancelRequested, false},
		{"running -> suspended", RunStateRunning, RunStateSuspended, false},
		{"finalizing -> succeeded", RunStateFinalizing, RunStateSucceeded, false},
		{"finalizing -> failed", RunStateFinalizing, RunStateFailed, false},
		{"finalizing -> cancelled", RunStateFinalizing, RunStateCancelled, false},
		{"retry_wait -> queued", RunStateRetryWait, RunStateQueued, false},
		{"pause_requested -> pausing", RunStatePauseRequested, RunStatePausing, false},
		{"pausing -> paused", RunStatePausing, RunStatePaused, false},
		{"pausing -> suspended", RunStatePausing, RunStateSuspended, false},
		{"paused -> queued", RunStatePaused, RunStateQueued, false},
		{"paused -> cancel_requested", RunStatePaused, RunStateCancelRequested, false},
		{"cancel_requested -> cancelled", RunStateCancelRequested, RunStateCancelled, false},

		// 非法转换
		{"queued -> finalizing", RunStateQueued, RunStateFinalizing, true},
		{"queued -> succeeded", RunStateQueued, RunStateSucceeded, true},
		{"running -> queued", RunStateRunning, RunStateQueued, true},
		{"running -> succeeded", RunStateRunning, RunStateSucceeded, true},
		{"finalizing -> running", RunStateFinalizing, RunStateRunning, true},
		{"succeeded -> running", RunStateSucceeded, RunStateRunning, true},
		{"failed -> running", RunStateFailed, RunStateRunning, true},
		{"cancelled -> running", RunStateCancelled, RunStateRunning, true},
		{"suspended -> running", RunStateSuspended, RunStateRunning, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTransition(tt.from, tt.to)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTransition(%s, %s) error = %v, wantErr %v", tt.from, tt.to, err, tt.wantErr)
			}
		})
	}
}

func TestValidateErrorClassForRetry(t *testing.T) {
	tests := []struct {
		ec       ErrorClass
		expected bool
	}{
		{ErrorClassTransient, true},
		{ErrorClassRateLimited, true},
		{ErrorClassTimeout, true},
		{ErrorClassPermanent, false},
		{ErrorClassCancelled, false},
		{ErrorClassResourceExhausted, false},
		{ErrorClassInvalidInput, false},
		{ErrorClassPermission, false},
		{ErrorClassDependencyPermanent, false},
		{ErrorClassWorkerLost, false},
		{ErrorClassCheckpointIncompatible, false},
		{ErrorClassSideEffectUnknown, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.ec), func(t *testing.T) {
			result := ValidateErrorClassForRetry(tt.ec)
			if result != tt.expected {
				t.Errorf("ValidateErrorClassForRetry(%s) = %v, want %v", tt.ec, result, tt.expected)
			}
		})
	}
}

func TestGetValidTransitions(t *testing.T) {
	tests := []struct {
		state    RunState
		expected int
	}{
		{RunStateQueued, 2},
		{RunStateRunning, 5},
		{RunStateFinalizing, 3},
		{RunStateRetryWait, 1},
		{RunStatePauseRequested, 1},
		{RunStatePausing, 2},
		{RunStatePaused, 2},
		{RunStateCancelRequested, 1},
		{RunStateSucceeded, 0},
		{RunStateFailed, 0},
		{RunStateCancelled, 0},
		{RunStateSuspended, 0},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			transitions := GetValidTransitions(tt.state)
			if len(transitions) != tt.expected {
				t.Errorf("GetValidTransitions(%s) returned %d transitions, want %d", tt.state, len(transitions), tt.expected)
			}
		})
	}
}
