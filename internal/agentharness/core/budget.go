// Package core 定义 Agent Harness 的领域契约。
// 此文件定义 Run Budget 类型。
package core

// RunBudget Run 预算快照。
type RunBudget struct {
	// WallTimeLimitNs 墙上时间限制（纳秒）。
	WallTimeLimitNs int64
	// IterationsLimit 迭代次数限制。
	IterationsLimit int
	// ModelCallsLimit 模型调用次数限制。
	ModelCallsLimit int
	// ToolCallsLimit 工具调用次数限制。
	ToolCallsLimit int
	// SearchCallsLimit 搜索调用次数限制。
	SearchCallsLimit int
	// TokenLimit Token 总数限制。
	TokenLimit int
	// CostLimitCents 费用限制（分）。
	CostLimitCents int
	// RetryUnitsLimit 重试单元限制。
	RetryUnitsLimit int
}

// BudgetUsage 预算使用量。
type BudgetUsage struct {
	WallTimeNs    int64
	Iterations    int
	ModelCalls    int
	ToolCalls     int
	SearchCalls   int
	Tokens        int
	CostCents     int
	RetryUnits    int
}

// BudgetExceededError 预算超限错误。
type BudgetExceededError struct {
	Resource string
	Limit    int
	Current  int
}

func (e *BudgetExceededError) Error() string {
	return "budget exceeded: " + e.Resource
}
