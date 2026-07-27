package agentcontext

import "fmt"

// ExhaustedAction 耗尽动作
type ExhaustedAction string

const (
	ExhaustedAbort ExhaustedAction = "abort"
	ExhaustedSkip  ExhaustedAction = "skip"
)

// RetryPolicy 重试策略
type RetryPolicy struct {
	MaxAttempts int
	Backoff     BackoffPolicy
	Retryable   ErrorClassifier
}

// BackoffPolicy 退避策略
type BackoffPolicy struct {
	InitialMs  int
	MaxMs      int
	Multiplier float64
}

// ErrorClassifier 错误分类器
type ErrorClassifier func(err error) bool

// ResolutionStage 解析阶段
type ResolutionStage struct {
	Name        string
	Provider    string
	RetryPolicy RetryPolicy
}

// ResolutionPolicy 解析策略
type ResolutionPolicy struct {
	Stages         []ResolutionStage
	TerminalAction ExhaustedAction
}

// Validate 验证解析策略
func (p *ResolutionPolicy) Validate() error {
	if len(p.Stages) == 0 {
		return NewError(ErrCodeInvalidConfig, "resolution policy must have at least one stage")
	}

	for i, stage := range p.Stages {
		if stage.Name == "" {
			return NewError(ErrCodeInvalidConfig, fmt.Sprintf("stage %d name is required", i))
		}
		if stage.RetryPolicy.MaxAttempts < 0 {
			return NewError(ErrCodeInvalidConfig, fmt.Sprintf("stage %s max attempts cannot be negative", stage.Name))
		}
	}

	if p.TerminalAction != ExhaustedAbort && p.TerminalAction != ExhaustedSkip {
		return NewError(ErrCodeInvalidConfig, "terminal action must be abort or skip")
	}

	return nil
}

// DefaultRetryPolicy 返回默认重试策略（不重试）
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 1,
		Backoff: BackoffPolicy{
			InitialMs:  100,
			MaxMs:      1000,
			Multiplier: 2.0,
		},
		Retryable: func(err error) bool {
			return true
		},
	}
}

// DefaultResolutionPolicy 返回默认解析策略
func DefaultResolutionPolicy(terminal ExhaustedAction) ResolutionPolicy {
	return ResolutionPolicy{
		TerminalAction: terminal,
	}
}
