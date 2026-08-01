// Package run 定义 RunStore 和 AdmissionStore 的使用方接口。
package run

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/google/uuid"

	"YoudaoNoteLm/internal/agentharness/core"
)

// AdmissionService 提供 Run 的 Admission 操作。
type AdmissionService struct {
	store core.AdmissionStore
}

// NewAdmissionService 创建 AdmissionService。
func NewAdmissionService(store core.AdmissionStore) *AdmissionService {
	return &AdmissionService{store: store}
}

// Accept 接纳用户意图，在同一事务中创建入口消息、queued Run 和首条事件。
// 如果客户端未提供 idempotency_key，则自动生成一个。
func (s *AdmissionService) Accept(ctx context.Context, req core.AcceptRequest) (core.AcceptedRun, error) {
	// 校验必填字段
	if req.UserID == 0 {
		return core.AcceptedRun{}, fmt.Errorf("user_id 不能为空")
	}
	if req.AgentType == "" {
		return core.AcceptedRun{}, fmt.Errorf("agent_type 不能为空")
	}
	if req.Input.Kind == "" || req.Input.Ref == "" {
		return core.AcceptedRun{}, fmt.Errorf("input.kind 和 input.ref 不能为空")
	}

	// 如果客户端未提供 idempotency_key，自动生成
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = uuid.NewString()
	}

	// 如果 InputHash 为空，根据 Input.Ref 计算
	if req.Input.Hash == "" {
		hash := sha256.Sum256([]byte(req.Input.Ref))
		req.Input.Hash = hex.EncodeToString(hash[:])
	}

	// 调用 store 的 Accept 方法
	accepted, err := s.store.Accept(ctx, req)
	if err != nil {
		return core.AcceptedRun{}, fmt.Errorf("admission accept 失败: %w", err)
	}

	return accepted, nil
}
