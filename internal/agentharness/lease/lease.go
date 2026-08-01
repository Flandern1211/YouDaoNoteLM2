// Package lease 实现 Lease、Fencing 与多实例安全。
package lease

import (
	"time"
)

// Lease Worker 租约。
type Lease struct {
	ID           string    `gorm:"primaryKey;type:varchar(36)"`
	WorkerID     string    `gorm:"type:varchar(128);not null;uniqueIndex"`
	FencingToken uint64    `gorm:"not null;unsigned"`
	ExpiresAt    time.Time `gorm:"not null;index"`
	CreatedAt    time.Time `gorm:"not null"`
	UpdatedAt    time.Time `gorm:"not null"`
}

func (Lease) TableName() string { return "agent_leases" }

// LeaseStore 租约存储接口。
type LeaseStore interface {
	// AcquireLease 获取租约（原子操作）。
	AcquireLease(workerID string, ttl time.Duration) (Lease, error)
	// RenewLease 续约。
	RenewLease(workerID string, ttl time.Duration) (Lease, error)
	// ReleaseLease 释放租约。
	ReleaseLease(workerID string) error
	// GetLease 获取租约。
	GetLease(workerID string) (Lease, error)
	// IsLeaseValid 检查租约是否有效。
	IsLeaseValid(workerID string) bool
}

// MemoryLeaseStore 内存实现的租约存储。
type MemoryLeaseStore struct {
	leases map[string]Lease
}

// NewMemoryLeaseStore 创建 MemoryLeaseStore。
func NewMemoryLeaseStore() *MemoryLeaseStore {
	return &MemoryLeaseStore{
		leases: make(map[string]Lease),
	}
}

// AcquireLease 获取租约。
func (s *MemoryLeaseStore) AcquireLease(workerID string, ttl time.Duration) (Lease, error) {
	now := time.Now()
	lease := Lease{
		ID:           workerID,
		WorkerID:     workerID,
		FencingToken: 1,
		ExpiresAt:    now.Add(ttl),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.leases[workerID] = lease
	return lease, nil
}

// RenewLease 续约。
func (s *MemoryLeaseStore) RenewLease(workerID string, ttl time.Duration) (Lease, error) {
	lease, exists := s.leases[workerID]
	if !exists {
		return Lease{}, ErrLeaseNotFound
	}

	now := time.Now()
	lease.FencingToken++
	lease.ExpiresAt = now.Add(ttl)
	lease.UpdatedAt = now
	s.leases[workerID] = lease
	return lease, nil
}

// ReleaseLease 释放租约。
func (s *MemoryLeaseStore) ReleaseLease(workerID string) error {
	delete(s.leases, workerID)
	return nil
}

// GetLease 获取租约。
func (s *MemoryLeaseStore) GetLease(workerID string) (Lease, error) {
	lease, exists := s.leases[workerID]
	if !exists {
		return Lease{}, ErrLeaseNotFound
	}
	return lease, nil
}

// IsLeaseValid 检查租约是否有效。
func (s *MemoryLeaseStore) IsLeaseValid(workerID string) bool {
	lease, exists := s.leases[workerID]
	if !exists {
		return false
	}
	return time.Now().Before(lease.ExpiresAt)
}

// Errors
var (
	ErrLeaseNotFound = &LeaseError{"lease not found"}
	ErrLeaseExpired  = &LeaseError{"lease expired"}
)

type LeaseError struct {
	msg string
}

func (e *LeaseError) Error() string {
	return e.msg
}

// FencingTokenManager Fencing Token 管理器。
type FencingTokenManager struct {
	store LeaseStore
}

// NewFencingTokenManager 创建 FencingTokenManager。
func NewFencingTokenManager(store LeaseStore) *FencingTokenManager {
	return &FencingTokenManager{store: store}
}

// ValidateFencingToken 验证 Fencing Token。
func (m *FencingTokenManager) ValidateFencingToken(workerID string, token uint64) bool {
	lease, err := m.store.GetLease(workerID)
	if err != nil {
		return false
	}
	// 只有当前 token 或更新的 token 有效
	return token >= lease.FencingToken
}

// GetCurrentToken 获取当前 Fencing Token。
func (m *FencingTokenManager) GetCurrentToken(workerID string) (uint64, error) {
	lease, err := m.store.GetLease(workerID)
	if err != nil {
		return 0, err
	}
	return lease.FencingToken, nil
}
