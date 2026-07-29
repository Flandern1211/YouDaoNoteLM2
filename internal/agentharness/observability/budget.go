package observability

import (
	"fmt"
)

// Budget 资源预算。
type Budget struct {
	WallTimeLimitNs  int64
	IterationsLimit  int
	ModelCallsLimit  int
	ToolCallsLimit   int
	SearchCallsLimit int
	TokenLimit       int
	CostLimitCents   int
	RetryUnitsLimit  int
}

// BudgetUsage 预算使用量。
type BudgetUsage struct {
	WallTimeNs  int64
	Iterations  int
	ModelCalls  int
	ToolCalls   int
	SearchCalls int
	Tokens      int
	CostCents   int
	RetryUnits  int
}

// BudgetEnforcer 预算执行器。
type BudgetEnforcer struct {
	budget Budget
	usage  BudgetUsage
}

// NewBudgetEnforcer 创建 BudgetEnforcer。
func NewBudgetEnforcer(budget Budget) *BudgetEnforcer {
	return &BudgetEnforcer{budget: budget}
}

// CheckLimit 检查是否超限。
func (e *BudgetEnforcer) CheckLimit(resource string, current, limit int) error {
	if limit > 0 && current >= limit {
		return &BudgetExceededError{Resource: resource, Limit: limit, Current: current}
	}
	return nil
}

// IncrementModelCalls 增加模型调用计数。
func (e *BudgetEnforcer) IncrementModelCalls() error {
	e.usage.ModelCalls++
	return e.CheckLimit("model_calls", e.usage.ModelCalls, e.budget.ModelCallsLimit)
}

// IncrementToolCalls 增加工具调用计数。
func (e *BudgetEnforcer) IncrementToolCalls() error {
	e.usage.ToolCalls++
	return e.CheckLimit("tool_calls", e.usage.ToolCalls, e.budget.ToolCallsLimit)
}

// IncrementSearchCalls 增加搜索调用计数。
func (e *BudgetEnforcer) IncrementSearchCalls() error {
	e.usage.SearchCalls++
	return e.CheckLimit("search_calls", e.usage.SearchCalls, e.budget.SearchCallsLimit)
}

// AddTokens 增加 Token 使用量。
func (e *BudgetEnforcer) AddTokens(tokens int) error {
	e.usage.Tokens += tokens
	return e.CheckLimit("tokens", e.usage.Tokens, e.budget.TokenLimit)
}

// AddCost 增加费用。
func (e *BudgetEnforcer) AddCost(cents int) error {
	e.usage.CostCents += cents
	return e.CheckLimit("cost_cents", e.usage.CostCents, e.budget.CostLimitCents)
}

// IncrementRetryUnits 增加重试单元。
func (e *BudgetEnforcer) IncrementRetryUnits() error {
	e.usage.RetryUnits++
	return e.CheckLimit("retry_units", e.usage.RetryUnits, e.budget.RetryUnitsLimit)
}

// GetUsage 获取当前使用量。
func (e *BudgetEnforcer) GetUsage() BudgetUsage {
	return e.usage
}

// BudgetExceededError 预算超限错误。
type BudgetExceededError struct {
	Resource string
	Limit    int
	Current  int
}

func (e *BudgetExceededError) Error() string {
	return fmt.Sprintf("budget exceeded: %s (limit=%d, current=%d)", e.Resource, e.Limit, e.Current)
}
