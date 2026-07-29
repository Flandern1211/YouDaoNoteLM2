// Package core 定义 Agent Harness 的领域契约。
// 此文件定义 H3 的 FinalizationPort 接口和 Outcome 类型。
package core

// Outcome 执行结果的冻结表示。
type Outcome struct {
	Status      OutcomeStatus
	ErrorClass  *ErrorClass
	ErrorCode   *string
	ErrorDetail *string
}

// OutcomeStatus 结果状态。
type OutcomeStatus string

const (
	OutcomeStatusSuccess  OutcomeStatus = "success"
	OutcomeStatusFailed   OutcomeStatus = "failed"
	OutcomeStatusCancelled OutcomeStatus = "cancelled"
)

// FinalizationRequest 终态化请求。
type FinalizationRequest struct {
	RunID                  RunID
	Authority              ExecutionAuthority
	FinalizingStateVersion StateVersion
	Revision               Revision
	Outcome                Outcome
}

// FinalizeResult 终态化结果。
type FinalizeResult struct {
	NewState     RunState
	NewVersion   StateVersion
	NewRevision  Revision
}

// FinalizationPort 终态化端口，由 H4 实现。
// H3 使用 fake 实现验证执行闭环。
type FinalizationPort interface {
	Finalize(ctx FinalizationRequest) (FinalizeResult, error)
}
