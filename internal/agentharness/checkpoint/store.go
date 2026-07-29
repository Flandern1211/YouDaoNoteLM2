package checkpoint

import (
	"context"
	"crypto/sha256"
	"fmt"
)

// CheckpointStore 检查点存储接口。
type CheckpointStore interface {
	// Set 写入检查点（不可变 version + CAS current pointer）。
	Set(ctx context.Context, cp Checkpoint) error
	// Get 获取当前检查点。
	Get(ctx context.Context, runID string) (Checkpoint, error)
	// GetByVersion 获取指定版本的检查点。
	GetByVersion(ctx context.Context, runID string, version uint64) (Checkpoint, error)
	// Delete 删除非当前检查点。
	Delete(ctx context.Context, runID string, version uint64) error
}

// MemoryCheckpointStore 内存实现的检查点存储（用于测试）。
type MemoryCheckpointStore struct {
	checkpoints map[string][]Checkpoint
}

// NewMemoryCheckpointStore 创建 MemoryCheckpointStore。
func NewMemoryCheckpointStore() *MemoryCheckpointStore {
	return &MemoryCheckpointStore{
		checkpoints: make(map[string][]Checkpoint),
	}
}

// Set 写入检查点。
func (s *MemoryCheckpointStore) Set(ctx context.Context, cp Checkpoint) error {
	// 设置 current pointer
	for i := range s.checkpoints[cp.RunID] {
		s.checkpoints[cp.RunID][i].IsCurrent = false
	}
	cp.IsCurrent = true
	s.checkpoints[cp.RunID] = append(s.checkpoints[cp.RunID], cp)
	return nil
}

// Get 获取当前检查点。
func (s *MemoryCheckpointStore) Get(ctx context.Context, runID string) (Checkpoint, error) {
	cps := s.checkpoints[runID]
	for _, cp := range cps {
		if cp.IsCurrent {
			return cp, nil
		}
	}
	return Checkpoint{}, fmt.Errorf("no current checkpoint for run %s", runID)
}

// GetByVersion 获取指定版本。
func (s *MemoryCheckpointStore) GetByVersion(ctx context.Context, runID string, version uint64) (Checkpoint, error) {
	for _, cp := range s.checkpoints[runID] {
		if cp.Version == version {
			return cp, nil
		}
	}
	return Checkpoint{}, fmt.Errorf("checkpoint version %d not found", version)
}

// Delete 删除检查点。
func (s *MemoryCheckpointStore) Delete(ctx context.Context, runID string, version uint64) error {
	cps := s.checkpoints[runID]
	for i, cp := range cps {
		if cp.Version == version && !cp.IsCurrent {
			s.checkpoints[runID] = append(cps[:i], cps[i+1:]...)
			return nil
		}
	}
	return nil
}

// ComputeChecksum 计算数据校验和。
func ComputeChecksum(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}
