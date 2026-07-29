package observability

import (
	"testing"
)

func TestBudgetEnforcer_IncrementModelCalls(t *testing.T) {
	budget := Budget{ModelCallsLimit: 3}
	enforcer := NewBudgetEnforcer(budget)

	// 前2次应该成功（第3次会触发超限，因为 >= limit）
	for i := 0; i < 2; i++ {
		if err := enforcer.IncrementModelCalls(); err != nil {
			t.Fatalf("call %d: unexpected error: %v", i+1, err)
		}
	}

	// 第3次应该超限
	if err := enforcer.IncrementModelCalls(); err == nil {
		t.Error("expected budget exceeded error")
	}
}

func TestBudgetEnforcer_AddTokens(t *testing.T) {
	budget := Budget{TokenLimit: 100}
	enforcer := NewBudgetEnforcer(budget)

	if err := enforcer.AddTokens(50); err != nil {
		t.Fatalf("AddTokens(50) failed: %v", err)
	}
	if err := enforcer.AddTokens(49); err != nil {
		t.Fatalf("AddTokens(49) failed: %v", err)
	}

	// 超限
	if err := enforcer.AddTokens(2); err == nil {
		t.Error("expected budget exceeded error")
	}
}

func TestBudgetEnforcer_NoLimit(t *testing.T) {
	budget := Budget{} // 无限制
	enforcer := NewBudgetEnforcer(budget)

	for i := 0; i < 1000; i++ {
		if err := enforcer.IncrementModelCalls(); err != nil {
			t.Fatalf("call %d: unexpected error: %v", i+1, err)
		}
	}
}

func TestFaultInjector_Disabled(t *testing.T) {
	injector := NewFaultInjector(FaultConfig{Enabled: false})

	if injector.ShouldInject() {
		t.Error("disabled injector should not inject")
	}
	if injector.GetFaultType() != FaultNone {
		t.Error("disabled injector should return FaultNone")
	}
}

func TestSLOTracker_Record(t *testing.T) {
	slos := []SLO{
		{Name: "availability", Target: 0.99},
	}
	tracker := NewSLOTracker(slos)

	// 记录100次，99次成功
	for i := 0; i < 99; i++ {
		tracker.Record("availability", true)
	}
	tracker.Record("availability", false)

	compliance := tracker.GetCompliance("availability")
	if compliance < 0.98 || compliance > 0.995 {
		t.Errorf("expected ~0.99 compliance, got %f", compliance)
	}

	if !tracker.IsCompliant("availability") {
		t.Error("expected SLO to be met")
	}
}
