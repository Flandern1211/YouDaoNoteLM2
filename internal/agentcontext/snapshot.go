package agentcontext

import (
	"fmt"

	"github.com/cloudwego/eino/schema"
)

// PreparedTurnSnapshot PreparedTurn 的可序列化快照
type PreparedTurnSnapshot struct {
	SchemaVersion     string
	RunID             string
	StepID            string
	Profile           ContextProfileSnapshot
	Prompt            PromptSnapshot
	MessagePlan       MessagePlanSnapshot
	ModelCapabilities ModelCapabilities
	BaseManifest      ContextManifest
}

// PromptSnapshot Prompt 快照
type PromptSnapshot struct {
	ID      string
	Version string
	// 不保存 Content，只保存版本信息
}

// MessagePlanSnapshot 消息计划快照
type MessagePlanSnapshot struct {
	Summary      *ContextItemSnapshot
	Memories     []ContextItemSnapshot
	HistoryCount int
	HasInput     bool
}

// ContextItemSnapshot 上下文项快照
type ContextItemSnapshot struct {
	ID         string
	Kind       ContextKind
	TokenCount int
	Pinned     bool
}

// SnapshotCodec 快照编解码器
type SnapshotCodec interface {
	Snapshot(turn *PreparedTurn) (PreparedTurnSnapshot, error)
	Restore(snapshot PreparedTurnSnapshot, session *TurnSession) (*PreparedTurn, error)
}

// DefaultSnapshotCodec 默认快照编解码器
type DefaultSnapshotCodec struct{}

// Snapshot 将 PreparedTurn 转换为快照
func (c *DefaultSnapshotCodec) Snapshot(turn *PreparedTurn) (PreparedTurnSnapshot, error) {
	if turn == nil {
		return PreparedTurnSnapshot{}, NewError(ErrCodeInvalidInput, "turn is nil")
	}

	snapshot := PreparedTurnSnapshot{
		SchemaVersion:     "v1",
		Profile:           turn.Profile,
		ModelCapabilities: turn.ModelCapabilities,
		BaseManifest:      turn.BaseManifest,
	}

	if turn.Session != nil {
		snapshot.RunID = turn.Session.Handle.RunID
		snapshot.StepID = turn.Session.Handle.StepID
	}

	// 快照 Prompt（只保存版本）
	snapshot.Prompt = PromptSnapshot{
		ID:      turn.Instruction,
		Version: "v1",
	}

	// 快照 MessagePlan
	snapshot.MessagePlan = c.snapshotMessagePlan(turn.MessagePlan)

	return snapshot, nil
}

// Restore 从快照恢复 PreparedTurn
func (c *DefaultSnapshotCodec) Restore(snapshot PreparedTurnSnapshot, session *TurnSession) (*PreparedTurn, error) {
	if snapshot.SchemaVersion != "v1" {
		return nil, NewError(ErrCodeInvalidConfig, fmt.Sprintf("unsupported schema version: %s", snapshot.SchemaVersion))
	}

	turn := &PreparedTurn{
		Session:           session,
		Profile:           snapshot.Profile,
		ModelCapabilities: snapshot.ModelCapabilities,
		BaseManifest:      snapshot.BaseManifest,
	}

	return turn, nil
}

func (c *DefaultSnapshotCodec) snapshotMessagePlan(plan MessagePlan) MessagePlanSnapshot {
	snapshot := MessagePlanSnapshot{
		HistoryCount: len(plan.History),
		HasInput:     plan.CurrentInput != nil,
	}

	if plan.Summary != nil {
		snapshot.Summary = &ContextItemSnapshot{
			ID:         plan.Summary.ID,
			Kind:       plan.Summary.Kind,
			TokenCount: plan.Summary.TokenCount,
			Pinned:     plan.Summary.Pinned,
		}
	}

	for _, mem := range plan.Memories {
		snapshot.Memories = append(snapshot.Memories, ContextItemSnapshot{
			ID:         mem.ID,
			Kind:       mem.Kind,
			TokenCount: mem.TokenCount,
			Pinned:     mem.Pinned,
		})
	}

	return snapshot
}

// ValidateSnapshot 验证快照兼容性
func ValidateSnapshot(snapshot PreparedTurnSnapshot, session *TurnSession) error {
	if snapshot.SchemaVersion != "v1" {
		return NewError(ErrCodeInvalidConfig, fmt.Sprintf("unsupported schema version: %s", snapshot.SchemaVersion))
	}

	if session != nil {
		if snapshot.RunID != session.Handle.RunID {
			return NewError(ErrCodeInvalidConfig, "snapshot run ID mismatch")
		}
		if snapshot.Profile.Key != session.Profile.Key {
			return NewError(ErrCodeInvalidConfig, "snapshot profile mismatch")
		}
	}

	return nil
}

// HistoryMessageToSchema 将 HistoryMessage 转换为 schema.Message
func HistoryMessageToSchema(msgs []HistoryMessage) []*schema.Message {
	result := make([]*schema.Message, len(msgs))
	for i, msg := range msgs {
		role := schema.RoleType(msg.Role)
		result[i] = &schema.Message{
			Role:    role,
			Content: msg.Content,
		}
	}
	return result
}
