package lease

import (
	"testing"
	"time"
)

func TestMemoryLeaseStore_AcquireAndRenew(t *testing.T) {
	store := NewMemoryLeaseStore()

	// 获取租约
	lease, err := store.AcquireLease("worker-1", 30*time.Second)
	if err != nil {
		t.Fatalf("AcquireLease failed: %v", err)
	}
	if lease.FencingToken != 1 {
		t.Errorf("expected fencing token 1, got %d", lease.FencingToken)
	}

	// 续约
	renewed, err := store.RenewLease("worker-1", 30*time.Second)
	if err != nil {
		t.Fatalf("RenewLease failed: %v", err)
	}
	if renewed.FencingToken != 2 {
		t.Errorf("expected fencing token 2, got %d", renewed.FencingToken)
	}
}

func TestMemoryLeaseStore_Release(t *testing.T) {
	store := NewMemoryLeaseStore()

	store.AcquireLease("worker-1", 30*time.Second)
	store.ReleaseLease("worker-1")

	_, err := store.GetLease("worker-1")
	if err == nil {
		t.Error("expected error after release")
	}
}

func TestMemoryLeaseStore_IsLeaseValid(t *testing.T) {
	store := NewMemoryLeaseStore()

	// 不存在
	if store.IsLeaseValid("worker-1") {
		t.Error("expected false for non-existent lease")
	}

	// 有效
	store.AcquireLease("worker-1", 30*time.Second)
	if !store.IsLeaseValid("worker-1") {
		t.Error("expected true for valid lease")
	}
}

func TestFencingTokenManager_Validate(t *testing.T) {
	store := NewMemoryLeaseStore()
	manager := NewFencingTokenManager(store)

	store.AcquireLease("worker-1", 30*time.Second)

	// 当前 token 有效
	if !manager.ValidateFencingToken("worker-1", 1) {
		t.Error("expected true for current token")
	}

	// 更新的 token 有效
	if !manager.ValidateFencingToken("worker-1", 2) {
		t.Error("expected true for newer token")
	}

	// 旧 token 无效
	if manager.ValidateFencingToken("worker-1", 0) {
		t.Error("expected false for old token")
	}
}
