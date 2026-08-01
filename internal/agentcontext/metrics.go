package agentcontext

import "time"

// ContextMetrics 上下文管理指标收集器接口。
// 沿用仓库现有指标设施，标签不得包含 userID、conversationID、原文或高基数指纹。
type ContextMetrics interface {
	// RecordPrepareDuration 记录 PrepareTurn 耗时
	RecordPrepareDuration(agentID AgentID, duration time.Duration)

	// RecordCompileDuration 记录 CompileModelInput 耗时
	RecordCompileDuration(agentID AgentID, duration time.Duration)

	// RecordProviderRetry 记录 Provider 重试
	RecordProviderRetry(provider string, stage string)

	// RecordProviderFallback 记录 Provider 回退
	RecordProviderFallback(provider string, stage string)

	// RecordProviderSkip 记录 Provider 跳过
	RecordProviderSkip(provider string, reason string)

	// RecordProviderAbort 记录 Provider 中止
	RecordProviderAbort(provider string, reason string)

	// RecordTokenCounterMode 记录 Token 计数模式
	RecordTokenCounterMode(mode TokenizerStrategy)

	// RecordTokenCounterError 记录 Token 计数错误
	RecordTokenCounterError(mode TokenizerStrategy, reason string)

	// RecordBudgetUtilization 记录预算利用率
	RecordBudgetUtilization(agentID AgentID, ratio float64)

	// RecordShadowMismatch 记录 Shadow 比较差异
	RecordShadowMismatch(diffType string)

	// RecordSnapshotRestore 记录快照恢复
	RecordSnapshotRestore(success bool)

	// RecordWritebackResult 记录写回结果
	RecordWritebackResult(operation string, status string)
}

// NoopMetrics 空操作指标收集器（用于测试和未配置时）
type NoopMetrics struct{}

func (n *NoopMetrics) RecordPrepareDuration(AgentID, time.Duration)      {}
func (n *NoopMetrics) RecordCompileDuration(AgentID, time.Duration)      {}
func (n *NoopMetrics) RecordProviderRetry(string, string)                {}
func (n *NoopMetrics) RecordProviderFallback(string, string)             {}
func (n *NoopMetrics) RecordProviderSkip(string, string)                 {}
func (n *NoopMetrics) RecordProviderAbort(string, string)                {}
func (n *NoopMetrics) RecordTokenCounterMode(TokenizerStrategy)          {}
func (n *NoopMetrics) RecordTokenCounterError(TokenizerStrategy, string) {}
func (n *NoopMetrics) RecordBudgetUtilization(AgentID, float64)          {}
func (n *NoopMetrics) RecordShadowMismatch(string)                       {}
func (n *NoopMetrics) RecordSnapshotRestore(bool)                        {}
func (n *NoopMetrics) RecordWritebackResult(string, string)              {}
