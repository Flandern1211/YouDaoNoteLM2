package agentcontext

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

// RolloutBucketSize 分桶总数（0-9999）
const RolloutBucketSize = 10000

// ShadowBucketSelector SHA-256 稳定分桶选择器。
// 按 D33 使用字节级算法，保证同一 userID + rolloutVersion 始终落在同一桶。
type ShadowBucketSelector struct{}

// NewShadowBucketSelector 创建分桶选择器。
func NewShadowBucketSelector() *ShadowBucketSelector {
	return &ShadowBucketSelector{}
}

// IsSelected 判断用户是否被选中参与 Shadow 采样。
// 算法：
//
//	input  = "context-shadow:" + rolloutVersion + ":" + decimal(userID)
//	digest = SHA-256(input)
//	bucket = BigEndianUint64(digest[0:8]) % 10000
//	selected = bucket < sampleRateBasisPoints
func (s *ShadowBucketSelector) IsSelected(userID uint, rolloutVersion string, sampleRateBasisPoints uint16) bool {
	if userID == 0 || rolloutVersion == "" || sampleRateBasisPoints == 0 {
		return false
	}

	bucket := s.Bucket(userID, rolloutVersion)
	return bucket < uint64(sampleRateBasisPoints)
}

// Bucket 计算用户的分桶值（0-9999）。
// 使用固定向量测试，不只验证"结果范围正确"。
func (s *ShadowBucketSelector) Bucket(userID uint, rolloutVersion string) uint64 {
	input := fmt.Sprintf("context-shadow:%s:%d", rolloutVersion, userID)
	digest := sha256.Sum256([]byte(input))

	// 取前 8 字节，按大端无符号整数解释
	bucket := binary.BigEndian.Uint64(digest[0:8]) % RolloutBucketSize
	return bucket
}
