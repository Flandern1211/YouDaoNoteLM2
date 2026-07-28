package eino

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"YoudaoNoteLm/internal/agentcontext"
)

func TestEinoSnapshotCodec_RoundTrip(t *testing.T) {
	codec := NewEinoSnapshotCodec()

	turn := &agentcontext.PreparedTurn{
		Session: &agentcontext.TurnSession{
			Handle: agentcontext.AcceptedTurnHandle{
				RunID:  "run-1",
				StepID: "step-0",
			},
		},
		Profile: agentcontext.ContextProfileSnapshot{
			Key:     agentcontext.ChatV1,
			AgentID: agentcontext.AgentIDChat,
		},
		Instruction: "You are a helpful assistant",
		MessagePlan: agentcontext.MessagePlan{
			Summary: &agentcontext.ContextItem{ID: "summary", Content: "test"},
			Memories: []agentcontext.ContextItem{
				{ID: "mem1", Content: "memory1"},
			},
			History:      nil,
			CurrentInput: agentcontext.UserMessageInput{Content: "hello"},
		},
		BaseManifest: agentcontext.ContextManifest{
			TurnStatus: "prepared",
			Degraded:   false,
		},
	}

	// Encode
	snapshot, err := codec.Encode(turn)
	require.NoError(t, err)
	assert.Equal(t, SnapshotCodecVersion, snapshot.SchemaVersion)
	assert.Equal(t, "v1", snapshot.ProfileVersion)
	assert.Equal(t, "chat", snapshot.ProfileName)
	assert.Equal(t, "chat", snapshot.AgentID)
	assert.Equal(t, "run-1", snapshot.RunID)
	assert.Equal(t, "step-0", snapshot.StepID)
	assert.True(t, snapshot.MessagePlanSnapshot.HasSummary)
	assert.Equal(t, 1, snapshot.MessagePlanSnapshot.MemoryCount)
	assert.True(t, snapshot.MessagePlanSnapshot.HasCurrentInput)

	// Decode
	decoded, err := codec.Decode(snapshot)
	require.NoError(t, err)
	assert.Equal(t, "run-1", decoded.Session.Handle.RunID)
	assert.Equal(t, "step-0", decoded.Session.Handle.StepID)
	assert.Equal(t, agentcontext.ChatV1, decoded.Profile.Key)
	assert.Equal(t, "You are a helpful assistant", decoded.Instruction)
	assert.Equal(t, "prepared", decoded.BaseManifest.TurnStatus)
}

func TestEinoSnapshotCodec_UnknownVersion(t *testing.T) {
	codec := NewEinoSnapshotCodec()

	snapshot := &VersionedSnapshot{
		SchemaVersion:  "v999",
		ProfileVersion: "v1",
		ProfileName:    "chat",
	}

	_, err := codec.Decode(snapshot)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "不支持的 schema 版本")
}

func TestEinoSnapshotCodec_NilTurn(t *testing.T) {
	codec := NewEinoSnapshotCodec()
	_, err := codec.Encode(nil)
	require.Error(t, err)
}

func TestEinoSnapshotCodec_NilSnapshot(t *testing.T) {
	codec := NewEinoSnapshotCodec()
	_, err := codec.Decode(nil)
	require.Error(t, err)
}

func TestEinoSnapshotCodec_ValidateVersion(t *testing.T) {
	codec := NewEinoSnapshotCodec()

	// 有效版本
	err := codec.ValidateVersion(&VersionedSnapshot{SchemaVersion: SnapshotCodecVersion})
	assert.NoError(t, err)

	// 无效版本
	err = codec.ValidateVersion(&VersionedSnapshot{SchemaVersion: "v2"})
	assert.Error(t, err)

	// nil
	err = codec.ValidateVersion(nil)
	assert.Error(t, err)
}
