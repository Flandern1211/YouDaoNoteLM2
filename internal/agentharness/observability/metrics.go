// Package observability 实现资源预算、可观测性与硬化。
package observability

import (
	"time"
)

// RunMetrics Run 指标。
type RunMetrics struct {
	RunID         string        `json:"run_id"`
	Duration      time.Duration `json:"duration"`
	StepCount     int           `json:"step_count"`
	ModelCalls    int           `json:"model_calls"`
	ToolCalls     int           `json:"tool_calls"`
	SearchCalls   int           `json:"search_calls"`
	TokensUsed    int           `json:"tokens_used"`
	CostCents     int           `json:"cost_cents"`
	RetryUnits    int           `json:"retry_units"`
	ErrorClass    string        `json:"error_class,omitempty"`
	ErrorCode     string        `json:"error_code,omitempty"`
}

// MetricsCollector 指标收集器接口。
type MetricsCollector interface {
	// RecordRun 记录 Run 指标。
	RecordRun(metrics RunMetrics)
	// RecordStep 记录 Step 指标。
	RecordStep(runID string, stepType string, duration time.Duration, success bool)
	// RecordError 记录错误。
	RecordError(errorClass string, errorCode string)
}

// NoopMetricsCollector 空实现的指标收集器。
type NoopMetricsCollector struct{}

func (c *NoopMetricsCollector) RecordRun(metrics RunMetrics)           {}
func (c *NoopMetricsCollector) RecordStep(runID, stepType string, duration time.Duration, success bool) {}
func (c *NoopMetricsCollector) RecordError(errorClass, errorCode string) {}
