package core

import (
	"testing"
)

func TestIsTerminalRunState(t *testing.T) {
	tests := []struct {
		state    RunState
		expected bool
	}{
		{RunStateQueued, false},
		{RunStateRunning, false},
		{RunStateFinalizing, false},
		{RunStateRetryWait, false},
		{RunStatePauseRequested, false},
		{RunStatePausing, false},
		{RunStatePaused, false},
		{RunStateCancelRequested, false},
		{RunStateSucceeded, true},
		{RunStateFailed, true},
		{RunStateCancelled, true},
		{RunStateSuspended, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			result := IsTerminalRunState(tt.state)
			if result != tt.expected {
				t.Errorf("IsTerminalRunState(%s) = %v, want %v", tt.state, result, tt.expected)
			}
		})
	}
}

func TestIsTerminalAttemptState(t *testing.T) {
	tests := []struct {
		state    AttemptState
		expected bool
	}{
		{AttemptStateRunning, false},
		{AttemptStateCompleted, true},
		{AttemptStateFailed, true},
		{AttemptStateCancelled, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			result := IsTerminalAttemptState(tt.state)
			if result != tt.expected {
				t.Errorf("IsTerminalAttemptState(%s) = %v, want %v", tt.state, result, tt.expected)
			}
		})
	}
}

func TestIsTerminalStepState(t *testing.T) {
	tests := []struct {
		state    StepState
		expected bool
	}{
		{StepStateRunning, false},
		{StepStateCompleted, true},
		{StepStateFailed, true},
		{StepStateCancelled, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			result := IsTerminalStepState(tt.state)
			if result != tt.expected {
				t.Errorf("IsTerminalStepState(%s) = %v, want %v", tt.state, result, tt.expected)
			}
		})
	}
}

func TestIsKnownErrorClass(t *testing.T) {
	tests := []struct {
		ec       ErrorClass
		expected bool
	}{
		{ErrorClassPermanent, true},
		{ErrorClassTransient, true},
		{ErrorClassRateLimited, true},
		{ErrorClassTimeout, true},
		{ErrorClassCancelled, true},
		{ErrorClassResourceExhausted, true},
		{ErrorClassInvalidInput, true},
		{ErrorClassPermission, true},
		{ErrorClassDependencyPermanent, true},
		{ErrorClassWorkerLost, true},
		{ErrorClassCheckpointIncompatible, true},
		{ErrorClassSideEffectUnknown, true},
		{"unknown", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.ec), func(t *testing.T) {
			result := IsKnownErrorClass(tt.ec)
			if result != tt.expected {
				t.Errorf("IsKnownErrorClass(%s) = %v, want %v", tt.ec, result, tt.expected)
			}
		})
	}
}
