package agentcontext

import (
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultSnapshotCodec_Snapshot(t *testing.T) {
	codec := &DefaultSnapshotCodec{}

	profile := NewChatV1Profile()
	turn := &PreparedTurn{
		Session: &TurnSession{
			Handle: AcceptedTurnHandle{
				RunID:  "run-1",
				StepID: "step-1",
			},
			Profile: profile.ToSnapshot(),
		},
		Profile:     profile.ToSnapshot(),
		Instruction: "test instruction",
		MessagePlan: MessagePlan{
			Summary: &ContextItem{
				ID:         "summary-1",
				Kind:       ContextKindConversationSummary,
				TokenCount: 100,
			},
			Memories: []ContextItem{
				{
					ID:         "memory-1",
					Kind:       ContextKindUserMemory,
					TokenCount: 50,
				},
			},
			History: []*schema.Message{
				{Role: schema.User, Content: "hello"},
			},
			CurrentInput: UserMessageInput{Content: "world"},
		},
	}

	snapshot, err := codec.Snapshot(turn)
	require.NoError(t, err)

	assert.Equal(t, "v1", snapshot.SchemaVersion)
	assert.Equal(t, "run-1", snapshot.RunID)
	assert.Equal(t, "step-1", snapshot.StepID)
	assert.NotNil(t, snapshot.MessagePlan.Summary)
	assert.Len(t, snapshot.MessagePlan.Memories, 1)
	assert.Equal(t, 1, snapshot.MessagePlan.HistoryCount)
	assert.True(t, snapshot.MessagePlan.HasInput)
}

func TestDefaultSnapshotCodec_Snapshot_NilTurn(t *testing.T) {
	codec := &DefaultSnapshotCodec{}

	_, err := codec.Snapshot(nil)
	require.Error(t, err)
	assert.True(t, IsErrorCode(err, ErrCodeInvalidInput))
}

func TestDefaultSnapshotCodec_Restore(t *testing.T) {
	codec := &DefaultSnapshotCodec{}

	profile := NewChatV1Profile()
	snapshot := PreparedTurnSnapshot{
		SchemaVersion: "v1",
		RunID:         "run-1",
		StepID:        "step-1",
		Profile:       profile.ToSnapshot(),
	}

	session := &TurnSession{
		Handle: AcceptedTurnHandle{
			RunID:  "run-1",
			StepID: "step-1",
		},
		Profile: profile.ToSnapshot(),
	}

	turn, err := codec.Restore(snapshot, session)
	require.NoError(t, err)
	assert.NotNil(t, turn)
}

func TestDefaultSnapshotCodec_Restore_InvalidVersion(t *testing.T) {
	codec := &DefaultSnapshotCodec{}

	snapshot := PreparedTurnSnapshot{
		SchemaVersion: "v2", // 不支持的版本
	}

	_, err := codec.Restore(snapshot, nil)
	require.Error(t, err)
	assert.True(t, IsErrorCode(err, ErrCodeInvalidConfig))
}

func TestValidateSnapshot(t *testing.T) {
	profile := NewChatV1Profile()
	snapshot := PreparedTurnSnapshot{
		SchemaVersion: "v1",
		RunID:         "run-1",
		Profile:       profile.ToSnapshot(),
	}

	session := &TurnSession{
		Handle: AcceptedTurnHandle{
			RunID: "run-1",
		},
		Profile: profile.ToSnapshot(),
	}

	err := ValidateSnapshot(snapshot, session)
	require.NoError(t, err)
}

func TestValidateSnapshot_RunIDMismatch(t *testing.T) {
	profile := NewChatV1Profile()
	snapshot := PreparedTurnSnapshot{
		SchemaVersion: "v1",
		RunID:         "run-1",
		Profile:       profile.ToSnapshot(),
	}

	session := &TurnSession{
		Handle: AcceptedTurnHandle{
			RunID: "run-2", // 不匹配
		},
		Profile: profile.ToSnapshot(),
	}

	err := ValidateSnapshot(snapshot, session)
	require.Error(t, err)
	assert.True(t, IsErrorCode(err, ErrCodeInvalidConfig))
}

func TestValidateSnapshot_ProfileMismatch(t *testing.T) {
	chatProfile := NewChatV1Profile()
	searchProfile := NewSearchV1Profile()
	snapshot := PreparedTurnSnapshot{
		SchemaVersion: "v1",
		RunID:         "run-1",
		Profile:       chatProfile.ToSnapshot(),
	}

	session := &TurnSession{
		Handle: AcceptedTurnHandle{
			RunID: "run-1",
		},
		Profile: searchProfile.ToSnapshot(), // 不匹配
	}

	err := ValidateSnapshot(snapshot, session)
	require.Error(t, err)
	assert.True(t, IsErrorCode(err, ErrCodeInvalidConfig))
}

func TestHistoryMessageToSchema(t *testing.T) {
	msgs := []HistoryMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
	}

	result := HistoryMessageToSchema(msgs)
	assert.Len(t, result, 2)
	assert.Equal(t, schema.User, result[0].Role)
	assert.Equal(t, "hello", result[0].Content)
	assert.Equal(t, schema.Assistant, result[1].Role)
	assert.Equal(t, "hi", result[1].Content)
}
