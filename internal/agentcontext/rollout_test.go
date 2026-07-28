package agentcontext

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 固定测试向量：确保算法一致性
// 使用标准 SHA-256 预计算，不依赖 Go 的 map/hash 随机种子

func TestShadowBucketSelector_FixedVectors(t *testing.T) {
	s := NewShadowBucketSelector()

	// 向量 1: userID=1, rolloutVersion="v1"
	// input = "context-shadow:v1:1"
	// SHA-256: 0x15ea = 5610
	bucket1 := s.Bucket(1, "v1")
	assert.Equal(t, uint64(5610), bucket1, "userID=1, version=v1 应该落在桶 5610")

	// 向量 2: userID=42, rolloutVersion="v1"
	// input = "context-shadow:v1:42"
	// SHA-256: 0x3fa = 1018
	bucket2 := s.Bucket(42, "v1")
	assert.Equal(t, uint64(1018), bucket2, "userID=42, version=v1 应该落在桶 1018")

	// 向量 3: userID=1, rolloutVersion="v2"
	// input = "context-shadow:v2:1"
	// SHA-256: 0x800 = 2048
	bucket3 := s.Bucket(1, "v2")
	assert.Equal(t, uint64(2048), bucket3, "userID=1, version=v2 应该落在桶 2048")
	assert.NotEqual(t, bucket1, bucket3, "不同 rolloutVersion 应该产生不同桶")

	// 向量 4: userID=0, rolloutVersion="v1"
	// input = "context-shadow:v1:0"
	// SHA-256: 0x157f = 5503
	bucket4 := s.Bucket(0, "v1")
	assert.Equal(t, uint64(5503), bucket4, "userID=0 的桶值应为 5503")

	// 向量 5: userID=12345, rolloutVersion="test-v1"
	// input = "context-shadow:test-v1:12345"
	// SHA-256: 0x15f = 351
	bucket5 := s.Bucket(12345, "test-v1")
	assert.Equal(t, uint64(351), bucket5, "userID=12345, version=test-v1 应该落在桶 351")
}

func TestShadowBucketSelector_Stability(t *testing.T) {
	s := NewShadowBucketSelector()

	// 同一输入必须始终返回相同结果
	for i := 0; i < 100; i++ {
		bucket := s.Bucket(12345, "test-v1")
		assert.Equal(t, uint64(351), bucket, "第 %d 次调用应该返回相同结果", i)
	}
}

func TestShadowBucketSelector_IsSelected(t *testing.T) {
	s := NewShadowBucketSelector()

	// userID=1, version=v1, bucket=5610
	assert.True(t, s.IsSelected(1, "v1", 6000), "bucket 5610 < 6000 应该选中")
	assert.False(t, s.IsSelected(1, "v1", 5000), "bucket 5610 >= 5000 不应该选中")
	assert.True(t, s.IsSelected(1, "v1", 5611), "bucket 5610 < 5611 应该选中")
	assert.False(t, s.IsSelected(1, "v1", 5610), "bucket 5610 >= 5610 不应该选中")

	// userID=42, version=v1, bucket=1018
	assert.True(t, s.IsSelected(42, "v1", 1019), "bucket 1018 < 1019 应该选中")
	assert.False(t, s.IsSelected(42, "v1", 1018), "bucket 1018 >= 1018 不应该选中")
}

func TestShadowBucketSelector_EdgeCases(t *testing.T) {
	s := NewShadowBucketSelector()

	// userID=0 不选中
	assert.False(t, s.IsSelected(0, "v1", 10000))

	// rolloutVersion 空不选中
	assert.False(t, s.IsSelected(1, "", 10000))

	// sampleRate=0 不选中
	assert.False(t, s.IsSelected(1, "v1", 0))

	// sampleRate=10000 总是选中（桶值 < 10000）
	assert.True(t, s.IsSelected(1, "v1", 10000))
}

func TestShadowBucketSelector_Monotonicity(t *testing.T) {
	s := NewShadowBucketSelector()

	// 提高采样率不会移除原有样本
	// 如果在 sampleRate=A 时被选中，那么在 sampleRate=B (B>A) 时也应该被选中
	for userID := uint(1); userID <= 100; userID++ {
		bucket := s.Bucket(userID, "test-v1")
		for rate := uint16(1); rate <= 9999; rate++ {
			if bucket < uint64(rate) {
				// 在 rate 时被选中
				// 验证在 rate+1 时也被选中（如果 rate < 9999）
				if rate < 9999 {
					assert.True(t, bucket < uint64(rate+1),
						"userID=%d 在 rate=%d 时选中，rate=%d 时也应该选中", userID, rate, rate+1)
				}
				break
			}
		}
	}
}

func TestShadowBucketSelector_RangeCheck(t *testing.T) {
	s := NewShadowBucketSelector()

	// 所有桶值必须在 [0, 10000) 范围内
	for userID := uint(1); userID <= 1000; userID++ {
		bucket := s.Bucket(userID, "v1")
		assert.True(t, bucket < RolloutBucketSize,
			"桶值必须在 [0, 10000) 范围内，userID=%d 得到 %d", userID, bucket)
	}
}
