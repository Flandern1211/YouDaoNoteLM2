package eino

import (
	"fmt"

	"YoudaoNoteLm/internal/agentcontext"
)

// SnapshotCodecVersion 当前快照 codec 版本
const SnapshotCodecVersion = "v1"

// EinoSnapshotCodec 版本化快照编解码器。
// 包含 schema version 和 profile version。
// W4 只在当前进程 Eino state 中使用；不得宣称已经跨进程恢复。
type EinoSnapshotCodec struct{}

// NewEinoSnapshotCodec 创建版本化快照编解码器。
func NewEinoSnapshotCodec() *EinoSnapshotCodec {
	return &EinoSnapshotCodec{}
}

// Encode 将 PreparedTurn 编码为可序列化的快照。
func (c *EinoSnapshotCodec) Encode(turn *agentcontext.PreparedTurn) (*VersionedSnapshot, error) {
	if turn == nil {
		return nil, agentcontext.NewError(agentcontext.ErrCodeInvalidInput, "turn 不能为空")
	}

	snapshot := &VersionedSnapshot{
		SchemaVersion:  SnapshotCodecVersion,
		ProfileVersion: turn.Profile.Key.Version,
		ProfileName:    turn.Profile.Key.Name,
		AgentID:        string(turn.Profile.AgentID),
		Instruction:    turn.Instruction,
		TurnStatus:     turn.BaseManifest.TurnStatus,
		Degraded:       turn.BaseManifest.Degraded,
	}

	if turn.Session != nil {
		snapshot.RunID = turn.Session.Handle.RunID
		snapshot.StepID = turn.Session.Handle.StepID
	}

	// 编码 MessagePlan 元数据（不保存完整内容，只保存版本信息）
	snapshot.MessagePlanSnapshot = MessagePlanMeta{
		HasSummary:      turn.MessagePlan.Summary != nil,
		MemoryCount:     len(turn.MessagePlan.Memories),
		HistoryCount:    len(turn.MessagePlan.History),
		HasCurrentInput: turn.MessagePlan.CurrentInput != nil,
	}

	return snapshot, nil
}

// Decode 从快照恢复 PreparedTurn。
// W4 只验证版本兼容性，不执行完整恢复（完整恢复属于 W5）。
func (c *EinoSnapshotCodec) Decode(snapshot *VersionedSnapshot) (*agentcontext.PreparedTurn, error) {
	if snapshot == nil {
		return nil, agentcontext.NewError(agentcontext.ErrCodeInvalidInput, "snapshot 不能为空")
	}

	if snapshot.SchemaVersion != SnapshotCodecVersion {
		return nil, agentcontext.NewError(
			agentcontext.ErrCodeInvalidConfig,
			fmt.Sprintf("不支持的 schema 版本: %s（期望: %s）", snapshot.SchemaVersion, SnapshotCodecVersion),
		)
	}

	// W4 只验证版本兼容性，返回最小 PreparedTurn
	// 完整恢复需要重新调用 Provider，属于 W5 范畴
	profileKey := agentcontext.ProfileKey{
		Name:    snapshot.ProfileName,
		Version: snapshot.ProfileVersion,
	}

	return &agentcontext.PreparedTurn{
		Session: &agentcontext.TurnSession{
			Handle: agentcontext.AcceptedTurnHandle{
				RunID:   snapshot.RunID,
				StepID:  snapshot.StepID,
				AgentID: agentcontext.AgentID(snapshot.AgentID),
			},
			Profile: agentcontext.ContextProfileSnapshot{
				Key:     profileKey,
				AgentID: agentcontext.AgentID(snapshot.AgentID),
			},
		},
		Profile: agentcontext.ContextProfileSnapshot{
			Key:     profileKey,
			AgentID: agentcontext.AgentID(snapshot.AgentID),
		},
		Instruction: snapshot.Instruction,
		BaseManifest: agentcontext.ContextManifest{
			TurnStatus: snapshot.TurnStatus,
			Degraded:   snapshot.Degraded,
		},
	}, nil
}

// ValidateVersion 验证快照版本兼容性。
func (c *EinoSnapshotCodec) ValidateVersion(snapshot *VersionedSnapshot) error {
	if snapshot == nil {
		return agentcontext.NewError(agentcontext.ErrCodeInvalidInput, "snapshot 不能为空")
	}
	if snapshot.SchemaVersion != SnapshotCodecVersion {
		return agentcontext.NewError(
			agentcontext.ErrCodeInvalidConfig,
			fmt.Sprintf("不支持的 schema 版本: %s（期望: %s）", snapshot.SchemaVersion, SnapshotCodecVersion),
		)
	}
	return nil
}

// VersionedSnapshot 版本化的 PreparedTurn 快照。
type VersionedSnapshot struct {
	// SchemaVersion 快照 schema 版本
	SchemaVersion string `json:"schema_version"`
	// ProfileVersion Profile 版本
	ProfileVersion string `json:"profile_version"`
	// ProfileName Profile 名称
	ProfileName string `json:"profile_name"`
	// AgentID Agent 标识
	AgentID string `json:"agent_id"`
	// RunID 运行 ID
	RunID string `json:"run_id"`
	// StepID 步骤 ID
	StepID string `json:"step_id"`
	// Instruction 系统指令
	Instruction string `json:"instruction"`
	// TurnStatus Turn 状态
	TurnStatus string `json:"turn_status"`
	// Degraded 是否降级
	Degraded bool `json:"degraded"`
	// MessagePlanSnapshot 消息计划元数据
	MessagePlanSnapshot MessagePlanMeta `json:"message_plan"`
}

// MessagePlanMeta 消息计划元数据（不包含完整内容）
type MessagePlanMeta struct {
	HasSummary      bool `json:"has_summary"`
	MemoryCount     int  `json:"memory_count"`
	HistoryCount    int  `json:"history_count"`
	HasCurrentInput bool `json:"has_current_input"`
}
