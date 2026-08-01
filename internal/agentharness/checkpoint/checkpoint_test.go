package checkpoint

import (
	"context"
	"testing"
)

func TestMemoryCheckpointStore_SetAndGet(t *testing.T) {
	store := NewMemoryCheckpointStore()
	ctx := context.Background()

	cp := Checkpoint{
		ID:          "cp-1",
		RunID:       "run-1",
		AttemptID:   "attempt-1",
		Version:     1,
		StorageKind: StorageKindMySQL,
		Bytes:       []byte("checkpoint data"),
		ByteSize:    15,
		SHA256:      ComputeChecksum([]byte("checkpoint data")),
	}

	if err := store.Set(ctx, cp); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	got, err := store.Get(ctx, "run-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.ID != "cp-1" {
		t.Errorf("expected ID 'cp-1', got '%s'", got.ID)
	}
	if !got.IsCurrent {
		t.Error("expected IsCurrent=true")
	}
}

func TestMemoryCheckpointStore_MultipleVersions(t *testing.T) {
	store := NewMemoryCheckpointStore()
	ctx := context.Background()

	cp1 := Checkpoint{ID: "cp-1", RunID: "run-1", Version: 1, Bytes: []byte("v1")}
	cp2 := Checkpoint{ID: "cp-2", RunID: "run-1", Version: 2, Bytes: []byte("v2")}

	store.Set(ctx, cp1)
	store.Set(ctx, cp2)

	// Get 应该返回最新的 current
	got, _ := store.Get(ctx, "run-1")
	if got.Version != 2 {
		t.Errorf("expected version 2, got %d", got.Version)
	}

	// GetByVersion 可以获取旧版本
	old, _ := store.GetByVersion(ctx, "run-1", 1)
	if old.Version != 1 {
		t.Errorf("expected version 1, got %d", old.Version)
	}
}

func TestComputeChecksum(t *testing.T) {
	data := []byte("test data")
	checksum := ComputeChecksum(data)

	if len(checksum) != 64 {
		t.Errorf("expected 64 char checksum, got %d", len(checksum))
	}

	// 相同数据应产生相同校验和
	checksum2 := ComputeChecksum(data)
	if checksum != checksum2 {
		t.Error("same data should produce same checksum")
	}

	// 不同数据应产生不同校验和
	checksum3 := ComputeChecksum([]byte("other data"))
	if checksum == checksum3 {
		t.Error("different data should produce different checksum")
	}
}
